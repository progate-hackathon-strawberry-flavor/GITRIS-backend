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
	// 共通モデルをインポート (Deckなど)
	// テトリスゲーム固有のモデルをインポート
)

// Client はWebSocket接続を持つ単一のクライアントを表します。
type Client struct {
	UserID string          // このクライアントに紐づくユーザーのID
	Conn   *websocket.Conn // クライアントとの実際のWebSocketコネクション
	Send   chan []byte     // クライアントへメッセージを送信するためのバッファ付きチャネル
	RoomID string          // このクライアントが現在参加しているルームのID
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
	mu          sync.RWMutex                   // sessions と clients マップへのアクセスを保護するためのRWMutex
	dbService   *database.DatabaseService      // データベース操作のためのサービス
}

// NewSessionManager は新しい SessionManager インスタンスを作成し、そのメインイベントループをバックグラウンドで開始します。
//
// Parameters:
//   db : データベースサービスへのポインタ
// Returns:
//   *SessionManager: 初期化されたセッションマネージャーのポインタ
func NewSessionManager(db *database.DatabaseService) *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[string]*GameSession),
		clients:     make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *GameStateEvent, 256),   // ゲーム状態更新の頻度を考慮し、大きめのバッファ
		inputEvents: make(chan PlayerInputEvent, 256), // プレイヤー操作のキューイング用
		dbService:   db,
	}
	go sm.Run() // SessionManager のメインイベントループをゴルーチンで開始
	return sm
}

// Run は SessionManager のメインイベントループです。
// このゴルーチンは、クライアントの登録/解除、プレイヤー入力の処理、自動落下タイマーの管理、
// そしてゲーム状態のブロードキャストといったすべての主要なイベントを処理します。
func (sm *SessionManager) Run() {
	// 自動落下用のタイマー (初期間隔で設定)
	ticker := time.NewTicker(InitialFallInterval)
	defer ticker.Stop()

	for {
		select {
		case client := <-sm.register:
			// 新しいクライアントの登録処理
			sm.mu.Lock()
			sm.clients[client.UserID] = client
			sm.mu.Unlock()
			log.Printf("[SessionManager] Client registered: %s (Room: %s)", client.UserID, client.RoomID)

			// クライアント登録後、セッションが開始可能かチェックし、可能なら開始を試みる
			sm.checkAndStartGame(client.RoomID)

		case client := <-sm.unregister:
			// クライアントの登録解除処理
			sm.mu.Lock()
			if _, ok := sm.clients[client.UserID]; ok {
				delete(sm.clients, client.UserID)
				close(client.Send) // クライアントへの送信チャネルを閉じる
				log.Printf("[SessionManager] Client unregistered: %s", client.UserID)
			}
			sm.mu.Unlock()

			// プレイヤーがゲーム中に退出した場合、セッションを終了させる
			sm.mu.RLock()
			session, ok := sm.sessions[client.RoomID]
			sm.mu.RUnlock()
			if ok && session.Status == "playing" {
				log.Printf("[SessionManager] Player %s left room %s during game. Ending session.", client.UserID, client.RoomID)
				sm.EndGameSession(client.RoomID)
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
				// 状態変更があった場合のみブロードキャスト
				sm.BroadcastGameState(session.ID)

				// プレイヤーのゲームが終了したか判定
				if targetPlayerState.IsGameOver {
					sm.EndGameSession(session.ID)
				}
			}

		case <-ticker.C:
			// 自動落下処理を全プレイ中セッションで実行
			sm.mu.RLock()
			for _, session := range sm.sessions {
				if session.Status == "playing" {
					moved1 := false
					moved2 := false

					// プレイヤー1の自動落下
					if session.Player1 != nil && !session.Player1.IsGameOver {
						moved1 = AutoFall(session.Player1)
					}
					// プレイヤー2の自動落下
					if session.Player2 != nil && !session.Player2.IsGameOver {
						moved2 = AutoFall(session.Player2)
					}

					// どちらかのプレイヤーの状態が変更されたらブロードキャスト
					if moved1 || moved2 {
						sm.BroadcastGameState(session.ID)
					}

					// ゲームオーバー判定 (自動落下でゲームオーバーになることもありえる)
					if (session.Player1 != nil && session.Player1.IsGameOver) || (session.Player2 != nil && session.Player2.IsGameOver) {
						sm.EndGameSession(session.ID)
					}
					// TODO: レベルに応じたティック間隔の調整は、各PlayerGameState内で lastFallTime と現在のレベルを使用して AutoFall が行うべき。
					// この ticker は全体のループ周期を制御し、個別のピース落下は AutoFall 内で制御する。
				}
			}
			sm.mu.RUnlock()

		case event := <-sm.broadcast:
			// ゲーム状態のブロードキャスト処理
			sm.mu.RLock()
			_, ok := sm.sessions[event.RoomID]
			if !ok {
				sm.mu.RUnlock()
				log.Printf("[SessionManager] Attempted to broadcast for non-existent room: %s", event.RoomID)
				continue
			}

			// ゲームセッション全体をJSON形式でシリアライズ
			stateJSON, err := json.Marshal(event.State)
			if err != nil {
				log.Printf("[SessionManager] Error marshaling game state for room %s: %v", event.RoomID, err)
				sm.mu.RUnlock()
				continue
			}

			// ルーム内の各クライアントにゲーム状態を送信
			for _, client := range sm.clients {
				if client.RoomID == event.RoomID {
					// クライアントへの送信はノンブロッキングで（チャネルがフルでもブロックしない）
					select {
					case client.Send <- stateJSON:
						// 送信成功
					default:
						// チャネルがフルで送信できない場合は、クライアントを切断（再接続を促す）
						log.Printf("[SessionManager] Client %s send channel full, unregistering.", client.UserID)
						// クライアントを強制的に切断するための処理をunregisteredチャネルに送る
						sm.unregister <- client
					}
				}
			}
			sm.mu.RUnlock()
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	roomID := uuid.New().String() // 新しいルームIDを生成

	// データベースからプレイヤー1のデッキデータをロード (TODO: database/database_service.goに実装)
	player1Deck, err := sm.dbService.GetDeckByID(player1DeckID)
	if err != nil {
		log.Printf("[SessionManager] Failed to get player1 deck %s: %v", player1DeckID, err)
		return "", fmt.Errorf("failed to get player1 deck: %w", err)
	}

	// 新しいゲームセッションを初期化
	session := NewGameSession(roomID, player1ID, player1Deck)
	sm.sessions[roomID] = session // セッションマネージャーのマップに追加

	// データベースにセッションを記録 (game_sessions テーブル)
	// TODO: sm.dbService.CreateGameSession(session) を実装
	// err = sm.dbService.CreateGameSession(session)
	// if err != nil {
	// 	delete(sm.sessions, roomID) // DB記録失敗時はセッションを削除
	// 	return "", fmt.Errorf("failed to save game session to DB: %w", err)
	// }
	log.Printf("[SessionManager] Created new game session: %s for player %s", roomID, player1ID)
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[roomID]
	if !ok {
		return errors.New("room not found")
	}
	if session.Status != "waiting" {
		return errors.New("room is not waiting for players")
	}
	if session.Player2 != nil {
		return errors.New("room is already full") // 既に2人目が参加している
	}
	if session.Player1 != nil && session.Player1.UserID == player2ID {
		return errors.New("cannot join your own room") // 自分自身の部屋には参加できない
	}

	// データベースからプレイヤー2のデッキデータをロード (TODO: database/database_service.goに実装)
	player2Deck, err := sm.dbService.GetDeckByID(player2DeckID)
	if err != nil {
		log.Printf("[SessionManager] Failed to get player2 deck %s: %v", player2DeckID, err)
		return fmt.Errorf("failed to get player2 deck: %w", err)
	}

	session.SetPlayer2(player2ID, player2Deck) // プレイヤー2のゲーム状態を設定

	// データベースの game_participants テーブルを更新
	// TODO: sm.dbService.AddParticipantToSession(roomID, player2ID, player2DeckID) を実装
	log.Printf("[SessionManager] Player %s joined room %s", player2ID, roomID)

	return nil
}

// checkAndStartGame はセッションが開始条件を満たしているかチェックし、満たしていればゲームを開始します。
//
// Parameters:
//   roomID : チェックするルームのID
func (sm *SessionManager) checkAndStartGame(roomID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[roomID]
	if !ok {
		return // ルームが存在しない
	}

	// 2人のプレイヤーが揃っていて、両方がWebSocketに接続済みであればゲーム開始
	// TODO: このロジックはより複雑になる可能性あり（例: 両プレイヤーが「準備OK」ボタンを押す必要がある場合など）
	if session.Player1 != nil && session.Player2 != nil &&
		sm.clients[session.Player1.UserID] != nil && sm.clients[session.Player2.UserID] != nil &&
		session.Status == "waiting" {

		session.Status = "playing"
		session.StartedAt = time.Now()
		log.Printf("[SessionManager] Game session %s started! Players: %s vs %s", roomID, session.Player1.UserID, session.Player2.UserID)

		// データベースの game_sessions テーブルを更新
		// TODO: sm.dbService.UpdateGameSessionStatus(roomID, "playing", session.StartedAt) を実装
		sm.BroadcastGameState(roomID) // ゲーム開始をクライアントに通知
	}
}

// RegisterClient は新しいWebSocketクライアントを SessionManager に登録します。
// この関数は game_handler.go の HandleWebSocketConnection から呼び出されます。
//
// Parameters:
//   roomID : 接続しようとしているルームのID
//   userID : 接続しているユーザーのID
//   conn   : アップグレードされたWebSocketコネクション
// Returns:
//   error : エラーが発生した場合
func (sm *SessionManager) RegisterClient(roomID, userID string, conn *websocket.Conn) error {
	sm.mu.RLock()
	session, ok := sm.sessions[roomID]
	sm.mu.RUnlock()
	if !ok {
		return errors.New("room not found for WebSocket connection")
	}

	// このユーザーがルームの参加者であるか確認
	isPlayer := false
	if session.Player1 != nil && session.Player1.UserID == userID {
		isPlayer = true
	} else if session.Player2 != nil && session.Player2.UserID == userID {
		isPlayer = true
	}
	if !isPlayer {
		return errors.New("user is not a participant of this room")
	}

	client := &Client{UserID: userID, Conn: conn, Send: make(chan []byte, 256), RoomID: roomID}
	sm.register <- client // SessionManager のメインループにクライアント登録を通知

	// クライアントへのメッセージ送信を専用のゴルーチンで処理
	go client.writePump()

	// クライアントからのメッセージ受信を専用のゴルーチンで処理
	go sm.readPump(client)

	return nil
}

// readPump はクライアントからのWebSocketメッセージを読み込み、 inputEvents チャネルに送信します。
func (sm *SessionManager) readPump(client *Client) {
	defer func() {
		sm.unregister <- client // クライアントが切断されたら登録解除を通知
		client.Conn.Close()
	}()

	for {
		// メッセージタイプはテキストメッセージを想定
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[SessionManager] WebSocket read error for user %s: %v", client.UserID, err)
			}
			break // エラーが発生したらループを終了し、切断処理へ
		}
		log.Printf("[SessionManager] Received message from %s (Room %s): %s", client.UserID, client.RoomID, message)

		// 受信したJSONメッセージを PlayerInputEvent 構造体にパース
		var inputEvent PlayerInputEvent
		err = json.Unmarshal(message, &inputEvent)
		if err != nil {
			log.Printf("[SessionManager] Failed to unmarshal input message from %s: %v, message: %s", client.UserID, err, message)
			continue // パース失敗時はこのメッセージをスキップ
		}
		inputEvent.UserID = client.UserID // 受信したメッセージのUserIDを上書き（セキュリティのため）

		// プレイヤー入力を SessionManager の inputEvents チャネルに送信
		sm.inputEvents <- inputEvent
	}
}

// writePump は Client の Send チャネルからのメッセージをWebSocketコネクションに書き込みます。
// クライアントごとにこのゴルーチンが動作します。
func (c *Client) writePump() {
	defer func() {
		c.Conn.Close() // ゴルーチン終了時にコネクションを閉じる
	}()

	for {
		select {
		case message, ok := <-c.Send:
			// Send チャネルからメッセージを受信
			if !ok {
				// マネージャーがチャネルを閉じた場合 (クライアントの登録解除時など)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// WebSocketにテキストメッセージとして書き込むためのWriterを取得
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return // Writer取得に失敗したら終了
			}
			w.Write(message) // メッセージを書き込み

			// 送信チャネルにまだメッセージが残っていれば、それらも続けて書き込む (最適化)
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'}) // メッセージ間に改行を挟む
				w.Write(<-c.Send)
			}

			// Writerをクローズし、メッセージをフラッシュ
			if err := w.Close(); err != nil {
				return // クローズに失敗したら終了
			}
		}
	}
}

// BroadcastGameState は指定された roomID のゲームセッションの現在の状態を、
// そのセッションに参加している全てのクライアントに WebSocket でブロードキャストします。
//
// Parameters:
//   roomID : ブロードキャスト対象のルームID
func (sm *SessionManager) BroadcastGameState(roomID string) {
	sm.mu.RLock()
	session, ok := sm.sessions[roomID]
	sm.mu.RUnlock()
	if !ok {
		log.Printf("[SessionManager] Attempted to broadcast for non-existent room: %s", roomID)
		return
	}

	// ゲーム状態更新イベントを SessionManager のブロードキャストチャネルに送信
	sm.broadcast <- &GameStateEvent{
		RoomID: roomID,
		State:  session, // セッション全体の状態を送信
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
		return // ルームが存在しない
	}

	if session.Status == "finished" {
		return // 既に終了済み
	}

	session.Status = "finished" // ステータスを「終了済み」に設定
	session.EndedAt = time.Now() // 終了日時を記録
	log.Printf("[SessionManager] Game session %s ended.", roomID)

	// ゲーム結果をデータベースに記録する (TODO: database/database_service.go に実装)
	// 例: sm.dbService.UpdateGameSessionResult(session)
	// 例: sm.dbService.SaveGameResults(session)

	// クライアントにゲーム終了を通知 (最後の状態をブロードキャスト)
	sm.BroadcastGameState(roomID)

	// セッションに関連するクライアントのクリーンアップ（Sendチャネルを閉じ、unregisterチャネルに送る）
	for _, client := range sm.clients {
		if client.RoomID == roomID {
			// クライアントのgoroutineが終了するようにSendチャネルを閉じる
			// writePumpが終了し、defer unregisterが呼ばれる
			close(client.Send)
			// ただし、unregisterチャネルに直接送ることで即時クリーンアップも可能
			// sm.unregister <- client
		}
	}
	// セッションマネージャーのマップからセッションを削除
	delete(sm.sessions, roomID)
}

// GetGameSession は指定されたルームIDのゲームセッションを取得します。
// 主にハンドラーからセッション情報を取得するために使用されます。
func (sm *SessionManager) GetGameSession(roomID string) (*GameSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[roomID]
	return session, ok
} 