package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"   // Added for os.Getenv
	"time" // Added for time.Time

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5" // Added for JWT parsing
	"github.com/gorilla/websocket" // WebSocketライブラリ

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/tetris" // SessionManager をインポート
)

// upgrader はHTTP接続をWebSocketプロトコルにアップグレードするための設定です。
// CheckOrigin はクロスオリジンリクエストを許可するかどうかを制御します。
// 開発中は true で良いですが、本番環境では適切な Origin チェックを行うべきです。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// すべてのOriginからの接続を許可 (開発用)
		// 本番環境では、フロントエンドのドメインなどを厳密にチェックしてください。
		// 例: return r.Header.Get("Origin") == "https://yourfrontend.com"
		return true
	},
}

// GameHandler はゲーム関連のHTTPリクエスト（部屋作成、参加、WebSocket接続）を処理します。
type GameHandler struct {
	sessionManager *tetris.SessionManager // ゲームセッションの管理サービス
	dbService      *database.DatabaseService // データベースサービス
}

// NewGameHandler は新しい GameHandler インスタンスを作成します。
//
// Parameters:
//   sm : セッションマネージャーへのポインタ
//   db : データベースサービスへのポインタ
// Returns:
//   *GameHandler: 新しく作成された GameHandler のポインタ
func NewGameHandler(sm *tetris.SessionManager, db *database.DatabaseService) *GameHandler {
	return &GameHandler{
		sessionManager: sm,
		dbService:      db,
	}
}

// ExtractUserIDFromContext はリクエストのコンテキストからユーザーIDを抽出します。
func ExtractUserIDFromContext(r *http.Request) (string, error) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		return "", fmt.Errorf("ユーザーIDがコンテキストに見つかりません")
	}
	return userID, nil
}

// WriteErrorResponse はエラーレスポンスをJSON形式で書き込みます。
func WriteErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// WriteJSONResponse はJSONレスポンスを書き込みます。
func WriteJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// CreateRoom は新しいゲームセッション（部屋）を作成するためのHTTPハンドラーです。
// リクエストボディからデッキIDを取得し、セッションマネージャーに部屋の作成を依頼します。
func (h *GameHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	// ユーザー認証情報をコンテキストから取得する
	userID, err := ExtractUserIDFromContext(r) // api/handlers/auth_utils.go の関数を使用
	if err != nil {
		// 認証ミドルウェアが適用されていない場合、テスト用のユーザーIDを使用
		log.Printf("[GameHandler] No user ID in context, using test user ID")
		userID = "test-user-123"
	}

	// リクエストボディからプレイヤーのデッキIDを取得
	var req struct {
		DeckID string `json:"deck_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}
	if req.DeckID == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "デッキIDが必要です")
		return
	}

	// セッションマネージャーに新しいルームの作成を依頼
	roomID, err := h.sessionManager.CreateSession(userID, req.DeckID)
	if err != nil {
		log.Printf("[GameHandler] Failed to create room for user %s: %v", userID, err)
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("ルームの作成に失敗しました: %v", err))
		return
	}

	WriteJSONResponse(w, http.StatusCreated, map[string]string{"room_id": roomID, "message": "ルームを作成しました"})
}

// JoinRoom は既存のゲームセッション（部屋）に参加するためのHTTPハンドラーです。
// URLパラメータからroomIDを、リクエストボディからデッキIDを取得します。
func (h *GameHandler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	// ユーザー認証情報をコンテキストから取得する
	userID, err := ExtractUserIDFromContext(r) // api/handlers/auth_utils.go の関数を使用
	if err != nil {
		// 認証ミドルウェアが適用されていない場合、テスト用のユーザーIDを使用
		log.Printf("[GameHandler] No user ID in context, using test user ID")
		userID = "test-user-123"
	}

	roomID := chi.URLParam(r, "roomID") // URLパラメータからroomIDを取得
	if roomID == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "ルームIDが必要です")
		return
	}

	var req struct {
		DeckID string `json:"deck_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "リクエストボディのパースに失敗しました")
		return
	}
	if req.DeckID == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "デッキIDが必要です")
		return
	}

	// セッションマネージャーに既存のルームへの参加を依頼
	err = h.sessionManager.JoinSession(roomID, userID, req.DeckID)
	if err != nil {
		log.Printf("[GameHandler] User %s failed to join room %s: %v", userID, roomID, err)
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("ルームへの参加に失敗しました: %v", err))
		return
	}

	WriteJSONResponse(w, http.StatusOK, map[string]string{"message": "ルームに参加しました", "room_id": roomID})
}

// GetRoomStatus は特定のルームの現在の状態を返すハンドラーです。（デバッグやルーム一覧表示用）
func (h *GameHandler) GetRoomStatus(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "ルームIDが必要です")
		return
	}

	session, ok := h.sessionManager.GetGameSession(roomID)
	if !ok {
		WriteErrorResponse(w, http.StatusNotFound, "指定されたルームは見つかりませんでした")
		return
	}

	WriteJSONResponse(w, http.StatusOK, session)
}


// HandleWebSocketConnection はHTTP接続をWebSocketプロトコルにアップグレードし、
// その後、WebSocketメッセージの送受信をセッションマネージャーに引き渡します。
// このエンドポイントにはルームIDが含まれます。
func (h *GameHandler) HandleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "roomID")
	if roomID == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "WebSocket接続にはルームIDが必要です")
		return
	}

	// HTTP接続をWebSocket接続にアップグレード
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[GameHandler] Failed to upgrade to websocket for room %s: %v", roomID, err)
		return // アップグレード失敗時はエラーログのみ
	}
	// defer conn.Close() // ここでは閉じない。SessionManagerが管理するため。

	log.Printf("[GameHandler] WebSocket upgraded for room %s.", roomID)

	// 認証メッセージを待つ
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) // 10秒のタイムアウト
	
	var userID string
	authReceived := false
	
	// 認証メッセージを待つ
	for !authReceived {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[GameHandler] Failed to read auth message: %v", err)
			conn.Close()
			return
		}
		
		var authMsg struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		
		if err := json.Unmarshal(message, &authMsg); err != nil {
			log.Printf("[GameHandler] Failed to parse auth message: %v", err)
			conn.Close()
			return
		}
		
		if authMsg.Type == "auth" {
			// JWTトークンの検証（auth_middleware.goと同じロジック）
			if authMsg.Token == "BYPASS_AUTH" {
				userID = "test-user-123"
				log.Printf("[GameHandler] Using BYPASS_AUTH for user: %s", userID)
			} else {
				// JWT Secretを取得
				jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
				if jwtSecret == "" {
					log.Println("Error: SUPABASE_JWT_SECRET environment variable is not set.")
					conn.WriteJSON(map[string]string{"error": "Server configuration error: JWT secret missing"})
					conn.Close()
					return
				}

				// Bearerプレフィックスを除去
				tokenString := authMsg.Token
				if len(tokenString) > 7 && tokenString[0:7] == "Bearer " {
					tokenString = tokenString[7:]
				}

				// JWTの検証とパース
				parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
					// アルゴリズムがHMACであることを確認
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						log.Printf("WebSocket Auth Error: Unexpected signing method: %v", token.Header["alg"])
						return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
					}
					return []byte(jwtSecret), nil
				})

				if err != nil {
					log.Printf("WebSocket Auth Error: JWT parse error: %v", err)
					conn.WriteJSON(map[string]string{"error": "Invalid token"})
					conn.Close()
					return
				}

				if !parsedToken.Valid {
					log.Printf("WebSocket Auth Error: Invalid token")
					conn.WriteJSON(map[string]string{"error": "Invalid token"})
					conn.Close()
					return
				}

				// トークンのクレームを取得
				claims, ok := parsedToken.Claims.(jwt.MapClaims)
				if !ok {
					log.Printf("WebSocket Auth Error: Invalid token claims")
					conn.WriteJSON(map[string]string{"error": "Invalid token claims"})
					conn.Close()
					return
				}

				// SupabaseのJWTは通常、ユーザーIDを 'sub' (Subject) クレームにUUIDとして格納します。
				userID, ok = claims["sub"].(string)
				if !ok {
					log.Printf("WebSocket Auth Error: JWT claims missing 'sub' (userID) or wrong type: %v", claims["sub"])
					conn.WriteJSON(map[string]string{"error": "Invalid token: missing user ID"})
					conn.Close()
					return
				}
				
				log.Printf("[GameHandler] Successfully authenticated user via JWT: %s", userID)
			}
			
			authReceived = true
			// 認証成功レスポンスを送信
			conn.WriteJSON(map[string]string{"type": "auth_success", "message": "Authentication successful"})
		} else {
			log.Printf("[GameHandler] Unexpected message type: %s", authMsg.Type)
			conn.WriteJSON(map[string]string{"error": "Expected auth message"})
			conn.Close()
			return
		}
	}

	// タイムアウトを解除
	conn.SetReadDeadline(time.Time{})

	// SessionManager に新しいWebSocket接続を登録
	err = h.sessionManager.RegisterClient(roomID, userID, conn)
	if err != nil {
		log.Printf("[GameHandler] Failed to register client %s to room %s: %v", userID, roomID, err)
		conn.Close() // 登録失敗時はコネクションを閉じる
		return
	}

	// RegisterClient内で readPump と writePump ゴルーチンが開始されるため、
	// ここではそれ以上の処理は不要です。ハンドラーは単にコネクションを引き渡すだけです。
	// コネクションが閉じられるまで、このハンドラーは「ぶら下がる」ことになります。
}
