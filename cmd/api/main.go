package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	api "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/handlers"
	auth "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/github"
	deck "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/deck"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/tetris"
)

func main() {
	ctx := context.Background()

	// .envファイルを読み込む (本番環境以外の場合)
	if os.Getenv("APP_ENV") != "production" {
		err := godotenv.Load()
		if err != nil {
			log.Printf("warning: .envファイルの読み込み中にエラーが発生しました (本番環境では問題ありません): %v", err)
		}
	}

	// 環境変数からデータベース接続情報を取得 (ESOがKubernetes Secretとして注入)
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USERNAME")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	var databaseURL string
	if dbHost != "" && dbPort != "" && dbUser != "" && dbPass != "" && dbName != "" {
		databaseURL = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=require", dbUser, dbPass, dbHost, dbPort, dbName)
	} else {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		log.Fatal("エラー: DB_HOST/DB_PORT/DB_USERNAME/DB_PASSWORD/DB_NAME または DATABASE_URL が設定されていません。")
	}

	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET が設定されていません。")
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

	// ユーザーリポジトリの初期化
	userRepo := database.NewUserRepository(databaseService.DB)

	// Deck関連の依存関係の初期化
	// databaseService.DB を直接リポジトリとサービスに渡す
	deckRepo := database.NewDeckRepository(databaseService.DB)
	deckService := deck.NewDeckService(databaseService.DB, deckRepo)

	// ゲーム結果関連の依存関係の初期化
	resultRepo := database.NewResultRepository(databaseService.DB)

	// Redis クライアントの初期化（REDIS_URL未設定の場合はnil、ローカル開発では動作継続）
	redisURL := os.Getenv("REDIS_URL")
	sessionManager := func() *tetris.SessionManager {
		if redisURL != "" {
			rc, err := tetris.NewRedisClient(redisURL)
			if err != nil {
				log.Printf("警告: Redisへの接続に失敗しました (シングルポッドモードで継続): %v", err)
				return tetris.NewSessionManager(databaseService, deckRepo, resultRepo, nil)
			}
			log.Printf("Redisへの接続が確立されました: %s", redisURL)
			return tetris.NewSessionManager(databaseService, deckRepo, resultRepo, rc)
		}
		log.Printf("REDIS_URL未設定: シングルポッドモードで起動します")
		return tetris.NewSessionManager(databaseService, deckRepo, resultRepo, nil)
	}()
	// SessionManager.Run()はNewSessionManager内で既に開始されているため、重複実行を回避

	// ハンドラ層の初期化
	contributionHandler := api.NewContributionHandler(githubService, databaseService)
	deckSaveHandler := api.NewDeckSaveHandler(deckService)             // デッキ保存ハンドラの初期化
	deckGetHandler := api.NewDeckGetHandler(deckService)               // デッキ取得ハンドラの初期化
	gameHandler := api.NewGameHandler(sessionManager, databaseService) // ゲームハンドラの初期化
	resultHandler := api.NewResultHandler(resultRepo)                  // ゲーム結果ハンドラの初期化
	publicHandler := api.NewPublicHandler(databaseService)             // 公開ハンドラの初期化
	oauthHandler := api.NewOAuthHandler(userRepo)
	// gorilla/mux ルーターの初期化
	r := mux.NewRouter()

	// これにより、すべてのリクエストがまずCORSハンドラを通過するようになります。
	r.Use(auth.CORSHandler())
	r.Use(stickySessionMiddleware)

	// 静的ファイル配信（テスト用）
	r.HandleFunc("/test_websocket_client.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "test_websocket_client.html")
	})

	// 認証不要な公開エンドポイント
	r.HandleFunc("/api/public", api.PublicHandlerFunc).Methods("GET")
	r.HandleFunc("/api/user/{userID}/display-name", publicHandler.GetUserDisplayNameHandler).Methods("GET", "OPTIONS")

	// OAuth callback endpoint (認証不要)
	r.HandleFunc("/api/auth/callback", oauthHandler.HandleOAuthCallback).Methods("POST", "OPTIONS")

	// 認証が必要なルートグループを作成
	protectedRouter := r.PathPrefix("/api/protected").Subrouter()
	protectedRouter.Use(auth.AuthMiddleware)
	protectedRouter.Use(auth.CORSHandler()) // CORSミドルウェアを追加

	// 認証済みユーザーのinfo取得エンドポイント
	protectedRouter.HandleFunc("/auth/me", oauthHandler.GetUserInfoHandler).Methods("GET", "OPTIONS")
	// JWTで認証されたユーザー本人のデータのみを扱うエンドポイント
	protectedRouter.HandleFunc("/contributions/{userID}", contributionHandler.GetSavedContributionsHandler).Methods("GET", "OPTIONS")
	protectedRouter.HandleFunc("/contributions/refresh/{userID}", contributionHandler.GetDailyContributionsAndSaveHandler).Methods("POST", "OPTIONS")
	protectedRouter.HandleFunc("/results", resultHandler.PostScore).Methods("POST", "OPTIONS")

	// 認証済みユーザーのみが自身のデッキを保存できるようにします
	protectedRouter.Handle("/deck/save", deckSaveHandler).Methods("POST", "OPTIONS")
	// 認証済みユーザーのデッキを取得できるようにします
	protectedRouter.Handle("/deck/{userID}", deckGetHandler).Methods("GET", "OPTIONS")

	// テトリスゲーム関連のルート
	// 認証が必要なゲームルート
	gameRouter := r.PathPrefix("/api/game").Subrouter()
	gameRouter.Use(auth.AuthMiddleware)
	gameRouter.Use(auth.CORSHandler())

	// ルーム作成（ホスト）
	gameRouter.HandleFunc("/room/create", gameHandler.CreateRoom).Methods("POST", "OPTIONS")

	// 合言葉ベースのマッチング・状態取得
	gameRouter.HandleFunc("/room/passcode/{passcode}/join", gameHandler.JoinRoomByPasscode).Methods("POST", "OPTIONS")
	gameRouter.HandleFunc("/room/passcode/{passcode}/status", gameHandler.GetRoomStatus).Methods("GET", "OPTIONS")
	gameRouter.HandleFunc("/room/passcode/{passcode}/delete", gameHandler.DeleteSession).Methods("DELETE", "OPTIONS")

	// WebSocket接続（合言葉ベース）
	r.HandleFunc("/api/game/ws/{passcode}", gameHandler.HandleWebSocketConnection)

	// ゲーム結果関連のエンドポイント
	r.HandleFunc("/api/results", resultHandler.GetTopResults).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/results/user/{user_id}", resultHandler.GetUserResult).Methods("GET", "OPTIONS")

	// ポート番号の設定
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// HTTPサーバーの設定
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ホスト設定を環境変数から取得
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost" // 開発環境のデフォルト
	}

	log.Printf("サーバーをポート %s で起動中...", port)
	// ユーザーに新しいURL形式を伝えるメッセージ
	fmt.Printf("保存済みのGitHub Contributionデータを取得するには、以下のURLにアクセスしてください： http://%s:%s/api/contributions/{あなたのSupabase usersテーブルのUUID}\n", host, port)
	fmt.Printf("GitHubから最新のデータを取得してデータベースを更新するには、以下のURLにPOSTリクエストを送ってください： http://%s:%s/api/contributions/refresh/{あなたのSupabase usersテーブルのUUID}\n", host, port)
	fmt.Printf("デッキを保存するには、認証トークンと以下のURLにPOSTリクエストを送ってください： http://%s:%s/api/protected/deck/save\n", host, port)
	fmt.Printf("テトリスゲームのテストクライアント: http://%s:%s/test_websocket_client.html\n", host, port)

	// シャットダウンシグナルの待機用チャネル
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// サーバーを別のGoroutineで起動
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("サーバーの起動に失敗しました: %v", err)
		}
	}()

	log.Println("サーバーが正常に起動しました。終了するには Ctrl+C を押してください。")

	// シャットダウンシグナルを待機
	<-quit
	log.Println("サーバーをシャットダウンしています...")

	// SessionManagerを先にシャットダウン
	sessionManager.Shutdown()

	// グレースフルシャットダウンの実行
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("サーバーのシャットダウン中にエラーが発生しました: %v", err)
	}

	log.Println("サーバーが正常にシャットダウンされました。")
}

// GITRIS_ROOM_POD クッキーをレスポンスにechoする
func stickySessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("GITRIS_ROOM_POD")
		isGameEndpoint := strings.Contains(r.URL.Path, "/status") ||
			strings.Contains(r.URL.Path, "/join")
		if err == nil && !isGameEndpoint {
			http.SetCookie(w, &http.Cookie{
				Name:     "GITRIS_ROOM_POD",
				Value:    cookie.Value,
				Path:     "/",
				MaxAge:   86400,
				HttpOnly: false,
				SameSite: http.SameSiteNoneMode,
				Secure:   true,
			})
		}
		next.ServeHTTP(w, r)
	})
}
