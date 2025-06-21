package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	api "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/handlers"
	auth "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/github"
	services "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/deck" // 新しいサービスのインポート
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/tetris"        // テトリスサービスをインポート
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
	githubService := github.NewGitHubService()
	// DatabaseService の初期化 (ここで *sql.DB インスタンスも保持している)
	databaseService, err := database.NewDatabaseService(databaseURL)
	if err != nil {
		log.Fatalf("DatabaseService の初期化に失敗しました: %v", err)
	}
	defer databaseService.DB.Close() // アプリケーション終了時にデータベース接続を閉じる
	fmt.Println("データベース接続が正常に確立されました。")


	// Deck関連の依存関係の初期化
	// databaseService.DB を直接リポジトリとサービスに渡す
	deckRepo := database.NewDeckRepository(databaseService.DB)
	deckService := services.NewDeckService(databaseService.DB, deckRepo)

	// テトリスゲームのセッションマネージャーを初期化
	sessionManager := tetris.NewSessionManager(databaseService)

	// ハンドラ層の初期化
	contributionHandler := api.NewContributionHandler(githubService, databaseService)
	deckSaveHandler := api.NewDeckSaveHandler(deckService) // デッキ保存ハンドラの初期化
	deckGetHandler := api.NewDeckGetHandler(deckService) // デッキ取得ハンドラの初期化
	gameHandler := api.NewGameHandler(sessionManager, databaseService) // ゲームハンドラの初期化
	// gorilla/mux ルーターの初期化
	r := mux.NewRouter()

	// これにより、すべてのリクエストがまずCORSハンドラを通過するようになります。
	r.Use(auth.CORSHandler())

	// 静的ファイル配信（テスト用）
	r.HandleFunc("/test_websocket_client.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "test_websocket_client.html")
	})

	// 認証不要な公開エンドポイント
	r.HandleFunc("/api/public", api.PublicHandler).Methods("GET")

	// データベースから保存済みのGitHub Contributionデータを取得するエンドポイント
	// GET /api/contributions/{userID}
	r.HandleFunc("/api/contributions/{userID}", contributionHandler.GetSavedContributionsHandler).Methods("GET", "OPTIONS")

	// GitHubから最新のContributionデータを取得し、データベースを更新するエンドポイント
	// POST /api/contributions/refresh/{userID} (または PUT)
	r.HandleFunc("/api/contributions/refresh/{userID}", contributionHandler.GetDailyContributionsAndSaveHandler).Methods("POST")

	// 認証が必要なルートグループを作成
	protectedRouter := r.PathPrefix("/api/protected").Subrouter()
	protectedRouter.Use(auth.AuthMiddleware)
	protectedRouter.Use(auth.CORSHandler()) // CORSミドルウェアを追加

	// 認証済みユーザーのみが自身のデッキを保存できるようにします
	protectedRouter.Handle("/deck/save", deckSaveHandler).Methods("POST", "OPTIONS")
	// 認証済みユーザーのデッキを取得できるようにします
	protectedRouter.Handle("/deck/{userID}", deckGetHandler).Methods("GET", "OPTIONS")

	// テトリスゲーム関連のルート
	// 認証が必要なゲームルート
	gameRouter := r.PathPrefix("/api/game").Subrouter()
	gameRouter.Use(auth.AuthMiddleware) // 認証を有効化
	gameRouter.Use(auth.CORSHandler()) // CORSミドルウェアを追加
	
	// ルーム参加
	gameRouter.HandleFunc("/room/{roomID}/join", gameHandler.JoinRoom).Methods("POST", "OPTIONS")
	gameRouter.HandleFunc("/room/{roomID}/status", gameHandler.GetRoomStatus).Methods("GET", "OPTIONS")
	
	// ルーム作成（認証ミドルウェアをバイパス）
	r.HandleFunc("/api/game/room", gameHandler.CreateRoom).Methods("POST", "OPTIONS")
	
	// WebSocket接続（認証ミドルウェアをバイパス）
	r.HandleFunc("/api/game/ws/{roomID}", gameHandler.HandleWebSocketConnection)

	// ポート番号の設定
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("サーバーをポート %s で起動中...", port)
	// ユーザーに新しいURL形式を伝えるメッセージ
	fmt.Printf("保存済みのGitHub Contributionデータを取得するには、以下のURLにアクセスしてください： http://localhost:%s/api/contributions/{あなたのSupabase usersテーブルのUUID}\n", port)
	fmt.Printf("GitHubから最新のデータを取得してデータベースを更新するには、以下のURLにPOSTリクエストを送ってください： http://localhost:%s/api/contributions/refresh/{あなたのSupabase usersテーブルのUUID}\n", port)
	fmt.Printf("デッキを保存するには、認証トークンと以下のURLにPOSTリクエストを送ってください： http://localhost:%s/api/protected/deck/save\n", port)
	fmt.Printf("テトリスゲームのテストクライアント: http://localhost:%s/test_websocket_client.html\n", port)


	// HTTPサーバーの起動
	log.Fatal(http.ListenAndServe(":"+port, r))
}
