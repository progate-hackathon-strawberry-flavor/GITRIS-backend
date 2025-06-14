package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/handlers"
)

func main(){
	if os.Getenv("APP_ENV") != "production"{
		err := godotenv.Load()
		if err != nil {
			log.Printf("warning: Error loading .env file (this is fine in production): %v", err)
		}
	}

	r := mux.NewRouter()
	r.HandleFunc("/api/public", handlers.PublicHandler).Methods("GET")

	// 認証が必要なルートグループを作成
	// PathPrefix を使用して、特定のパスから始まるURLにのみミドルウェアを適用できます。
	// 例えば、/api/protected/ で始まる全てのパスにAuthMiddlewareを適用します。
	protectedRouter := r.PathPrefix("/api/protected").Subrouter()
	protectedRouter.Use(handlers.AuthMiddleware)

	// 認証が必要なエンドポイント
	protectedRouter.HandleFunc("/decks", handlers.GetDecksForUser).Methods("GET")

	port := os.Getenv("PORT")
	if port == ""{
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
