package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

type UserIDKey struct{}

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey{}).(string)
	return userID, ok
}

// AuthMiddleware is a middleware function that checks for a valid JWT token.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. authorizationヘッダーからJWTを取得
		authHeader := r.Header.Get("Authorization")
		log.Printf("AuthMiddleware Debug: Authorization header: %s", authHeader)
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		tokenString := ""
		if len(authHeader) > 7 && authHeader[0:7] == "Bearer " {
			tokenString = authHeader[7:]
			log.Printf("AuthMiddleware Debug: Extracted token: %s", tokenString)
		} else {
			http.Error(w, "Invalid Authorization header format. Must be 'Bearer <token>'",
				http.StatusUnauthorized)
			return
		}

		// 2. JWT Secretを取得
		jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
		log.Printf("AuthMiddleware Debug: JWT Secret length: %d", len(jwtSecret))
		if jwtSecret == "" {
			log.Println("Error: SUPABASE_JWT_SECRET environment variable is not set.")
			http.Error(w, "Server configuration error: JWT secret missing", http.StatusInternalServerError)
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
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !token.Valid {
			log.Printf("AuthMiddleware Error: Invalid token")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// トークンのクレームを取得
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			log.Printf("AuthMiddleware Error: Invalid token claims")
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}

		// SupabaseのJWTは通常、ユーザーIDを 'sub' (Subject) クレームにUUIDとして格納します。
		userID, ok := claims["sub"].(string)
		if !ok {
			log.Printf("AuthMiddleware Error: JWT claims missing 'sub' (userID) or wrong type: %v", claims["sub"])
			http.Error(w, "Invalid token: missing user ID", http.StatusUnauthorized)
			return
		}

		log.Printf("AuthMiddleware Debug: Successfully authenticated user: %s", userID)
		// 6. ユーザーIDをContextに設定して次のハンドラに渡す
		ctx := context.WithValue(r.Context(), UserIDKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}