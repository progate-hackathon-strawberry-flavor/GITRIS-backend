package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
)

// ResultRepository はゲーム結果関連のデータベース操作を定義するインターフェースです。
type ResultRepository interface {
	// CreateResult は新しいゲーム結果レコードを作成します
	CreateResult(tx *sql.Tx, userID string, score int) (*models.Result, error)
	
	// GetTopResults は上位N件の結果を取得します（ランキング用）
	GetTopResults(limit int) ([]models.ResultResponse, error)
	
	// GetUserBestScore は指定したユーザーの最高スコアを取得します
	GetUserBestScore(userID string) (*models.Result, error)
	
	// GetUserRanking は指定したユーザーの現在のランキング順位を取得します
	GetUserRanking(userID string) (*models.ResultResponse, error)
}

// resultRepositoryImpl はResultRepositoryインターフェースの実装です。
type resultRepositoryImpl struct {
	db *sql.DB
}

// NewResultRepository はResultRepositoryの新しいインスタンスを作成します。
func NewResultRepository(db *sql.DB) ResultRepository {
	return &resultRepositoryImpl{db: db}
}

// CreateResult は新しいゲーム結果レコードを作成します。
func (r *resultRepositoryImpl) CreateResult(tx *sql.Tx, userID string, score int) (*models.Result, error) {
	now := time.Now()
	var id int64
	
	// トランザクションの有無を確認して適切にクエリを実行
	var row *sql.Row
	if tx != nil {
		row = tx.QueryRow(
			"INSERT INTO results (user_id, score, created_at) VALUES ($1, $2, $3) RETURNING id",
			userID, score, now,
		)
	} else {
		row = r.db.QueryRow(
			"INSERT INTO results (user_id, score, created_at) VALUES ($1, $2, $3) RETURNING id",
			userID, score, now,
		)
	}
	
	err := row.Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("ゲーム結果レコードの作成に失敗しました: %w", err)
	}
	
	return &models.Result{
		ID:        id,
		UserID:    userID,
		Score:     score,
		CreatedAt: now,
	}, nil
}

// GetTopResults は上位N件の結果を取得します（ランキング用）。
func (r *resultRepositoryImpl) GetTopResults(limit int) ([]models.ResultResponse, error) {
	query := `
		SELECT 
			id, user_id, score, created_at,
			ROW_NUMBER() OVER (ORDER BY score DESC, created_at ASC) as rank
		FROM results 
		ORDER BY score DESC, created_at ASC
		LIMIT $1
	`
	
	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("ゲーム結果取得に失敗しました: %w", err)
	}
	defer rows.Close()
	
	var results []models.ResultResponse
	for rows.Next() {
		var result models.ResultResponse
		err := rows.Scan(&result.ID, &result.UserID, &result.Score, &result.CreatedAt, &result.Rank)
		if err != nil {
			return nil, fmt.Errorf("ゲーム結果データのスキャンに失敗しました: %w", err)
		}
		results = append(results, result)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ゲーム結果取得中にエラーが発生しました: %w", err)
	}
	
	return results, nil
}

// GetUserBestScore は指定したユーザーの最高スコアを取得します。
func (r *resultRepositoryImpl) GetUserBestScore(userID string) (*models.Result, error) {
	query := `
		SELECT id, user_id, score, created_at
		FROM results 
		WHERE user_id = $1 
		ORDER BY score DESC, created_at ASC
		LIMIT 1
	`
	
	row := r.db.QueryRow(query, userID)
	
	var result models.Result
	err := row.Scan(&result.ID, &result.UserID, &result.Score, &result.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // ユーザーのスコアが存在しない場合はnilを返す
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザーの最高スコア取得に失敗しました: %w", err)
	}
	
	return &result, nil
}

// GetUserRanking は指定したユーザーの現在のランキング順位を取得します。
func (r *resultRepositoryImpl) GetUserRanking(userID string) (*models.ResultResponse, error) {
	// ユーザーの最高スコアを先に取得
	bestScore, err := r.GetUserBestScore(userID)
	if err != nil {
		return nil, err
	}
	if bestScore == nil {
		return nil, nil // ユーザーのスコアが存在しない
	}
	
	// そのスコアでの順位を計算
	query := `
		SELECT COUNT(*) + 1 as rank
		FROM results 
		WHERE score > $1 OR (score = $1 AND created_at < $2)
	`
	
	var rank int
	err = r.db.QueryRow(query, bestScore.Score, bestScore.CreatedAt).Scan(&rank)
	if err != nil {
		return nil, fmt.Errorf("ユーザーランキング順位の計算に失敗しました: %w", err)
	}
	
	return &models.ResultResponse{
		ID:        bestScore.ID,
		UserID:    bestScore.UserID,
		Score:     bestScore.Score,
		CreatedAt: bestScore.CreatedAt,
		Rank:      rank,
	}, nil
} 