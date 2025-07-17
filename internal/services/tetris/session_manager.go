package tetris

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket" // WebSocketライブラリのインポート

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database" // データベースサービスをインポート
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
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
	ID             string                    `json:"id"`
	Player1        *LightweightPlayerState   `json:"player1"`
	Player2        *LightweightPlayerState   `json:"player2"`
	Status         string                    `json:"status"`
	StartedAt      time.Time                 `json:"started_at,omitempty"`
	EndedAt        time.Time                 `json:"ended_at,omitempty"`
	TimeLimit      int                       `json:"time_limit"`       // 制限時間（秒）
	RemainingTime  int                       `json:"remaining_time"`   // 残り時間（秒）
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
	sessions    map[string]*GameSession // 合言葉 -> GameSession のマップ (アクティブなゲームセッションを保持)
	clients     map[string]*Client             // userID -> Client のマップ (現在接続中の全WebSocketクライアント)
	register    chan *Client                   // 新しいクライアント接続の登録リクエスト用チャネル
	unregister  chan *Client                   // クライアント切断の登録解除リクエスト用チャネル
	broadcast   chan *GameStateEvent          // ゲーム状態の更新をブロードキャストするためのチャネル
	inputEvents chan PlayerInputEvent         // クライアントからのプレイヤー操作入力を受け取るチャネル
	quit        chan struct{}                  // シャットダウン用チャネル
	mu          sync.RWMutex                   // sessions と clients マップへのアクセスを保護するためのRWMutex
	dbService   *database.DatabaseService      // データベース操作のためのサービス
	deckRepo    database.DeckRepository        // デッキリポジトリ（テトリミノ配置データ取得用）
	resultRepo database.ResultRepository       // ゲーム結果リポジトリ（スコア保存用）
	lastBroadcast map[string]time.Time          // ルームごとの最後のブロードキャスト時刻
	broadcastMu   sync.Mutex                    // lastBroadcastマップへのアクセス保護用
}

// NewSessionManager は新しい SessionManager インスタンスを作成し、そのメインイベントループをバックグラウンドで開始します。
//
// Parameters:
//   db : データベースサービスへのポインタ
//   deckRepo : デッキリポジトリ
//   resultRepo : ゲーム結果リポジトリ
// Returns:
//   *SessionManager: 初期化されたセッションマネージャーのポインタ
func NewSessionManager(db *database.DatabaseService, deckRepo database.DeckRepository, resultRepo database.ResultRepository) *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[string]*GameSession),
		clients:     make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *GameStateEvent, 512),   // ゲーム状態更新の頻度を考慮し、大きめのバッファ
		inputEvents: make(chan PlayerInputEvent, 512), // プレイヤー操作のキューイング用
		quit:        make(chan struct{}),
		dbService:  db,
		deckRepo:   deckRepo,
		resultRepo: resultRepo,
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

			// クライアント登録後に最新の状態をブロードキャスト（非同期実行）
			go func(passcode string) {
				sm.BroadcastGameState(passcode)
			}(client.RoomID)

			// クライアント登録後、セッションが開始可能かチェック（非同期実行、少し遅延させてレースコンディション回避）
			go func(passcode string) {
				time.Sleep(50 * time.Millisecond) // 50ms遅延でレースコンディション回避
				sm.CheckAndStartGame(passcode)
			}(client.RoomID)

		case client := <-sm.unregister:
			// クライアントの登録解除処理
			sm.mu.Lock()
			if registeredClient, ok := sm.clients[client.UserID]; ok {
				// 同じクライアントインスタンスの場合のみ登録解除（重複解除防止）
				if registeredClient == client {
					// Sendチャネルを安全に閉じる
					registeredClient.SafeClose()
					delete(sm.clients, client.UserID)
				} else {
				}
			} else {
			}
			sm.mu.Unlock()

			// プレイヤーがゲーム中に退出した場合、セッションを終了させる
			sm.mu.RLock()
			session, ok := sm.sessions[client.RoomID]
			sm.mu.RUnlock()
			if ok && session.Status == "playing" {
				sm.EndGameSession(client.RoomID)
			} else if ok {
				// ゲーム中でない場合は、セッション状態を更新してブロードキャスト
				sm.BroadcastGameState(client.RoomID)
			}

		case event := <-sm.inputEvents:
			// プレイヤーからの入力イベントを処理
			// クライアントの合言葉を取得
			sm.mu.RLock()
			client, clientExists := sm.clients[event.UserID]
			sm.mu.RUnlock()
			
			if !clientExists {
				continue
			}
			
			sm.mu.RLock()
			session, ok := sm.sessions[client.RoomID]
			sm.mu.RUnlock()

			if !ok || session.Status != "playing" {
				continue // 存在しないか、プレイ中でない合言葉への入力は無視
			}

			// どちらのプレイヤーからの入力か判定し、対応するゲーム状態を更新
			var targetPlayerState *PlayerGameState
			if session.Player1 != nil && session.Player1.UserID == event.UserID {
				targetPlayerState = session.Player1
			} else if session.Player2 != nil && session.Player2.UserID == event.UserID {
				targetPlayerState = session.Player2
			} else {
				continue
			}

			// ゲームオーバーしたプレイヤーの操作は無視
			if targetPlayerState.IsGameOver {
				continue
			}

			// ゲームロジックを適用し、状態が実際に変更されたか確認
			if ApplyPlayerInput(targetPlayerState, event.Action) {
				// 自分の操作は即座に自分にだけ送信（レスポンシブ感を維持）
				go func(userID, passcode string) {
					sm.BroadcastToSpecificClient(userID, passcode)
				}(event.UserID, session.ID)
				
				// 相手への更新は1秒間隔のブロードキャストに任せる（負荷軽減）
				// （自動落下タイマーでブロードキャストされるため、ここでは相手への送信は不要）

				// プレイヤーのゲームが終了したか判定（ゲームオーバーは即座に通知）
				if targetPlayerState.IsGameOver {
					// ゲームオーバーは重要なので即座にブロードキャスト
					go func(passcode string) {
						sm.BroadcastGameState(passcode)
					}(session.ID)
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
				// 時間制限チェック（100秒）
				if session.IsTimeUp() {
					sm.EndGameSession(session.ID)
					continue // 時間切れのセッションは処理をスキップ
				}

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

				// ゲームオーバー判定 - 両方のプレイヤーがゲームオーバーした場合のみ終了
				if session.Player1 != nil && session.Player2 != nil && 
				   session.Player1.IsGameOver && session.Player2.IsGameOver {
					// 両プレイヤーがゲームオーバーした場合のみセッション終了
					go func(sessionID string) {
						time.Sleep(2 * time.Second)
						sm.EndGameSession(sessionID)
					}(session.ID)
				}
			}

		case event := <-sm.broadcast:
			// ゲーム状態のブロードキャスト処理
			sm.mu.RLock()
			session, ok := sm.sessions[event.RoomID]
			if !ok {
				sm.mu.RUnlock()
				continue
			}

			// GameSessionを軽量な構造体に変換してからJSON形式でシリアライズ
			lightweightState := session.ToLightweight()
			stateJSON, err := json.Marshal(lightweightState)
			if err != nil {
				sm.mu.RUnlock()
				continue
			}

			// ルーム内の各クライアントにゲーム状態を送信
			for _, client := range sm.clients {
				if client.RoomID == event.RoomID {
					// 安全な送信メソッドを使用
					if !client.SafeSend(stateJSON) {
					}
				}
			}
			sm.mu.RUnlock()
		
		case <-sm.quit:
			// シャットダウンシグナルを受信したらメインループを終了
			return
		}
	}
}

// CheckAndStartGame はセッションが開始条件を満たしているかチェックし、満たしていればゲームを開始します。
//
// Parameters:
//   passcode : チェックする合言葉
func (sm *SessionManager) CheckAndStartGame(passcode string) {
	
	sm.mu.Lock()
	defer sm.mu.Unlock() // defer で必ずアンロックされるように変更


	
	session, ok := sm.sessions[passcode]
	if !ok {
		// デバッグ用: 存在するセッションパスコードをログ出力
		var existingPasscodes []string
		for code := range sm.sessions {
			existingPasscodes = append(existingPasscodes, code)
		}
		return // セッションが存在しない
	}
	
	// セッションの状態をチェック（削除された可能性を考慮）
	if session == nil {
		return
	}
	
	
	// 各条件をチェック
	hasPlayer1 := session.Player1 != nil
	hasPlayer2 := session.Player2 != nil
	
	
	if hasPlayer1 {
	}
	if hasPlayer2 {
	}
	
	// WebSocket接続をチェック
	var player1Connected, player2Connected bool
	if hasPlayer1 {
		player1Connected = sm.clients[session.Player1.UserID] != nil
	}
	if hasPlayer2 {
		player2Connected = sm.clients[session.Player2.UserID] != nil
	}
	
	isWaiting := session.Status == "waiting"

	// 2人のプレイヤーが揃っていて、両方がWebSocketに接続済みであればゲーム開始
	if hasPlayer1 && hasPlayer2 && player1Connected && player2Connected && isWaiting {
		
		session.Status = "playing"
		session.StartedAt = time.Now()

		// ゲーム開始をクライアントに通知（非同期実行）
		go func(passcode string) {
			sm.BroadcastGameState(passcode) 
		}(passcode)
		return
	}
}

// RegisterClient は新しいWebSocketクライアントをSessionManagerに登録します。
//
// Parameters:
//   passcode : クライアントが参加する合言葉
//   userID : クライアントのユーザーID
//   conn   : WebSocketコネクション
// Returns:
//   error: エラーが発生した場合
func (sm *SessionManager) RegisterClient(passcode, userID string, conn *websocket.Conn) error {

	// 既存の接続があれば状況に応じてクリーンアップ
	sm.mu.Lock()
	if existingClient, exists := sm.clients[userID]; exists {
		// 同一ユーザーの複数接続許可が有効な場合は、既存接続を保持
		if os.Getenv("ALLOW_SAME_USER_JOIN") == "true" {
		} else {
			if existingClient.Conn != nil {
				existingClient.Conn.Close()
			}
			// 安全なチャネル閉じ方を使用
			existingClient.SafeClose()
			delete(sm.clients, userID) // 明示的に削除
		}
	}

	// 新しいクライアントを作成
	client := &Client{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 512), // バッファサイズをさらに増加
		RoomID: passcode, // 合言葉をRoomIDフィールドに格納
	}
	
	// 同一ユーザーの複数接続許可が有効な場合は、常に新しい接続を登録
	// （既存接続は上の処理で保持されている）
	if os.Getenv("ALLOW_SAME_USER_JOIN") == "true" {
		sm.clients[userID] = client
	} else {
		// 通常モード：既存接続がない場合のみ登録
		if _, exists := sm.clients[userID]; !exists {
			sm.clients[userID] = client
		} else {
			sm.clients[userID] = client
		}
	}
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

	return nil
}

// readPump はクライアントからのWebSocketメッセージを読み込み、 inputEvents チャネルに送信します。
func (sm *SessionManager) readPump(client *Client) {
	defer func() {
		// パニック回復処理
		if r := recover(); r != nil {
		}
		
		// クライアントの切断処理（unregisterのみ実行、コネクション切断はwritePumpで処理）
		
		// unregister チャネルが閉じられていない場合のみ送信
		select {
		case sm.unregister <- client:
			// 正常に登録解除リクエストを送信
		default:
			// unregisterチャネルがフルまたは閉じられている場合
		}
	}()

	// WebSocket接続のタイムアウト設定を緩和
	if client.Conn != nil {
		client.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5分に延長

		// Pongハンドラーを設定（ピングに対する応答でタイムアウトをリセット）
		client.Conn.SetPongHandler(func(string) error {
			client.Conn.SetReadDeadline(time.Now().Add(300 * time.Second))
			return nil
		})

		// メッセージサイズ制限を設定
		client.Conn.SetReadLimit(1024) // 1KBに増加（パフォーマンス改善）
	}

	for {
		// 接続状態チェック
		if client.Conn == nil {
			break
		}

		// メッセージタイプはテキストメッセージを想定
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			// より詳細なエラー分類とパニック防止
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
			} else if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
			} else {
			}
			// 安全に終了（コネクション切断はwritePumpに任せる）
			return
		}
		
		// メッセージサイズチェック
		if len(message) == 0 {
			continue
		}
		
		// ログ出力を削減（パフォーマンス改善）

		// 受信したJSONメッセージを PlayerInputEvent 構造体にパース
		var inputEvent PlayerInputEvent
		err = json.Unmarshal(message, &inputEvent)
		if err != nil {
			continue // パース失敗時はこのメッセージをスキップ
		}
		inputEvent.UserID = client.UserID // 受信したメッセージのUserIDを上書き（セキュリティのため）

		// プレイヤー入力を SessionManager の inputEvents チャネルに送信
		// チャネルがブロックされないように非同期で送信
		select {
		case sm.inputEvents <- inputEvent:
			// 正常に送信
		default:
		}
	}
}

// writePump は Client の Send チャネルからのメッセージをWebSocketコネクションに書き込みます。
// クライアントごとにこのゴルーチンが動作します。
func (c *Client) writePump() {
	defer func() {
		// パニック回復処理
		if r := recover(); r != nil {
		}
		
		// WebSocket接続を安全に閉じる（一度だけ実行されるように）
		if c.Conn != nil {
			if err := c.Conn.Close(); err != nil {
				// 既に閉じられている場合のエラーは無視
				if err.Error() != "use of closed network connection" {
				}
			}
			c.Conn = nil // 重複切断を防ぐ
		}
	}()

	// WebSocket接続のタイムアウト設定を緩和
	if c.Conn != nil {
		c.Conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
	}

	// ピング送信のタイマー設定（頻度をさらに下げる）
	ticker := time.NewTicker(60 * time.Second) // 1分間隔に変更
	defer ticker.Stop()

	// 連続エラーカウンター
	consecutiveErrors := 0
	maxConsecutiveErrors := 3

	for {
		select {
		case message, ok := <-c.Send:
			// 接続状態チェック
			if c.Conn == nil {
				return
			}

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
				
				if consecutiveErrors >= maxConsecutiveErrors {
					return
				}
				continue
			}
			
			// 送信成功時はエラーカウンターをリセット
			consecutiveErrors = 0
			
		case <-ticker.C:
			// 接続状態チェック
			if c.Conn == nil {
				return
			}

			// ピングメッセージを定期的に送信してコネクションの生存確認
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// BroadcastToSpecificClient は指定されたクライアントにのみゲーム状態を送信します（自分の操作の即座反映用）
//
// Parameters:
//   userID : 送信対象のユーザーID
//   passcode : 合言葉
func (sm *SessionManager) BroadcastToSpecificClient(userID, passcode string) {
	sm.mu.RLock()
	session, ok := sm.sessions[passcode]
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
	}
}

// BroadcastGameState は指定された passcode のゲームセッションの現在の状態を、
// そのセッションに参加している全てのクライアントに WebSocket でブロードキャストします。
//
// Parameters:
//   passcode : ブロードキャスト対象の合言葉
func (sm *SessionManager) BroadcastGameState(passcode string) {
	// ブロードキャストスロットリング：対戦相手の動きは1秒おきで十分
	const minBroadcastInterval = 1000 * time.Millisecond // 1秒間隔（大幅負荷軽減）
	
	sm.broadcastMu.Lock()
	lastTime, exists := sm.lastBroadcast[passcode]
	now := time.Now()
	
	// 前回のブロードキャストから十分な時間が経過していない場合はスキップ
	if exists && now.Sub(lastTime) < minBroadcastInterval {
		sm.broadcastMu.Unlock()
		return
	}
	
	sm.lastBroadcast[passcode] = now
	sm.broadcastMu.Unlock()
	
	// ログ出力を削減（パフォーマンス改善）
	sm.mu.RLock()
	session, ok := sm.sessions[passcode]
	sm.mu.RUnlock()
	if !ok {
		return
	}

	// ゲーム状態更新イベントを SessionManager のブロードキャストチャネルに送信
	// チャネルがフルの場合は最新の状態のみ保持（負荷軽減）
	select {
	case sm.broadcast <- &GameStateEvent{
		RoomID: passcode, // 合言葉を使用
		State:  session, // セッション全体の状態を送信
	}:
	default:
	}
}

// EndGameSession はゲームセッションを終了させ、結果をデータベースに記録し、セッションをクリーンアップします。
//
// Parameters:
//   passcode : 終了する合言葉
func (sm *SessionManager) EndGameSession(passcode string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[passcode]
	if !ok {
		return // 合言葉が存在しない
	}

	if session.Status == "finished" {
		return // 既に終了済み
	}

	session.Status = "finished" // ステータスを「終了済み」に設定
	session.EndedAt = time.Now() // 終了日時を記録
	
	// 終了理由を判定してログ出力
	if session.IsTimeUp() {
	} else if session.Player1 != nil && session.Player1.IsGameOver {
	} else if session.Player2 != nil && session.Player2.IsGameOver {
	} else {
	}

	// ゲーム結果をランキングデータベースに記録する
	sm.saveGameResultsToRanking(session)

	// クライアントにゲーム終了を通知 (最後の状態をブロードキャスト)
	// mutexをアンロックしてからブロードキャスト（デッドロック回避）
	sm.mu.Unlock()
	sm.BroadcastGameState(passcode)
	
	// ゲーム終了の通知をクライアントが受信する時間を確保（3秒待機）
	time.Sleep(3 * time.Second)
	
	sm.mu.Lock()

	// セッションに関連するクライアントのクリーンアップ
	var clientsToUnregister []*Client
	for _, client := range sm.clients {
		if client.RoomID == passcode {
			clientsToUnregister = append(clientsToUnregister, client)
		}
	}

	// クライアントの実際のクリーンアップ
	for _, client := range clientsToUnregister {
		// Sendチャネルを安全に閉じる
		client.SafeClose()
		delete(sm.clients, client.UserID)
	}

	// セッションマネージャーのマップからセッションを削除
	delete(sm.sessions, passcode)
}

// GetGameSession は指定された合言葉のゲームセッションを取得します。
// セッションが存在しない場合は nil と false を返します。
func (sm *SessionManager) GetGameSession(passcode string) (*GameSession, bool) {
	sm.mu.RLock()
	session, ok := sm.sessions[passcode]
	sm.mu.RUnlock()
	return session, ok
}

// DeleteSession は指定された合言葉のセッションを削除します。
func (sm *SessionManager) DeleteSession(passcode string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[passcode]
	if !exists {
		return fmt.Errorf("passcode %s のセッションは見つかりませんでした", passcode)
	}
	
	// セッションに接続されているクライアントをすべて切断
	if session.Player1 != nil {
		if client, ok := sm.clients[session.Player1.UserID]; ok {
			client.SafeClose()
			delete(sm.clients, session.Player1.UserID)
		}
	}
	
	if session.Player2 != nil {
		if client, ok := sm.clients[session.Player2.UserID]; ok {
			client.SafeClose()
			delete(sm.clients, session.Player2.UserID)
		}
	}
	
	// セッションをマップから削除
	delete(sm.sessions, passcode)
	
	return nil
}

// Shutdown はSessionManagerを安全にシャットダウンします
func (sm *SessionManager) Shutdown() {
	
	// quitチャネルを閉じてRunメソッドのメインループを終了
	close(sm.quit)
	
	// 全クライアントを安全に切断
	sm.mu.Lock()
	for _, client := range sm.clients {
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
	
} 

// saveGameResultsToRanking はゲーム終了時に両プレイヤーのスコアをresultsテーブルに保存します
func (sm *SessionManager) saveGameResultsToRanking(session *GameSession) {
	if session == nil {
		return
	}


	// プレイヤー1のスコアを保存
	if session.Player1 != nil {
		err := sm.savePlayerScore(session.Player1.UserID, session.Player1.Score, "Player1")
		if err != nil {
		}
	}

	// プレイヤー2のスコアを保存
	if session.Player2 != nil {
		err := sm.savePlayerScore(session.Player2.UserID, session.Player2.Score, "Player2")
		if err != nil {
		}
	}
}

// savePlayerScore は個別のプレイヤーのスコアを保存します（result_handlerのロジックを使用）
func (sm *SessionManager) savePlayerScore(userID string, score int, playerName string) error {
	// result_handlerと同じバリデーション
	if userID == "" {
		return fmt.Errorf("user_idは必須です")
	}
	if score < 0 {
		return fmt.Errorf("スコアは0以上である必要があります")
	}

	// resultsテーブルに保存
	_, err := sm.resultRepo.CreateResult(nil, userID, score)
	if err != nil {
		return fmt.Errorf("スコア保存に失敗しました: %w", err)
	}
	return nil
}

// JoinRoomByPasscode は合言葉を使ってルームに参加します。
// 合言葉のセッションが存在しない場合は新しく作成し、存在する場合は参加します。
//
// Parameters:
//   passcode     : ユーザーが入力した合言葉
//   playerID     : 参加するプレイヤーのユーザーID
//   playerDeckID : プレイヤーが使用するデッキのUUID
// Returns:
//   string: セッションID（合言葉と同じ）
//   bool: 新しくセッションを作成したかどうか（true: 作成、false: 既存セッションに参加）
//   error: エラーが発生した場合
func (sm *SessionManager) JoinRoomByPasscode(passcode, playerID, playerDeckID string) (string, bool, error) {
	
	// 合言葉のバリデーション
	if passcode == "" {
		return "", false, errors.New("合言葉が必要です")
	}
	if len(passcode) < 3 || len(passcode) > 20 {
		return "", false, errors.New("合言葉は3文字以上20文字以下で入力してください")
	}
	
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[passcode]
	
	if !exists {
		// セッションが存在しない場合、新しく作成（プレイヤー1として）
		
			// ゲストユーザーの場合はnilデッキを設定
	var playerDeck *models.Deck
	if playerDeckID == "guest" || playerDeckID == "" {
		playerDeck = nil
	} else {
		// データベースからプレイヤーのデッキデータをロード
		var err error
		playerDeck, err = sm.dbService.GetDeckByID(playerDeckID)
		if err != nil {
			return "", false, fmt.Errorf("failed to get player deck: %w", err)
		}
	}
		
		// 新しいゲームセッションを初期化（IDは合言葉を使用）
		newSession, err := NewGameSession(passcode, playerID, playerDeck, sm.deckRepo)
		if err != nil {
			return "", false, fmt.Errorf("failed to create game session: %w", err)
		}
		sm.sessions[passcode] = newSession
		
		return passcode, true, nil
		
	} else {
		// セッションが存在する場合、プレイヤー2として参加
		
		if session.Status != "waiting" {
			return "", false, errors.New("このルームは既にゲーム中または終了しています")
		}
		
		if session.Player2 != nil {
			return "", false, errors.New("このルームは既に満室です")
		}
		
		// 開発・テスト用: 環境変数でこの制限を無効化可能
		if os.Getenv("ALLOW_SAME_USER_JOIN") != "true" {
			if session.Player1 != nil && session.Player1.UserID == playerID {
				return "", false, errors.New("自分が作成したルームには参加できません")
			}
		} else {
		}

	
	// ゲストユーザーの場合はnilデッキを設定
	var playerDeck *models.Deck
	if playerDeckID == "guest" || playerDeckID == "" {
		playerDeck = nil
	} else {
		// データベースからプレイヤー2のデッキデータをロード
		var err error
		playerDeck, err = sm.dbService.GetDeckByID(playerDeckID)
		if err != nil {
			return "", false, fmt.Errorf("failed to get player2 deck: %w", err)
		}
	}

		session.SetPlayer2(playerID, playerDeck, sm.deckRepo)

		return passcode, false, nil
	}
}

// IsUserConnected は指定されたユーザーIDが現在接続中かどうかを確認します。
func (sm *SessionManager) IsUserConnected(userID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	_, connected := sm.clients[userID]
	return connected
} 