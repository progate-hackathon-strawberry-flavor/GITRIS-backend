package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq" // PostgreSQLドライバー
	"github.com/joho/godotenv" // .envファイルを読み込むため
)

func main() {
	// .envファイルを読み込む (開発環境の場合)
	err := godotenv.Load()
	if err != nil {
		log.Printf("warning: Error loading .env file: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("エラー: DATABASE_URL 環境変数が設定されていません。")
	}

	fmt.Printf("テスト開始: データベース接続を試行中...\nURLの最初の50文字: %s...\n", databaseURL[:min(len(databaseURL), 50)])

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("エラー: データベースへの接続オブジェクト作成に失敗しました: %v", err)
	}
	defer db.Close() // 関数終了時に接続を閉じる

	err = db.Ping()
	if err != nil {
		log.Fatalf("エラー: データベースのPingに失敗しました。接続情報やネットワークを確認してください: %v", err)
	}

	fmt.Println("成功: データベースに正常に接続し、Pingが成功しました！")
	fmt.Println("これで、アプリケーションからデータベースにアクセスできるはずです。")

    // テストとして簡単なクエリを実行してみる (任意)
    var version string
    err = db.QueryRow("SELECT version()").Scan(&version)
    if err != nil {
        log.Printf("警告: SELECT version() クエリの実行に失敗しました: %v", err)
    } else {
        fmt.Printf("データベースバージョン: %s\n", version)
    }
}

// min helper function for logging
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

