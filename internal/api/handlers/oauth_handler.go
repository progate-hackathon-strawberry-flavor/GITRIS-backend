package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
)

// GitHub Oathの認証を処理するハンドラー
type OAuthHandler struct {
	userRepo *database.UserRepository
}

// 新しいOAuthHandlerを作成します。
func NewOAuthHandler(userRepo *database.UserRepository) *OAuthHandler {
	return &OAuthHandler{
		userRepo: userRepo,
	}
}

// GitHub Oathのコールバックリクエストを表す構造体
type OAuth2CallbackRequest struct {
	Code string `json:"code"`
}

// GitHub Oathのコールバックレスポンスを表す構造体
type OAuth2CallbackResponse struct {
	Token   string `json:"token"`
	UserID  string `json:"user_id"`
	Login   string `json:"login"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// クライアントからのOAuthコールバックを処理します
// このエンドポイントはGitHub OAuthコードをJWTトークンに交換します
func (h *OAuthHandler) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req OAuth2CallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.Code == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   "Code is required",
		})
		return
	}

	log.Printf("Processing OAuth callback with code: %s...", req.Code[:20])

	accessToken, err := ExchangeCodeForToken(req.Code)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to exchange code: %v", err),
		})
		return
	}

	// GitHub APIを呼び出してユーザー情報を取得します
	user, err := GetGitHubUser(accessToken)
	if err != nil {
		log.Printf("Error getting GitHub user: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to get user info: %v", err),
		})
		return
	}

	githubID := fmt.Sprintf("%d", user.ID)

	// 既存のユーザーをGitHub IDで検索します
	existingUser, err := h.userRepo.GetUserByGitHubID(ctx, githubID)
	if err != nil {
		log.Printf("Error querying user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   "Database error",
		})
		return
	}

	var userID string
	if existingUser != nil {
		// 既存のユーザーが見つかりました。必要に応じて更新します。
		log.Printf("User already exists: GitHub ID=%d, UUID=%s", user.ID, existingUser.ID)
		userID = existingUser.ID.String()

		// ここでは、ユーザーの表示名やアイコンURLをGitHubから最新の情報に更新することができます。
		existingUser.DisplayName = user.Name
		existingUser.IconURL = user.AvatarURL
		existingUser.UserName = user.Login
		existingUser.GithubAccessToken = accessToken
		_, err := h.userRepo.UpdateUser(ctx, existingUser)
		if err != nil {
			log.Printf("Error updating user: %v", err)
		}
	} else {
		// ユーザーが存在しない場合は新規作成します。
		newUser := &database.User{
			ID:                uuid.New(),
			GitHubID:          githubID,
			DisplayName:       user.Name,
			IconURL:           user.AvatarURL,
			UserName:          user.Login,
			GithubAccessToken: accessToken,
		}

		createdUser, err := h.userRepo.CreateUser(ctx, newUser)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(OAuth2CallbackResponse{
				Success: false,
				Error:   "Failed to create user",
			})
			return
		}

		userID = createdUser.ID.String()
	}

	// JWTトークンを生成します。ここでは、ユーザーIDとGitHub IDをペイロードに含めます。
	token, err := GenerateJWT(userID, int(user.ID), user.Login)
	if err != nil {
		log.Printf("Error generating JWT: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(OAuth2CallbackResponse{
			Success: false,
			Error:   "Failed to generate token",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(OAuth2CallbackResponse{
		Success: true,
		Token:   token,
		UserID:  userID,
		Login:   user.Login,
	})
}

// GitHub OAuthコードをクレデンシャルを使用してアクセストークンに交換します
func exchangeCodeForTokenWithCreds(code, clientID, clientSecret string) (string, error) {
	return ExchangeCodeForTokenWithParams(code, clientID, clientSecret)
}

//github oauthのコード交換とユーザー情報取得の関数
//ロゴが入っていない？

// 認証されたユーザーの情報を取得するハンドラー
func (h *OAuthHandler) GetUserInfoHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		// CORSプリフライトリクエストに対応
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	// コンテキストからユーザーIDを取得（AuthMiddlewareで設定される）
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok || userID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "User ID not found in context"})
		return
	}

	// String から UUID に変換
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		log.Printf("Error parsing user ID: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid user ID format"})
		return
	}

	// ユーザー情報をデータベースから取得
	user, err := h.userRepo.GetUserByID(ctx, userUUID)
	if err != nil {
		log.Printf("Error fetching user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch user info"})
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "User not found"})
		return
	}

	// ユーザー情報をレスポンスとして返す
	userInfo := map[string]interface{}{
		"user_id":      user.ID.String(),
		"github_id":    user.GitHubID,
		"username":     user.UserName,
		"display_name": user.DisplayName,
		"icon_url":     user.IconURL,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(userInfo)
}
