package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type UserIDKey struct{}

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey{}).(string)
	return userID, ok
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// AuthMiddleware is a middleware function that checks for a valid JWT token.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// テスト用: 環境変数で認証をバイパス可能にする
		if os.Getenv("BYPASS_AUTH") == "true" {
			// テスト用のランダムなユーザーIDを生成（毎回異なるユーザーとして扱う）
			testUserID := uuid.New().String()
			log.Printf("AuthMiddleware: BYPASS_AUTH enabled, generated test user ID: %s", testUserID)
			ctx := context.WithValue(r.Context(), UserIDKey{}, testUserID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 1. authorizationヘッダーからJWTを取得
		authHeader := r.Header.Get("Authorization")
		log.Printf("AuthMiddleware Debug: Authorization header: %s", authHeader)
		if authHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "Authorization header is required")
			return
		}

		tokenString := ""
		if len(authHeader) > 7 && authHeader[0:7] == "Bearer " {
			tokenString = authHeader[7:]
			log.Printf("AuthMiddleware Debug: Extracted token: %s", tokenString)
		} else {
			writeJSONError(w, http.StatusUnauthorized, "Invalid Authorization header format. Must be 'Bearer <token>'")
			return
		}

		// 2. JWT Secretを取得
		jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
		log.Printf("AuthMiddleware Debug: JWT Secret length: %d", len(jwtSecret))
		if jwtSecret == "" {
			log.Println("Error: SUPABASE_JWT_SECRET environment variable is not set.")
			writeJSONError(w, http.StatusInternalServerError, "Server configuration error: JWT secret missing")
			return
		}

		// JWTの検証とパース
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// アルゴリズムがHMACであることを確認
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				log.Printf("AuthMiddleware Error: Unexpected signing method: %v", token.Header["alg"])
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil {
			log.Printf("AuthMiddleware Error: JWT parse error: %v", err)
			writeJSONError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		if !token.Valid {
			log.Printf("AuthMiddleware Error: Invalid token")
			writeJSONError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		// トークンのクレームを取得
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			log.Printf("AuthMiddleware Error: Invalid token claims")
			writeJSONError(w, http.StatusUnauthorized, "Invalid token claims")
			return
		}

		// SupabaseのJWTは通常、ユーザーIDを 'sub' (Subject) クレームにUUIDとして格納します。
		userID, ok := claims["sub"].(string)
		if !ok {
			log.Printf("AuthMiddleware Error: JWT claims missing 'sub' (userID) or wrong type: %v", claims["sub"])
			writeJSONError(w, http.StatusUnauthorized, "Invalid token: missing user ID")
			return
		}

		log.Printf("AuthMiddleware Debug: Successfully authenticated user: %s", userID)
		// 6. ユーザーIDをContextに設定して次のハンドラに渡す
		ctx := context.WithValue(r.Context(), UserIDKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}