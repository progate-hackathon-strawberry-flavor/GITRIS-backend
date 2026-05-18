package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"
)

// コンテキストからユーザーIDを取得する関数
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	return middleware.GetUserIDFromContext(ctx)
}

// OAuth用の定数
const (
	gitHubTokenURL = "https://github.com/login/oauth/access_token"
)

// GitHubOAuthTokenResponse represents the response from GitHub OAuth token endpoint
type GitHubOAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// GitHubUserResponse represents the GitHub user API response
type GitHubUserResponse struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// CustomJWTClaims represents custom JWT claims for this application
type CustomJWTClaims struct {
	UserID   string `json:"user_id"`
	GitHubID int    `json:"github_id"`
	Login    string `json:"login"`
	jwt.RegisteredClaims
}

// GitHubから受け取った OAuth code を　GitHub apiにアクセスする access tokenに交換する関数
func ExchangeCodeForToken(code string) (string, error) {
	clientID := os.Getenv("GITHUB_CLIENT_ID")
	clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("GitHub OAuth credentials not configured")
	}

	return ExchangeCodeForTokenWithParams(code, clientID, clientSecret)
}

// 取得したアクセストークンを使ってGitHub APIからユーザ情報を取得する関数
func ExchangeCodeForTokenWithParams(code, clientID, clientSecret string) (string, error) {
	payload := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"code":          code,
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", gitHubTokenURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp GitHubOAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("GitHub error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return tokenResp.AccessToken, nil
}

// アクセストークンを使って、GitHub APIからユーザ情報を取得する関数
func GetGitHubUser(accessToken string) (*GitHubUserResponse, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(body))
	}

	var user GitHubUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &user, nil
}

// アプリ内で認証されたユーザに使うJWTを生成する関数
func GenerateJWT(userID string, githubID int, login string) (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}

	now := time.Now()
	expirationTime := now.Add(24 * time.Hour) // JWT有効期限: 24時間

	claims := &CustomJWTClaims{
		UserID:   userID,
		GitHubID: githubID,
		Login:    login,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			Issuer:    "gitris-backend",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	log.Printf("Generated JWT for user %s (GitHub: %s)", userID, login)
	return tokenString, nil
}
