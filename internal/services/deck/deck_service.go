package services

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database" // プロジェクトのルートパスに合わせて修正
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models" // modelsパッケージをインポート
	// プロジェクトのルートパスに合わせて修正
)

// DeckService はデッキ関連のビジネスロジックを定義するインターフェースです。
type DeckService interface {
	SaveDeck(userID string, tetriminos []models.TetriminoPlacementRequest) error
}

// deckServiceImpl はDeckServiceインターフェースの実装です。
type deckServiceImpl struct {
	db          *sql.DB
	deckRepo    database.DeckRepository
}

// NewDeckService はDeckServiceの新しいインスタンスを作成します。
func NewDeckService(db *sql.DB, deckRepo database.DeckRepository) DeckService {
	return &deckServiceImpl{
		db:          db,
		deckRepo:    deckRepo,
	}
}

// SaveDeck はユーザーのデッキデータを保存するビジネスロジックを実行します。
// 既存のデッキ配置を削除し、新しい配置を挿入し、デッキの合計スコアを更新します。
func (s *deckServiceImpl) SaveDeck(userID string, tetriminos []models.TetriminoPlacementRequest) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	defer func() {
		if r := recover(); r != nil { // パニック発生時にリカバリー
			tx.Rollback()
			panic(r)
		} else if err != nil { // 関数内でエラーが発生した場合のみロールバック
			tx.Rollback()
		}
	}()

	// ユーザーの既存のデッキを取得または新規作成します
	deck, err := s.deckRepo.GetDeckByUserID(tx, userID)
	if err != nil {
		return fmt.Errorf("デッキの取得に失敗しました: %w", err)
	}

	var deckID string
	if deck == nil {
		// デッキが存在しない場合、新規作成します
		newDeck, err := s.deckRepo.CreateDeck(tx, userID, 0) // total_scoreは後で更新
		if err != nil {
			return fmt.Errorf("新しいデッキの作成に失敗しました: %w", err)
		}
		deckID = newDeck.ID
		log.Printf("ユーザー %s の新しいデッキが作成されました: %s", userID, deckID)
	} else {
		deckID = deck.ID
	}

	// 該当ユーザーの既存のtetrimino_placementsレコードを全て削除します
	err = s.deckRepo.DeletetetriminoPlacementsByDeckID(tx, deckID)
	if err != nil {
		return fmt.Errorf("既存のテトリミノ配置の削除に失敗しました: %w", err)
	}
	log.Printf("デッキ %s の既存のテトリミノ配置が削除されました。", deckID)

	// 受け取ったtetriminos配列の各要素をtetrimino_placementsテーブルに新規レコードとして挿入します
	err = s.deckRepo.BulkInserttetriminoPlacements(tx, deckID, tetriminos)
	if err != nil {
		return fmt.Errorf("テトリミノ配置の挿入に失敗しました: %w", err)
	}
	log.Printf("デッキ %s に %d 個のテトリミノ配置が挿入されました。", deckID, len(tetriminos))

	// decksテーブルのtotal_scoreを更新します
	newTotalScore := 0
	for _, t := range tetriminos {
		newTotalScore += t.ScorePotential
	}
	err = s.deckRepo.UpdateDeckTotalScore(tx, deckID, newTotalScore)
	if err != nil {
		return fmt.Errorf("デッキの合計スコアの更新に失敗しました: %w", err)
	}
	log.Printf("デッキ %s のtotal_scoreが %d に更新されました。", deckID, newTotalScore)

	// トランザクションをコミットします
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("トランザクションのコミットに失敗しました: %w", err)
	}

	log.Println("デッキが正常に保存されました。")
	return nil
}