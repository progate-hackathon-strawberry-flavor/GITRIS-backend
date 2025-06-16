package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQLドライバー
)

// DatabaseService provides methods for interacting with the database.
type DatabaseService struct {
	DB *sql.DB
}

// NewDatabaseService creates a new instance of DatabaseService and establishes a database connection.
func NewDatabaseService(databaseURL string) (*DatabaseService, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("データベースへの接続に失敗しました: %w", err)
	}

	// データベース接続の確認
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("データベースのPingに失敗しました: %w", err)
	}

	log.Println("データベースに正常に接続しました。")
	return &DatabaseService{DB: db}, nil
}

// SaveContributions saves a slice of daily contributions for a given user.
// It first deletes existing contributions for the user and then inserts the new ones.
func (s *DatabaseService) SaveContributions(userID string, contributions []DailyContribution) error {
	tx, err := s.DB.Begin() // トランザクションを開始
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer tx.Rollback() // エラーが発生した場合はロールバック

	// 1. 既存のcontribution_dataを全て削除
	log.Printf("ユーザーID %s の既存の貢献データを削除中...", userID)
	deleteStmt := `DELETE FROM contribution_data WHERE user_id = $1`
	_, err = tx.Exec(deleteStmt, userID)
	if err != nil {
		return fmt.Errorf("既存の貢献データの削除に失敗しました: %w", err)
	}
	log.Printf("ユーザーID %s の既存の貢献データを削除しました。", userID)

	// 2. 新しい貢献データを挿入
	insertStmt := `
		INSERT INTO contribution_data (user_id, date, contribution_count, created_at)
		VALUES ($1, $2, $3, $4)
	`
	for _, c := range contributions {
		// 日付文字列を time.Time にパース
		// GitHub GraphQL APIのdateは "YYYY-MM-DD" 形式で来ることを想定
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
	log.Printf("ユーザーID %s の新しい貢献データ %d 件を挿入しました。", userID, len(contributions))

	return tx.Commit() // トランザクションをコミット
}
