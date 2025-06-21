package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"   // Added for os.Getenv
	"time" // Added for time.Time

	"github.com/golang-jwt/jwt/v5" // Added for JWT parsing
	"github.com/gorilla/mux"       // gorilla/muxをインポート
	"github.com/gorilla/websocket" // WebSocketライブラリ

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/tetris" // SessionManager をインポート
)

// upgrader はHTTP接続をWebSocketプロトコルにアップグレードするための設定です。
// CheckOrigin はクロスオリジンリクエストを許可するかどうかを制御します。
// 開発中は true で良いですが、本番環境では適切な Origin チェックを行うべきです。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,  // 読み取りバッファを4KBに増加
	WriteBufferSize: 4096,  // 書き込みバッファを4KBに増加
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



// GetRoomStatus は特定の合言葉のセッションの現在の状態を返すハンドラーです。（デバッグやセッション一覧表示用）
func (h *GameHandler) GetRoomStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	passcode := vars["passcode"] // 合言葉をURLパラメータから取得
	if passcode == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "合言葉が必要です")
		return
	}

	session, ok := h.sessionManager.GetGameSession(passcode)
	if !ok {
		WriteErrorResponse(w, http.StatusNotFound, "指定された合言葉のセッションは見つかりませんでした")
		return
	}

	WriteJSONResponse(w, http.StatusOK, session)
}


// HandleWebSocketConnection はHTTP接続をWebSocketプロトコルにアップグレードし、
// その後、WebSocketメッセージの送受信をセッションマネージャーに引き渡します。
// このエンドポイントには合言葉が含まれます。
func (h *GameHandler) HandleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	log.Printf("[GameHandler] WebSocket connection attempt for path: %s", r.URL.Path)
	
	vars := mux.Vars(r)
	log.Printf("[GameHandler] mux.Vars result: %+v", vars)
	passcode := vars["passcode"] // 合言葉をURLパラメータから取得
	log.Printf("[GameHandler] Extracted passcode: '%s'", passcode)
	
	if passcode == "" {
		log.Printf("[GameHandler] Missing passcode in WebSocket connection")
		WriteErrorResponse(w, http.StatusBadRequest, "WebSocket接続には合言葉が必要です")
		return
	}

	// 合言葉のセッションが存在するかどうかを確認
	session, exists := h.sessionManager.GetGameSession(passcode)
	if !exists {
		log.Printf("[GameHandler] Passcode %s does not exist", passcode)
		WriteErrorResponse(w, http.StatusNotFound, "指定された合言葉のセッションは存在しません")
		return
	}
	log.Printf("[GameHandler] Passcode %s exists, status: %s", passcode, session.Status)

	log.Printf("[GameHandler] Attempting to upgrade connection for passcode: %s", passcode)

	// HTTP接続をWebSocket接続にアップグレード
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[GameHandler] Failed to upgrade to websocket for passcode %s: %v", passcode, err)
		return // アップグレード失敗時はエラーログのみ
	}
	// defer conn.Close() // ここでは閉じない。SessionManagerが管理するため。

	log.Printf("[GameHandler] WebSocket upgraded successfully for passcode %s.", passcode)

	// 認証メッセージを待つ
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) // 10秒のタイムアウト
	log.Printf("[GameHandler] Waiting for auth message from client...")
	
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
		
		log.Printf("[GameHandler] Received message: %s", string(message))
		
		var authMsg struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		
		if err := json.Unmarshal(message, &authMsg); err != nil {
			log.Printf("[GameHandler] Failed to parse auth message: %v", err)
			conn.Close()
			return
		}
		
		log.Printf("[GameHandler] Parsed auth message - Type: %s, Token length: %d", authMsg.Type, len(authMsg.Token))
		
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
			log.Printf("[GameHandler] Sending auth success response to client")
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
	log.Printf("[GameHandler] Auth completed, registering client %s to passcode %s", userID, passcode)

	// SessionManager に新しいWebSocket接続を登録
	err = h.sessionManager.RegisterClient(passcode, userID, conn)
	if err != nil {
		log.Printf("[GameHandler] Failed to register client %s to passcode %s: %v", userID, passcode, err)
		conn.Close() // 登録失敗時はコネクションを閉じる
		return
	}

	log.Printf("[GameHandler] Successfully registered client %s to passcode %s", userID, passcode)
	
	// ゲーム開始条件のチェックはSessionManager.Register内で自動実行されるため、ここでは不要
	log.Printf("[GameHandler] Client registration completed for passcode %s", passcode)

	// RegisterClient内で readPump と writePump ゴルーチンが開始されるため、
	// ここではそれ以上の処理は不要です。ハンドラーは単にコネクションを引き渡すだけです。
	// コネクションが閉じられるまで、このハンドラーは「ぶら下がる」ことになります。
}

// JoinRoomByPasscode は合言葉を使ってルームに参加するHTTPハンドラーです。
// URLパラメータから合言葉を、リクエストボディからデッキIDを取得し、
// セッションマネージャーに合言葉でのマッチングを依頼します。
func (h *GameHandler) JoinRoomByPasscode(w http.ResponseWriter, r *http.Request) {
	log.Printf("[GameHandler] JoinRoomByPasscode called")
	
	// ユーザー認証情報をコンテキストから取得する
	userID, err := ExtractUserIDFromContext(r)
	if err != nil {
		log.Printf("[GameHandler] Failed to extract user ID for passcode join: %v", err)
		WriteErrorResponse(w, http.StatusUnauthorized, "認証情報が必要です")
		return
	}
	log.Printf("[GameHandler] User ID extracted for passcode join: %s", userID)

	vars := mux.Vars(r)
	passcode := vars["passcode"] // 合言葉をURLパラメータから取得
	if passcode == "" {
		log.Printf("[GameHandler] Missing passcode in join request")
		WriteErrorResponse(w, http.StatusBadRequest, "合言葉が必要です")
		return
	}
	log.Printf("[GameHandler] Passcode for join: %s", passcode)

	// リクエストボディからプレイヤーのデッキIDを取得
	var req struct {
		DeckID string `json:"deck_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[GameHandler] Failed to parse passcode join request body: %v", err)
		WriteErrorResponse(w, http.StatusBadRequest, "リクエストボディの解析に失敗しました")
		return
	}
	if req.DeckID == "" {
		log.Printf("[GameHandler] Missing deck_id in passcode join request")
		WriteErrorResponse(w, http.StatusBadRequest, "デッキIDが必要です")
		return
	}
	log.Printf("[GameHandler] Request parsed for passcode join, deck_id: %s", req.DeckID)

	log.Printf("[GameHandler] Calling sessionManager.JoinRoomByPasscode for user %s, passcode %s, deck %s", userID, passcode, req.DeckID)
	
	// セッションマネージャーに合言葉でのマッチングを依頼
	sessionID, isNewSession, err := h.sessionManager.JoinRoomByPasscode(passcode, userID, req.DeckID)
	if err != nil {
		log.Printf("[GameHandler] User %s failed to join passcode %s: %v", userID, passcode, err)
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("合言葉でのマッチングに失敗しました: %v", err))
		return
	}

	var message string
	if isNewSession {
		message = fmt.Sprintf("合言葉「%s」でルームを作成しました。相手の参加をお待ちください。", passcode)
		log.Printf("[GameHandler] User %s created new session with passcode %s", userID, passcode)
	} else {
		message = fmt.Sprintf("合言葉「%s」のルームに参加しました。", passcode)
		log.Printf("[GameHandler] User %s joined existing session with passcode %s", userID, passcode)
	}

	log.Printf("[GameHandler] User %s successfully matched with passcode %s (session: %s)", userID, passcode, sessionID)
	WriteJSONResponse(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"message":        message,
		"session_id":     sessionID,
		"is_new_session": isNewSession,
	})
}


