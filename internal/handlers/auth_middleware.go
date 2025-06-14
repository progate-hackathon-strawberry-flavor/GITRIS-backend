package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

type UserIDKey struct{}

func AuthMiddleware(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		// 1. authorizationヘッダーからJWTを取得
		authHeader := r.Header.Get("Authorization")
		if authHeader ==""{
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		tokenString := ""
		if len(authHeader) > 7 && authHeader[0:7] == "Bearer "{
			tokenString = authHeader[7:]
		} else {
			http.Error(w, "Invalid Authorization header format. Must be 'Bearer <token>'",
			http.StatusUnauthorized)
			return
		}

		// 2. JWT Secretを取得
		jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
		if jwtSecret == ""{
			log.Println("Error: SUPABASE_JWT_SECRET environment variable is not set.")
			http.Error(w, "Server configuration error: JWT secret missing", http.StatusInternalServerError)
			return
		}

		// JWTの検証とパース
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error){
			// アルゴリズムがHMACであることを確認
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected igning method: %v", token.Header["alg"])
			}
			// Secret Keyを返す
			return []byte(jwtSecret), nil
		})

		if err != nil {
			log.Printf("JWT parsing error: %v", err)
			http.Error(w, "Invalid token format or signature", http.StatusUnauthorized)
			return
		}

		// 4. トークンが有効であることを確認
		if !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// 5. クレーム（ペイロード）からユーザーIDを取得
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "Invalid token claims format", http.StatusUnauthorized)
			return
		}

		// SupabaseのJWTは通常、ユーザーIDを 'sub' (Subject) クレームにUUIDとして格納します。
		userID, ok := claims["sub"].(string) 
		if !ok {
			log.Printf("JWT claims missing 'sub' (userID) or wrong type: %v", claims["sub"])
			http.Error(w, "User ID not found in token", http.StatusUnauthorized)
			return
		}

		// 6. ユーザーIDをContextに設定して次のハンドラに渡す
		// ContextにユーザーIDを設定することで、次のハンドラ関数で安全に取得できます。
		ctx := context.WithValue(r.Context(), UserIDKey{}, userID) 
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPリクエストのContextからユーザーIDを取得する関数（ヘルパー）
func GetUserIDFromContext(ctx context.Context)(string, bool){
	userID, ok := ctx.Value(UserIDKey{}).(string)
	return userID, ok
}