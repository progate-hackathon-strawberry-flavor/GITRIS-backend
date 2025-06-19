package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	// "log"
	"time"

	"github.com/google/uuid"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models" // プロジェクトのルートパスに合わせて修正
)

// DeckRepository はデッキ関連のデータベース操作を定義するインターフェースです。
type DeckRepository interface {
	GetDeckByUserID(tx *sql.Tx, userID string) (*models.Deck, error)
	CreateDeck(tx *sql.Tx, userID string, initialTotalScore int) (*models.Deck, error)
	UpdateDeckTotalScore(tx *sql.Tx, deckID string, totalScore int) error
	DeleteTetriminoPlacementsByDeckID(tx *sql.Tx, deckID string) error
	BulkInsertTetriminoPlacements(tx *sql.Tx, deckID string, placements []models.TetriminoPlacementRequest) error
	GetTetriminoPlacementsByDeckID(tx *sql.Tx, deckID string) ([]models.TetriminoPlacement, error)
}

// deckRepositoryImpl はDeckRepositoryインターフェースの実装です。
type deckRepositoryImpl struct {
	db *sql.DB // プライベートにするため小文字で開始
}

// NewDeckRepository はDeckRepositoryの新しいインスタンスを作成します。
func NewDeckRepository(db *sql.DB) DeckRepository {
	return &deckRepositoryImpl{db: db}
}

// GetDeckByUserID は指定されたユーザーIDのデッキを取得します。
func (r *deckRepositoryImpl) GetDeckByUserID(tx *sql.Tx, userID string) (*models.Deck, error) {
	deck := &models.Deck{}
	// NOTE: トランザクションがnilの場合も考慮 (Read-only操作のため)
	var row *sql.Row
	if tx != nil {
		row = tx.QueryRow("SELECT id, user_id, total_score, created_at, updated_at FROM decks WHERE user_id = $1", userID)
	} else {
		row = r.db.QueryRow("SELECT id, user_id, total_score, created_at, updated_at FROM decks WHERE user_id = $1", userID)
	}

	err := row.Scan(&deck.ID, &deck.UserID, &deck.TotalScore, &deck.CreatedAt, &deck.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // デッキが存在しない場合はnilを返す
	}
	if err != nil {
		return nil, fmt.Errorf("ユーザーIDでデッキを取得できませんでした: %w", err)
	}
	return deck, nil
}

// CreateDeck は新しいデッキを作成します。
func (r *deckRepositoryImpl) CreateDeck(tx *sql.Tx, userID string, initialTotalScore int) (*models.Deck, error) {
	newDeckID := uuid.New().String()
	now := time.Now()
	_, err := tx.Exec(
		"INSERT INTO decks (id, user_id, total_score, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
		newDeckID, userID, initialTotalScore, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("新しいデッキの挿入に失敗しました: %w", err)
	}
	return &models.Deck{
		ID:        newDeckID,
		UserID:    userID,
		TotalScore: initialTotalScore,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// UpdateDeckTotalScore は指定されたデッキのtotal_scoreを更新します。
func (r *deckRepositoryImpl) UpdateDeckTotalScore(tx *sql.Tx, deckID string, totalScore int) error {
	_, err := tx.Exec("UPDATE decks SET total_score = $1, updated_at = NOW() WHERE id = $2", totalScore, deckID)
	if err != nil {
		return fmt.Errorf("デッキの合計スコアの更新に失敗しました: %w", err)
	}
	return nil
}

// DeleteTetriminoPlacementsByDeckID は指定されたデッキIDの全てのテトリミノ配置を削除します。
func (r *deckRepositoryImpl) DeleteTetriminoPlacementsByDeckID(tx *sql.Tx, deckID string) error {
	_, err := tx.Exec("DELETE FROM tetrimino_placements WHERE deck_id = $1", deckID)
	if err != nil {
		return fmt.Errorf("既存のテトリミノ配置の削除に失敗しました: %w", err)
	}
	return nil
}

// BulkInsertTetriminoPlacements は複数のテトリミノ配置を一度に挿入します。
func (r *deckRepositoryImpl) BulkInsertTetriminoPlacements(tx *sql.Tx, deckID string, placements []models.TetriminoPlacementRequest) error {
	if len(placements) == 0 {
		return nil // 挿入するデータがない場合は何もしない
	}

	stmt, err := tx.Prepare(
		`INSERT INTO tetrimino_placements (id, deck_id, tetrimino_type, rotation, start_date, positions, score_potential, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`)
	if err != nil {
		return fmt.Errorf("一括挿入のためのプリペアードステートメントの準備に失敗しました: %w", err)
	}
	defer stmt.Close()

	for _, p := range placements {
		parsedDate, err := time.Parse("2006-01-02", p.StartDate)
		if err != nil {
			return fmt.Errorf("開始日付 '%s' のパースに失敗しました: %w", p.StartDate, err)
		}

		positionsJSON, err := json.Marshal(p.Positions)
		if err != nil {
			return fmt.Errorf("テトリミノタイプ '%s' のポジションのマーシャルに失敗しました: %w", p.Type, err)
		}

		_, err = stmt.Exec(
			uuid.New().String(), deckID, p.Type, p.Rotation, parsedDate, positionsJSON, p.ScorePotential,
		)
		if err != nil {
			return fmt.Errorf("テトリミノ配置の挿入に失敗しました: %w", err)
		}
	}
	return nil
}

// GetTetriminoPlacementsByDeckID は指定されたデッキIDの全てのテトリミノ配置を取得します。
func (r *deckRepositoryImpl) GetTetriminoPlacementsByDeckID(tx *sql.Tx, deckID string) ([]models.TetriminoPlacement, error) {
	placements := []models.TetriminoPlacement{}

	// NOTE: トランザクションがnilの場合も考慮 (Read-only操作のため)
	var rows *sql.Rows
	var err error
	if tx != nil {
		rows, err = tx.Query(
			`SELECT id, deck_id, tetrimino_type, rotation, start_date, positions, score_potential, created_at
			 FROM tetrimino_placements WHERE deck_id = $1`, deckID)
	} else {
		rows, err = r.db.Query(
			`SELECT id, deck_id, tetrimino_type, rotation, start_date, positions, score_potential, created_at
			 FROM tetrimino_placements WHERE deck_id = $1`, deckID)
	}

	if err != nil {
		return nil, fmt.Errorf("テトリミノ配置のクエリに失敗しました: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p models.TetriminoPlacement
		err := rows.Scan(
			&p.ID,
			&p.DeckID,
			&p.TetriminoType,
			&p.Rotation,
			&p.StartDate,
			&p.Positions, // json.RawMessage に直接スキャン
			&p.ScorePotential,
			&p.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("テトリミノ配置のスキャンに失敗しました: %w", err)
		}
		placements = append(placements, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("テトリミノ配置の行イテレーション中にエラーが発生しました: %w", err)
	}

	return placements, nil
}