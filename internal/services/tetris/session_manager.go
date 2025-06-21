package tetris

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket" // WebSocketライブラリのインポート

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database" // データベースサービスをインポート
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// Client はWebSocket接続を持つ単一のクライアントを表します。
type Client struct {
	UserID string          // このクライアントに紐づくユーザーのID
	Conn   *websocket.Conn // クライアントとの実際のWebSocketコネクション
	Send   chan []byte     // クライアントへメッセージを送信するためのバッファ付きチャネル
	RoomID string          // このクライアントが現在参加しているルームのID
	closed bool            // チャネルが閉じられたかどうかのフラグ
	mu     sync.Mutex      // closedフラグ保護用
}

// SafeSend は安全にチャネルにメッセージを送信します（closedチェック付き）
func (c *Client) SafeSend(message []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.closed {
		return false // 既に閉じられている
	}
	
	select {
	case c.Send <- message:
		return true // 送信成功
	default:
		return false // チャネルがフル
	}
}

// SafeClose は安全にチャネルを閉じます
func (c *Client) SafeClose() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.closed {
		close(c.Send)
		c.closed = true
	}
}

// LightweightGameState はWebSocket送信用の軽量なゲーム状態構造体です。
// GameSessionの全情報ではなく、クライアントが必要とする最小限の情報のみを含みます。
type LightweightGameState struct {
	ID        string                    `json:"id"`
	Player1   *LightweightPlayerState   `json:"player1"`
	Player2   *LightweightPlayerState   `json:"player2"`
	Status    string                    `json:"status"`
	StartedAt time.Time                 `json:"started_at,omitempty"`
	EndedAt   time.Time                 `json:"ended_at,omitempty"`
}

// LightweightPlayerState はプレイヤー状態の軽量版です。
type LightweightPlayerState struct {
	UserID             string             `json:"user_id"`
	Board              tetris.Board       `json:"board"`
	CurrentPiece       *tetris.Piece      `json:"current_piece"`
	NextPiece          *tetris.Piece      `json:"next_piece"`
	HeldPiece          *tetris.Piece      `json:"held_piece,omitempty"`
	Score              int                `json:"score"`
	LinesCleared       int                `json:"lines_cleared"`
	Level              int                `json:"level"`
	IsGameOver         bool               `json:"is_game_over"`
	ContributionScores map[string]int     `json:"contribution_scores"`
	CurrentPieceScores map[string]int     `json:"current_piece_scores"`
}

// SessionManager はゲームセッションとWebSocketクライアント接続の全体を管理します。
// これはアプリケーション内でシングルトンとして動作することが想定されます。
type SessionManager struct {
	sessions    map[string]*GameSession // roomID -> GameSession のマップ (アクティブなゲームセッションを保持)
	clients     map[string]*Client             // userID -> Client のマップ (現在接続中の全WebSocketクライアント)
	register    chan *Client                   // 新しいクライアント接続の登録リクエスト用チャネル
	unregister  chan *Client                   // クライアント切断の登録解除リクエスト用チャネル
	broadcast   chan *GameStateEvent          // ゲーム状態の更新をブロードキャストするためのチャネル
	inputEvents chan PlayerInputEvent         // クライアントからのプレイヤー操作入力を受け取るチャネル
	quit        chan struct{}                  // シャットダウン用チャネル
	mu          sync.RWMutex                   // sessions と clients マップへのアクセスを保護するためのRWMutex
	dbService   *database.DatabaseService      // データベース操作のためのサービス
	deckRepo    database.DeckRepository        // デッキリポジトリ（テトリミノ配置データ取得用）
	lastBroadcast map[string]time.Time          // ルームごとの最後のブロードキャスト時刻
	broadcastMu   sync.Mutex                    // lastBroadcastマップへのアクセス保護用
}

// NewSessionManager は新しい SessionManager インスタンスを作成し、そのメインイベントループをバックグラウンドで開始します。
//
// Parameters:
//   db : データベースサービスへのポインタ
//   deckRepo : デッキリポジトリ
// Returns:
//   *SessionManager: 初期化されたセッションマネージャーのポインタ
func NewSessionManager(db *database.DatabaseService, deckRepo database.DeckRepository) *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[string]*GameSession),
		clients:     make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *GameStateEvent, 512),   // ゲーム状態更新の頻度を考慮し、大きめのバッファ
		inputEvents: make(chan PlayerInputEvent, 512), // プレイヤー操作のキューイング用
		quit:        make(chan struct{}),
		dbService:   db,
		deckRepo:    deckRepo,
		lastBroadcast: make(map[string]time.Time),
		broadcastMu: sync.Mutex{},
	}
	go sm.Run() // SessionManager のメインイベントループをゴルーチンで開始
	return sm
}

// Run は SessionManager のメインイベントループです。
// このゴルーチンは、クライアントの登録/解除、プレイヤー入力の処理、自動落下タイマーの管理、
// そしてゲーム状態のブロードキャストといったすべての主要なイベントを処理します。
func (sm *SessionManager) Run() {
	// 自動落下用のタイマー（さらに軽量化）
	ticker := time.NewTicker(1000 * time.Millisecond) // 1秒間隔で大幅軽量化
	defer ticker.Stop()

	for {
		select {
		case client := <-sm.register:
			// 新しいクライアントの登録処理
			sm.mu.Lock()
			sm.clients[client.UserID] = client
			sm.mu.Unlock()
			log.Printf("[SessionManager] Client registered: %s (Room: %s)", client.UserID, client.RoomID)

			// クライアント登録後に最新の状態をブロードキャスト（非同期実行）
			go func(roomID string) {
				sm.BroadcastGameState(roomID)
			}(client.RoomID)

			// クライアント登録後、セッションが開始可能かチェック（非同期実行、少し遅延させてレースコンディション回避）
			go func(roomID string) {
				time.Sleep(50 * time.Millisecond) // 50ms遅延でレースコンディション回避
				sm.CheckAndStartGame(roomID)
			}(client.RoomID)

		case client := <-sm.unregister:
			// クライアントの登録解除処理
			sm.mu.Lock()
			if registeredClient, ok := sm.clients[client.UserID]; ok {
				// Sendチャネルを安全に閉じる
				registeredClient.SafeClose()
				delete(sm.clients, client.UserID)
				log.Printf("[SessionManager] Client unregistered: %s (Room: %s)", client.UserID, client.RoomID)
			} else {
				log.Printf("[SessionManager] Attempted to unregister non-existent client: %s", client.UserID)
			}
			sm.mu.Unlock()

			// プレイヤーがゲーム中に退出した場合、セッションを終了させる
			sm.mu.RLock()
			session, ok := sm.sessions[client.RoomID]
			sm.mu.RUnlock()
			if ok && session.Status == "playing" {
				log.Printf("[SessionManager] Player %s left room %s during game. Ending session.", client.UserID, client.RoomID)
				sm.EndGameSession(client.RoomID)
			} else if ok {
				// ゲーム中でない場合は、セッション状態を更新してブロードキャスト
				log.Printf("[SessionManager] Player %s left room %s (status: %s)", client.UserID, client.RoomID, session.Status)
				sm.BroadcastGameState(client.RoomID)
			}

		case event := <-sm.inputEvents:
			// プレイヤーからの入力イベントを処理
			// クライアントのルームIDを取得
			sm.mu.RLock()
			client, clientExists := sm.clients[event.UserID]
			sm.mu.RUnlock()
			
			if !clientExists {
				log.Printf("[SessionManager] Received input from unregistered user %s", event.UserID)
				continue
			}
			
			sm.mu.RLock()
			session, ok := sm.sessions[client.RoomID]
			sm.mu.RUnlock()

			if !ok || session.Status != "playing" {
				log.Printf("[SessionManager] Received input for non-existent or non-playing room %s from user %s", client.RoomID, event.UserID)
				continue // 存在しないか、プレイ中でない部屋への入力は無視
			}

			// どちらのプレイヤーからの入力か判定し、対応するゲーム状態を更新
			var targetPlayerState *PlayerGameState
			if session.Player1 != nil && session.Player1.UserID == event.UserID {
				targetPlayerState = session.Player1
			} else if session.Player2 != nil && session.Player2.UserID == event.UserID {
				targetPlayerState = session.Player2
			} else {
				log.Printf("[SessionManager] Input from unknown user %s in room %s", event.UserID, client.RoomID)
				continue
			}

			// ゲームロジックを適用し、状態が実際に変更されたか確認
			if ApplyPlayerInput(targetPlayerState, event.Action) {
				// 自分の操作は即座に自分にだけ送信（レスポンシブ感を維持）
				go func(userID, roomID string) {
					sm.BroadcastToSpecificClient(userID, roomID)
				}(event.UserID, session.ID)
				
				// 相手への更新は1秒間隔のブロードキャストに任せる（負荷軽減）
				// （自動落下タイマーでブロードキャストされるため、ここでは相手への送信は不要）

				// プレイヤーのゲームが終了したか判定（ゲームオーバーは即座に通知）
				if targetPlayerState.IsGameOver {
					// ゲームオーバーは重要なので即座にブロードキャスト
					go func(roomID string) {
						sm.BroadcastGameState(roomID)
					}(session.ID)
					sm.EndGameSession(session.ID)
				}
			}

		case <-ticker.C:
			// 自動落下処理を全プレイ中セッションで実行（パフォーマンス最適化）
			sm.mu.RLock()
			activeSessions := make([]*GameSession, 0) // アクティブセッションのみコピー
			for _, session := range sm.sessions {
				if session.Status == "playing" {
					activeSessions = append(activeSessions, session)
				}
			}
			sm.mu.RUnlock()

			// ロック外で処理を実行（パフォーマンス改善）
			for _, session := range activeSessions {
				// プレイヤー1の自動落下
				if session.Player1 != nil && !session.Player1.IsGameOver {
					AutoFall(session.Player1)
				}
				// プレイヤー2の自動落下
				if session.Player2 != nil && !session.Player2.IsGameOver {
					AutoFall(session.Player2)
				}

				// 自動落下時は常にブロードキャスト（1秒間隔なので相手の状態更新のタイミング）
				go func(roomID string) {
					sm.BroadcastGameState(roomID)
				}(session.ID)

				// ゲームオーバー判定 (自動落下でゲームオーバーになることもありえる)
				if (session.Player1 != nil && session.Player1.IsGameOver) || (session.Player2 != nil && session.Player2.IsGameOver) {
					sm.EndGameSession(session.ID)
				}
			}

		case event := <-sm.broadcast:
			// ゲーム状態のブロードキャスト処理
			sm.mu.RLock()
			session, ok := sm.sessions[event.RoomID]
			if !ok {
				sm.mu.RUnlock()
				log.Printf("[SessionManager] Attempted to broadcast for non-existent room: %s", event.RoomID)
				continue
			}

			// GameSessionを軽量な構造体に変換してからJSON形式でシリアライズ
			lightweightState := session.ToLightweight()
			stateJSON, err := json.Marshal(lightweightState)
			if err != nil {
				log.Printf("[SessionManager] Error marshaling lightweight game state for room %s: %v", event.RoomID, err)
				sm.mu.RUnlock()
				continue
			}

			// ルーム内の各クライアントにゲーム状態を送信
			for _, client := range sm.clients {
				if client.RoomID == event.RoomID {
					// 安全な送信メソッドを使用
					if !client.SafeSend(stateJSON) {
						log.Printf("[SessionManager] Failed to send to client %s (channel closed or full)", client.UserID)
					}
				}
			}
			sm.mu.RUnlock()
		
		case <-sm.quit:
			// シャットダウンシグナルを受信したらメインループを終了
			log.Printf("[SessionManager] シャットダウンシグナルを受信、メインループを終了します")
			return
		}
	}
}

// CreateSession は新しいゲームセッションを作成します。
//
// Parameters:
//   player1ID   : プレイヤー1のユーザーID
//   player1DeckID : プレイヤー1が使用するデッキのUUID
// Returns:
//   string: 作成されたルームのID
//   error : エラーが発生した場合
func (sm *SessionManager) CreateSession(player1ID, player1DeckID string) (string, error) {
	log.Printf("[SessionManager] CreateSession called with player1ID: %s, player1DeckID: %s", player1ID, player1DeckID)
	
	sm.mu.Lock()

	roomID := uuid.New().String() // 新しいルームIDを生成
	log.Printf("[SessionManager] Generated roomID: %s", roomID)

	// データベースからプレイヤー1のデッキデータをロード (TODO: database/database_service.goに実装)
	log.Printf("[SessionManager] Attempting to get deck for player1: %s", player1DeckID)
	player1Deck, err := sm.dbService.GetDeckByID(player1DeckID)
	if err != nil {
		log.Printf("[SessionManager] Failed to get player1 deck %s: %v", player1DeckID, err)
		sm.mu.Unlock() // エラー時にアンロック
		return "", fmt.Errorf("failed to get player1 deck: %w", err)
	}
	log.Printf("[SessionManager] Successfully retrieved deck for player1")

	// 新しいゲームセッションを初期化
	log.Printf("[SessionManager] Creating new GameSession...")
	session, err := NewGameSession(roomID, player1ID, player1Deck, sm.deckRepo)
	if err != nil {
		log.Printf("[SessionManager] Failed to create GameSession: %v", err)
		sm.mu.Unlock()
		return "", fmt.Errorf("failed to create game session: %w", err)
	}
	sm.sessions[roomID] = session // セッションマネージャーのマップに追加
	log.Printf("[SessionManager] GameSession created and added to sessions map")

	// データベースにセッションを記録 (game_sessions テーブル)
	// TODO: sm.dbService.CreateGameSession(session) を実装
	// err = sm.dbService.CreateGameSession(session)
	// if err != nil {
	// 	delete(sm.sessions, roomID) // DB記録失敗時はセッションを削除
	// 	return "", fmt.Errorf("failed to save game session to DB: %w", err)
	// }
	log.Printf("[SessionManager] Created new game session: %s for player %s", roomID, player1ID)
	
	// mutexをアンロックしてからブロードキャスト
	sm.mu.Unlock()
	
	// セッション作成後に状態をブロードキャスト
	log.Printf("[SessionManager] Broadcasting game state for room: %s", roomID)
	sm.BroadcastGameState(roomID)
	
	return roomID, nil
}

// JoinSession は既存のゲームセッションにプレイヤーを参加させます。
//
// Parameters:
//   roomID      : 参加するルームのID
//   player2ID   : プレイヤー2のユーザーID
//   player2DeckID : プレイヤー2が使用するデッキのUUID
// Returns:
//   error : エラーが発生した場合
func (sm *SessionManager) JoinSession(roomID, player2ID, player2DeckID string) error {
	log.Printf("[SessionManager] JoinSession called with roomID: %s, player2ID: %s, player2DeckID: %s", roomID, player2ID, player2DeckID)
	
	sm.mu.Lock()

	session, ok := sm.sessions[roomID]
	if !ok {
		log.Printf("[SessionManager] Room %s not found for player2 %s", roomID, player2ID)
		sm.mu.Unlock()
		return errors.New("room not found")
	}
	log.Printf("[SessionManager] Room %s found, current status: %s", roomID, session.Status)
	
	if session.Status != "waiting" {
		log.Printf("[SessionManager] Room %s is not waiting (status: %s) for player2 %s", roomID, session.Status, player2ID)
		sm.mu.Unlock()
		return errors.New("room is not waiting for players")
	}
	
	if session.Player2 != nil {
		log.Printf("[SessionManager] Room %s already has player2, cannot add %s", roomID, player2ID)
		sm.mu.Unlock()
		return errors.New("room is already full") // 既に2人目が参加している
	}
	
	if session.Player1 != nil && session.Player1.UserID == player2ID {
		log.Printf("[SessionManager] Player %s cannot join their own room %s", player2ID, roomID)
		sm.mu.Unlock()
		return errors.New("cannot join your own room") // 自分自身の部屋には参加できない
	}

	log.Printf("[SessionManager] Validation passed, getting deck for player2: %s", player2DeckID)
	
	// データベースからプレイヤー2のデッキデータをロード (TODO: database/database_service.goに実装)
	player2Deck, err := sm.dbService.GetDeckByID(player2DeckID)
	if err != nil {
		log.Printf("[SessionManager] Failed to get player2 deck %s: %v", player2DeckID, err)
		sm.mu.Unlock()
		return fmt.Errorf("failed to get player2 deck: %w", err)
	}
	log.Printf("[SessionManager] Successfully retrieved deck for player2: %s", player2DeckID)

	log.Printf("[SessionManager] Setting player2 for room %s", roomID)
	session.SetPlayer2(player2ID, player2Deck, sm.deckRepo) // プレイヤー2のゲーム状態を設定

	// データベースの game_participants テーブルを更新
	// TODO: sm.dbService.AddParticipantToSession(roomID, player2ID, player2DeckID) を実装
	log.Printf("[SessionManager] Player %s joined room %s successfully", player2ID, roomID)

	// mutexをアンロックしてからブロードキャスト
	sm.mu.Unlock()

	// プレイヤー参加後に状態をブロードキャスト
	log.Printf("[SessionManager] Broadcasting game state after player2 join for room: %s", roomID)
	sm.BroadcastGameState(roomID)

	return nil
}

// CheckAndStartGame はセッションが開始条件を満たしているかチェックし、満たしていればゲームを開始します。
//
// Parameters:
//   roomID : チェックするルームのID
func (sm *SessionManager) CheckAndStartGame(roomID string) {
	log.Printf("[SessionManager] CheckAndStartGame called for room: %s", roomID)
	
	sm.mu.Lock()
	defer sm.mu.Unlock() // defer で必ずアンロックされるように変更

	// デバッグ用: 現在のセッション一覧をログ出力
	sessionCount := len(sm.sessions)
	log.Printf("[SessionManager] Current session count: %d", sessionCount)
	
	session, ok := sm.sessions[roomID]
	if !ok {
		log.Printf("[SessionManager] Room %s not found in CheckAndStartGame (total sessions: %d)", roomID, sessionCount)
		// デバッグ用: 存在するセッションIDをログ出力
		var existingRooms []string
		for id := range sm.sessions {
			existingRooms = append(existingRooms, id)
		}
		log.Printf("[SessionManager] Existing room IDs: %v", existingRooms)
		return // ルームが存在しない
	}
	
	// セッションの状態をチェック（削除された可能性を考慮）
	if session == nil {
		log.Printf("[SessionManager] Session for room %s is nil", roomID)
		return
	}
	
	log.Printf("[SessionManager] Room %s status: %s", roomID, session.Status)
	
	// 各条件をチェック
	hasPlayer1 := session.Player1 != nil
	hasPlayer2 := session.Player2 != nil
	
	log.Printf("[SessionManager] Room %s - hasPlayer1: %v, hasPlayer2: %v", roomID, hasPlayer1, hasPlayer2)
	
	if hasPlayer1 {
		log.Printf("[SessionManager] Room %s - Player1 ID: %s", roomID, session.Player1.UserID)
	}
	if hasPlayer2 {
		log.Printf("[SessionManager] Room %s - Player2 ID: %s", roomID, session.Player2.UserID)
	}
	
	// WebSocket接続をチェック
	var player1Connected, player2Connected bool
	if hasPlayer1 {
		player1Connected = sm.clients[session.Player1.UserID] != nil
		log.Printf("[SessionManager] Room %s - Player1 (%s) connected: %v", roomID, session.Player1.UserID, player1Connected)
	}
	if hasPlayer2 {
		player2Connected = sm.clients[session.Player2.UserID] != nil
		log.Printf("[SessionManager] Room %s - Player2 (%s) connected: %v", roomID, session.Player2.UserID, player2Connected)
	}
	
	isWaiting := session.Status == "waiting"
	log.Printf("[SessionManager] Room %s - isWaiting: %v", roomID, isWaiting)

	// 2人のプレイヤーが揃っていて、両方がWebSocketに接続済みであればゲーム開始
	// TODO: このロジックはより複雑になる可能性あり（例: 両プレイヤーが「準備OK」ボタンを押す必要がある場合など）
	if hasPlayer1 && hasPlayer2 && player1Connected && player2Connected && isWaiting {
		log.Printf("[SessionManager] All conditions met, starting game for room %s", roomID)
		
		session.Status = "playing"
		session.StartedAt = time.Now()
		log.Printf("[SessionManager] Game session %s started! Players: %s vs %s", roomID, session.Player1.UserID, session.Player2.UserID)

		// データベースの game_sessions テーブルを更新
		// TODO: sm.dbService.UpdateGameSessionStatus(roomID, "playing", session.StartedAt) を実装
		
		// ゲーム開始をクライアントに通知（非同期実行）
		go func(roomID string) {
			sm.BroadcastGameState(roomID) 
		}(roomID)
		return
	} else {
		log.Printf("[SessionManager] Game start conditions not met for room %s", roomID)
		log.Printf("[SessionManager] - hasPlayer1: %v, hasPlayer2: %v, player1Connected: %v, player2Connected: %v, isWaiting: %v", 
			hasPlayer1, hasPlayer2, player1Connected, player2Connected, isWaiting)
	}
}

// RegisterClient は新しいWebSocketクライアントをSessionManagerに登録します。
//
// Parameters:
//   roomID : クライアントが参加するルームのID
//   userID : クライアントのユーザーID
//   conn   : WebSocketコネクション
// Returns:
//   error: エラーが発生した場合
func (sm *SessionManager) RegisterClient(roomID, userID string, conn *websocket.Conn) error {
	log.Printf("[SessionManager] RegisterClient called for user %s in room %s", userID, roomID)

	// 既存の接続があれば先にクリーンアップ（再接続対応）
	sm.mu.Lock()
	if existingClient, exists := sm.clients[userID]; exists {
		log.Printf("[SessionManager] Replacing existing connection for user %s", userID)
		if existingClient.Conn != nil {
			existingClient.Conn.Close()
		}
		if existingClient.Send != nil {
			close(existingClient.Send)
		}
	}

	// 新しいクライアントを作成
	client := &Client{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 512), // バッファサイズをさらに増加
		RoomID: roomID,
	}
	sm.clients[userID] = client
	sm.mu.Unlock()

	// WebSocket接続の基本設定（パフォーマンス最適化）
	conn.SetReadLimit(2048)                                    // 読み取り制限を2KBに増加
	conn.SetReadDeadline(time.Now().Add(300 * time.Second))    // 5分のタイムアウト
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // Pong受信時にタイムアウトリセット
		return nil
	})

	// readPump と writePump を別々のゴルーチンで開始
	go sm.readPump(client)
	go client.writePump()

	// クライアント登録イベントを SessionManager に送信
	sm.register <- client

	log.Printf("[SessionManager] Client %s registered for room %s", userID, roomID)
	return nil
}

// readPump はクライアントからのWebSocketメッセージを読み込み、 inputEvents チャネルに送信します。
func (sm *SessionManager) readPump(client *Client) {
	defer func() {
		// パニック回復処理
		if r := recover(); r != nil {
			log.Printf("[SessionManager] Panic in readPump for user %s: %v", client.UserID, r)
		}
		
		// クライアントの切断処理
		log.Printf("[SessionManager] Client %s disconnecting from room %s", client.UserID, client.RoomID)
		sm.unregister <- client // クライアントが切断されたら登録解除を通知
		
		// WebSocket接続を安全に閉じる
		if client.Conn != nil {
			if err := client.Conn.Close(); err != nil {
				log.Printf("[SessionManager] Error closing WebSocket connection for user %s: %v", client.UserID, err)
			}
		}
	}()

	// WebSocket接続のタイムアウト設定を緩和
	client.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分に延長

	// Pongハンドラーを設定（ピングに対する応答でタイムアウトをリセット）
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
		return nil
	})

	// メッセージサイズ制限を設定
	client.Conn.SetReadLimit(1024) // 1KBに増加（パフォーマンス改善）

	for {
		// 接続状態チェック
		if client.Conn == nil {
			log.Printf("[SessionManager] WebSocket connection is nil for user %s", client.UserID)
			break
		}

		// メッセージタイプはテキストメッセージを想定
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			// より詳細なエラー分類とパニック防止
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("[SessionManager] WebSocket unexpected close error for user %s: %v", client.UserID, err)
			} else if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("[SessionManager] WebSocket normal close for user %s: %v", client.UserID, err)
			} else {
				log.Printf("[SessionManager] WebSocket read error for user %s: %v", client.UserID, err)
			}
			// 安全に終了
			return
		}
		
		// メッセージサイズチェック
		if len(message) == 0 {
			log.Printf("[SessionManager] Received empty message from user %s", client.UserID)
			continue
		}
		
		// ログ出力を削減（パフォーマンス改善）
		// log.Printf("[SessionManager] Received message from %s (Room %s): %s", client.UserID, client.RoomID, message)

		// 受信したJSONメッセージを PlayerInputEvent 構造体にパース
		var inputEvent PlayerInputEvent
		err = json.Unmarshal(message, &inputEvent)
		if err != nil {
			log.Printf("[SessionManager] Failed to unmarshal input message from %s: %v, message: %s", client.UserID, err, message)
			continue // パース失敗時はこのメッセージをスキップ
		}
		inputEvent.UserID = client.UserID // 受信したメッセージのUserIDを上書き（セキュリティのため）

		// プレイヤー入力を SessionManager の inputEvents チャネルに送信
		// チャネルがブロックされないように非同期で送信
		select {
		case sm.inputEvents <- inputEvent:
			// 正常に送信
		default:
			log.Printf("[SessionManager] Input events channel is full, dropping message from user %s", client.UserID)
		}
	}
}

// writePump は Client の Send チャネルからのメッセージをWebSocketコネクションに書き込みます。
// クライアントごとにこのゴルーチンが動作します。
func (c *Client) writePump() {
	defer func() {
		// パニック回復処理
		if r := recover(); r != nil {
			log.Printf("[Client] Panic in writePump for user %s: %v", c.UserID, r)
		}
		
		// WebSocket接続を安全に閉じる
		if c.Conn != nil {
			if err := c.Conn.Close(); err != nil {
				log.Printf("[Client] Error closing WebSocket connection for user %s: %v", c.UserID, err)
			}
		}
		log.Printf("[Client] WritePump ended for user %s", c.UserID)
	}()

	// WebSocket接続のタイムアウト設定を緩和
	c.Conn.SetWriteDeadline(time.Now().Add(60 * time.Second))

	// ピング送信のタイマー設定（頻度をさらに下げる）
	ticker := time.NewTicker(60 * time.Second) // 1分間隔に変更
	defer ticker.Stop()

	// 連続エラーカウンター
	consecutiveErrors := 0
	maxConsecutiveErrors := 3

	for {
		select {
		case message, ok := <-c.Send:
			// WebSocket書き込みタイムアウトを設定
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) // 短縮してレスポンシブに
			
			// Send チャネルからメッセージを受信
			if !ok {
				// マネージャーがチャネルを閉じた場合 (クライアントの登録解除時など)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// WebSocketにテキストメッセージとして書き込み
			err := c.Conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				consecutiveErrors++
				log.Printf("[Client] Error writing message for user %s (attempt %d/%d): %v", c.UserID, consecutiveErrors, maxConsecutiveErrors, err)
				
				if consecutiveErrors >= maxConsecutiveErrors {
					log.Printf("[Client] Too many consecutive errors for user %s, terminating connection", c.UserID)
					return
				}
				continue
			}
			
			// 送信成功時はエラーカウンターをリセット
			consecutiveErrors = 0
			
		case <-ticker.C:
			// ピングメッセージを定期的に送信してコネクションの生存確認
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[Client] Error sending ping for user %s: %v", c.UserID, err)
				return
			}
		}
	}
}

// BroadcastToSpecificClient は指定されたクライアントにのみゲーム状態を送信します（自分の操作の即座反映用）
//
// Parameters:
//   userID : 送信対象のユーザーID
//   roomID : ルームID
func (sm *SessionManager) BroadcastToSpecificClient(userID, roomID string) {
	sm.mu.RLock()
	session, ok := sm.sessions[roomID]
	if !ok {
		sm.mu.RUnlock()
		return
	}
	
	client, clientOk := sm.clients[userID]
	if !clientOk {
		sm.mu.RUnlock()
		return
	}

	// GameSessionを軽量な構造体に変換してからJSON形式でシリアライズ
	lightweightState := session.ToLightweight()
	stateJSON, err := json.Marshal(lightweightState)
	if err != nil {
		sm.mu.RUnlock()
		return
	}
	sm.mu.RUnlock()

	// 指定されたクライアントにのみ送信（安全な送信メソッドを使用）
	if !client.SafeSend(stateJSON) {
		log.Printf("[SessionManager] Failed to send to specific client %s (channel closed or full)", userID)
	}
}

// BroadcastGameState は指定された roomID のゲームセッションの現在の状態を、
// そのセッションに参加している全てのクライアントに WebSocket でブロードキャストします。
//
// Parameters:
//   roomID : ブロードキャスト対象のルームID
func (sm *SessionManager) BroadcastGameState(roomID string) {
	// ブロードキャストスロットリング：対戦相手の動きは1秒おきで十分
	const minBroadcastInterval = 1000 * time.Millisecond // 1秒間隔（大幅負荷軽減）
	
	sm.broadcastMu.Lock()
	lastTime, exists := sm.lastBroadcast[roomID]
	now := time.Now()
	
	// 前回のブロードキャストから十分な時間が経過していない場合はスキップ
	if exists && now.Sub(lastTime) < minBroadcastInterval {
		sm.broadcastMu.Unlock()
		return
	}
	
	sm.lastBroadcast[roomID] = now
	sm.broadcastMu.Unlock()
	
	// ログ出力を削減（パフォーマンス改善）
	// log.Printf("[SessionManager] BroadcastGameState called for room: %s", roomID)
	sm.mu.RLock()
	session, ok := sm.sessions[roomID]
	sm.mu.RUnlock()
	if !ok {
		log.Printf("[SessionManager] Attempted to broadcast for non-existent room: %s", roomID)
		return
	}
	// log.Printf("[SessionManager] Session found for room %s, status: %s", roomID, session.Status)

	// ゲーム状態更新イベントを SessionManager のブロードキャストチャネルに送信
	// チャネルがフルの場合は最新の状態のみ保持（負荷軽減）
	select {
	case sm.broadcast <- &GameStateEvent{
		RoomID: roomID,
		State:  session, // セッション全体の状態を送信
	}:
		// log.Printf("[SessionManager] Broadcast event sent to channel for room: %s", roomID)
	default:
		log.Printf("[SessionManager] Broadcast channel full, skipping update for room: %s", roomID)
	}
}

// EndGameSession はゲームセッションを終了させ、結果をデータベースに記録し、セッションをクリーンアップします。
//
// Parameters:
//   roomID : 終了するルームのID
func (sm *SessionManager) EndGameSession(roomID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[roomID]
	if !ok {
		log.Printf("[SessionManager] EndGameSession called for non-existent room: %s", roomID)
		return // ルームが存在しない
	}

	if session.Status == "finished" {
		log.Printf("[SessionManager] EndGameSession called for already finished room: %s", roomID)
		return // 既に終了済み
	}

	session.Status = "finished" // ステータスを「終了済み」に設定
	session.EndedAt = time.Now() // 終了日時を記録
	log.Printf("[SessionManager] Game session %s ended.", roomID)

	// ゲーム結果をデータベースに記録する (TODO: database/database_service.go に実装)
	// 例: sm.dbService.UpdateGameSessionResult(session)
	// 例: sm.dbService.SaveGameResults(session)

	// クライアントにゲーム終了を通知 (最後の状態をブロードキャスト)
	// mutexをアンロックしてからブロードキャスト（デッドロック回避）
	sm.mu.Unlock()
	sm.BroadcastGameState(roomID)
	sm.mu.Lock()

	// セッションに関連するクライアントのクリーンアップ
	var clientsToUnregister []*Client
	for userID, client := range sm.clients {
		if client.RoomID == roomID {
			clientsToUnregister = append(clientsToUnregister, client)
			log.Printf("[SessionManager] Marking client %s for cleanup from ended room %s", userID, roomID)
		}
	}

	// クライアントの実際のクリーンアップ
	for _, client := range clientsToUnregister {
		// Sendチャネルを安全に閉じる
		client.SafeClose()
		delete(sm.clients, client.UserID)
		log.Printf("[SessionManager] Cleaned up client %s from ended room %s", client.UserID, roomID)
	}

	// セッションマネージャーのマップからセッションを削除
	delete(sm.sessions, roomID)
	log.Printf("[SessionManager] Removed session %s from sessions map", roomID)
}

// GetGameSession は指定されたルームIDのゲームセッションを取得します。
// 主にハンドラーからセッション情報を取得するために使用されます。
func (sm *SessionManager) GetGameSession(roomID string) (*GameSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[roomID]
	return session, ok
}

// Shutdown はSessionManagerを安全にシャットダウンします
func (sm *SessionManager) Shutdown() {
	log.Printf("[SessionManager] シャットダウン開始...")
	
	// quitチャネルを閉じてRunメソッドのメインループを終了
	close(sm.quit)
	
	// 全クライアントを安全に切断
	sm.mu.Lock()
	for userID, client := range sm.clients {
		log.Printf("[SessionManager] クライアント %s を切断中...", userID)
		if client.Conn != nil {
			client.Conn.Close()
		}
		client.SafeClose()
	}
	// クライアントマップをクリア
	sm.clients = make(map[string]*Client)
	
	// セッションマップをクリア
	sm.sessions = make(map[string]*GameSession)
	sm.mu.Unlock()
	
	log.Printf("[SessionManager] シャットダウン完了")
} 