package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQLドライバー
)

// DailyContribution represents a single day's contribution data.
// type DailyContribution struct {
// 	Date            string
// 	ContributionCount int
// }

// DatabaseService provides methods for interacting with the database.
type DatabaseService struct {
	DB *sql.DB
}

// NewDatabaseService creates a new instance of DatabaseService and establishes a database connection.
func NewDatabaseService(databaseURL string) (*DatabaseService, error) {
	log.Printf("データベース接続を試行中: URLの最初の50文字: %s...", databaseURL[:min(len(databaseURL), 50)]) // URLの冒頭をログ出力
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Printf("DatabaseService Error: sql.Openに失敗しました: %v", err)
		return nil, fmt.Errorf("データベースへの接続オブジェクト作成に失敗しました: %w", err)
	}

	// データベース接続の確認 (Ping)
	err = db.Ping()
	if err != nil {
		log.Printf("DatabaseService Error: db.Pingに失敗しました: %v", err)
		log.Printf("DatabaseService Error: データベース接続エラーの詳細: %s", err.Error())
		return nil, fmt.Errorf("データベースのPingに失敗しました。接続情報やネットワークを確認してください: %w", err)
	}

	log.Println("データベースに正常に接続しました。")
	return &DatabaseService{DB: db}, nil
}

// GetGitHubUsernameByUserID fetches the GitHub username for a given user ID (UUID).
func (s *DatabaseService) GetGitHubUsernameByUserID(userID string) (string, error) {
	var githubUsername string
	// users テーブルから userID に紐づく user_name を取得するクエリ
	query := `SELECT user_name FROM users WHERE id = $1`
	err := s.DB.QueryRow(query, userID).Scan(&githubUsername)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("ユーザーID %s に紐づくGitHubユーザー名が見つかりません。", userID)
		}
		return "", fmt.Errorf("GitHubユーザー名の取得に失敗しました: %w", err)
	}
	log.Printf("DatabaseService Info: ユーザーID %s に対応するGitHubユーザー名 '%s' を取得しました。", userID, githubUsername)
	return githubUsername, nil
}

func (s *DatabaseService) GetContributionsByUserID(userID string)([]DailyContribution, error){
	log.Printf("DatabaseService Info: ユーザーID %s の保存済み貢献データを取得中...", userID)
	var contributions []DailyContribution
	query := `SELECT date, contribution_count FROM contribution_data WHERE user_id = $1 ORDER BY date ASC`

	rows, err := s.DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("保存ずみ貢献データのクエリ実行に失敗しました： %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var date time.Time
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, fmt.Errorf("保存ずみ貢献データのスキャンに失敗しました: %w", err)
		}
		contributions = append(contributions, DailyContribution{
			Date: date.Format("2006-01-02"),
			ContributionCount: count,
		})
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("保存ずみ貢献データの行処理中にエラーが発生しました: %w", err)
	}

	log.Printf("DatabaseService Info: ユーザーID %s の保存ずみ貢献データ %d 券を取得しました",userID, len(contributions))
	return contributions, nil
}


// SaveContributions saves a slice of daily contributions for a given user.
// It first deletes existing contributions for the user and then inserts the new ones.
func (s *DatabaseService) SaveContributions(userID string, contributions []DailyContribution) error {
	tx, err := s.DB.Begin() // トランザクションを開始
	if err != nil {
		log.Printf("DatabaseService Error: トランザクションの開始に失敗しました: %v", err)
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer tx.Rollback() // エラーが発生した場合はロールバック

	// 1. 既存のcontribution_dataを全て削除
	log.Printf("DatabaseService Info: ユーザーID %s の既存の貢献データを削除中...", userID)
	deleteStmt := `DELETE FROM contribution_data WHERE user_id = $1`
	_, err = tx.Exec(deleteStmt, userID)
	if err != nil {
		return fmt.Errorf("既存の貢献データの削除に失敗しました: %w", err)
	}
	log.Printf("DatabaseService Info: ユーザーID %s の既存の貢献データを削除しました。", userID)

	// 2. 新しい貢献データを挿入
	insertStmt := `
		INSERT INTO contribution_data (user_id, date, contribution_count, created_at)
		VALUES ($1, $2, $3, $4)
	`
	for _, c := range contributions {
		// 日付文字列を time.Time にパース
		parsedDate, err := time.Parse("2006-01-02", c.Date)
		if err != nil {
			return fmt.Errorf("日付のパースに失敗しました (%s): %w", c.Date, err)
		}

		_, err = tx.Exec(
			insertStmt,
			userID,
			parsedDate,
			c.ContributionCount,
			time.Now(), // created_at
		)
		if err != nil {
			return fmt.Errorf("貢献データの挿入に失敗しました (日付: %s, 貢献数: %d): %w", c.Date, c.ContributionCount, err)
		}
	}
	log.Printf("DatabaseService Info: ユーザーID %s の新しい貢献データ %d 件を挿入しました。", userID, len(contributions))

	return tx.Commit() // トランザクションをコミット
}

// min helper function for logging
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
