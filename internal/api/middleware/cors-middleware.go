package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/rs/cors"
)

// CORSHandler はCORS設定を適用するミドルウェアを返します。
// 環境変数 CORS_ALLOWED_ORIGINS にカンマ区切りでオリジンを追加できます。
func CORSHandler() func(http.Handler) http.Handler {
	origins := []string{"http://localhost:3000", "https://gitris-frontend-mauve.vercel.app", "https://gitris-frontend-deploy.vercel.app"}
	if extra := os.Getenv("CORS_ALLOWED_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})
	return c.Handler
}