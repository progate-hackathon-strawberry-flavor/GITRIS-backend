package middleware

import (
	"net/http"

	"github.com/rs/cors"
)

// CORSHandler はCORS設定を適用するミドルウェアを返します。
func CORSHandler() func(http.Handler) http.Handler {
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000","https://gitris-frontend-deploy.vercel.app"}, // フロントエンドのオリジン
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})
	return c.Handler
}
