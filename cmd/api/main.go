package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/handlers"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services"
)

func main() {
	// .envファイルを読み込む (本番環境以外の場合)
	if os.Getenv("APP_ENV") != "production" {
		err := godotenv.Load()
		if err != nil {
			log.Printf("warning: .envファイルの読み込み中にエラーが発生しました (本番環境では問題ありません): %v", err)
		}
	}

	// データベースURLを環境変数から取得
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("エラー: DATABASE_URL 環境変数が設定されていません。")
	}

	// サービス層の初期化
	githubService := services.NewGitHubService()
	// DatabaseService の初期化
	databaseService, err := services.NewDatabaseService(databaseURL)
	if err != nil {
		log.Fatalf("DatabaseService の初期化に失敗しました: %v", err)
	}
	defer databaseService.DB.Close() // アプリケーション終了時にデータベース接続を閉じる

	// ハンドラ層の初期化
	contributionHandler := handlers.NewContributionHandler(githubService, databaseService)

	// gorilla/mux ルーターの初期化
	r := mux.NewRouter()

	// 認証不要な公開エンドポイント
	r.HandleFunc("/api/public", handlers.PublicHandler).Methods("GET")

	// データベースから保存済みのGitHub Contributionデータを取得するエンドポイント
	// GET /api/contributions/{userID}
	r.HandleFunc("/api/contributions/{userID}", contributionHandler.GetSavedContributionsHandler).Methods("GET")

	// GitHubから最新のContributionデータを取得し、データベースを更新するエンドポイント
	// POST /api/contributions/refresh/{userID} (または PUT)
	r.HandleFunc("/api/contributions/refresh/{userID}", contributionHandler.GetDailyContributionsAndSaveHandler).Methods("POST")

	// 認証が必要なルートグループを作成
	protectedRouter := r.PathPrefix("/api/protected").Subrouter()
	protectedRouter.Use(handlers.AuthMiddleware)

	// 認証が必要なエンドポイント
	protectedRouter.HandleFunc("/decks", handlers.GetDecksForUser).Methods("GET")

	// ポート番号の設定
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("サーバーをポート %s で起動中...", port)
	// ユーザーに新しいURL形式を伝えるメッセージ
	fmt.Printf("保存済みのGitHub Contributionデータを取得するには、以下のURLにアクセスしてください： http://localhost:%s/api/contributions/{あなたのSupabase usersテーブルのUUID}\n", port)
	fmt.Printf("GitHubから最新のデータを取得してデータベースを更新するには、以下のURLにPOSTリクエストを送ってください： http://localhost:%s/api/contributions/refresh/{あなたのSupabase usersテーブルのUUID}\n", port)


	// HTTPサーバーの起動
	log.Fatal(http.ListenAndServe(":"+port, r))
}
