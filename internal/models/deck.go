package models

import (
	"encoding/json"
	"math/rand"
	"time"

	"log"

	"github.com/google/uuid"
)

// Deck はdecksテーブルのレコードに対応する構造体です。
type Deck struct {
    ID          string    `json:"id"`
    UserID      string    `json:"userId"`      // ユーザーごとに1つのデッキを保証
    TotalScore  int       `json:"totalScore"`  // このデッキに含まれる全ブロックの合計ポテンシャルスコア
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

// DeckWithPlacements はデッキとその配置されたテトリミノの詳細を含むAPIレスポンス用の構造体です。
type DeckWithPlacements struct {
	Deck       *Deck                   `json:"deck"`
	Placements []TetriminoPlacementAPI `json:"placements"` // APIレスポンス用の配置情報
}

// TetriminoPlacementAPI はAPIレスポンスで返すためのテトリミノ配置情報です。
// PositionsはJSONBデータとしてそのまま返すためjson.RawMessageを使用します。
type TetriminoPlacementAPI struct {
	ID           string          `json:"id"`
	TetriminoType string          `json:"type"`
	Rotation     int             `json:"rotation"`
	StartDate    string          `json:"startDate"` // YYYY-MM-DD 形式で文字列として返す
	Positions    json.RawMessage `json:"positions"` // DBから取得したJSONBをそのまま出力
	ScorePotential int             `json:"scorePotential"`
	// CreatedAt は必要に応じて含める
}

// CreateGuestDeck はゲスト用のまばらな色分布を持つランダムデッキを生成します
// 5段階の色レベル（very-low, low, medium, high, very-high）すべてが含まれるようにスコアを分散させます
// 7-bagシステムに対応するため、各テトリミノタイプを複数個作成します
func CreateGuestDeck(userID string) *DeckWithPlacements {
	// ゲスト用デッキID
	deckID := uuid.New().String()
	
	// 7-bagシステムに対応するため、各テトリミノタイプを3個ずつ作成（計21個）
	// これにより同じタイプが複数回選ばれても異なるスコア分布のピースが使用される
	tetriminoTypes := []string{"I", "O", "T", "S", "Z", "J", "L"}
	placements := make([]TetriminoPlacementAPI, 0, 21) // 7タイプ × 3個 = 21個
	totalScore := 0
	
	log.Printf("[CreateGuestDeck] Generating colorful varied deck with multiple pieces for guest user %s", userID)
	
	// 各テトリミノタイプごとに3個ずつ作成し、それぞれ異なる主要色レベルを持つ
	for _, tetType := range tetriminoTypes {
		for pieceVariant := 0; pieceVariant < 3; pieceVariant++ {
			// 各バリアント用の主要色レベルを計算（0-4をローテーション）
			mainColorLevel := (pieceVariant * 2) % 5 // 0, 2, 4, 1, 3, 0, 2 ... の順序
			
			placement := generateRandomTetriminoPlacementWithMainColor(deckID, tetType, mainColorLevel)
			placements = append(placements, placement)
			totalScore += placement.ScorePotential
			
			log.Printf("[CreateGuestDeck] Created %s variant %d with main color level %d, score potential: %d", 
				tetType, pieceVariant, mainColorLevel, placement.ScorePotential)
		}
	}
	
	log.Printf("[CreateGuestDeck] Generated deck with %d pieces, total score %d, ensuring colorful variety", 
		len(placements), totalScore)
	
	// ゲスト用デッキオブジェクト
	deck := &Deck{
		ID:         deckID,
		UserID:     userID,
		TotalScore: totalScore,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	
	return &DeckWithPlacements{
		Deck:       deck,
		Placements: placements,
	}
}

// generateRandomTetriminoPlacement はランダムなまばらスコアテトリミノ配置を生成します
// 色がまばらになるように幅広いスコア範囲を使用します
func generateRandomTetriminoPlacement(deckID, tetType string) TetriminoPlacementAPI {
	// テトリミノの形状定義（簡易版）
	tetriminoShapes := map[string][][2]int{
		"I": {{0, 0}, {1, 0}, {2, 0}, {3, 0}},
		"O": {{0, 0}, {1, 0}, {0, 1}, {1, 1}},
		"T": {{1, 0}, {0, 1}, {1, 1}, {2, 1}},
		"S": {{1, 0}, {2, 0}, {0, 1}, {1, 1}},
		"Z": {{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		"J": {{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		"L": {{2, 0}, {0, 1}, {1, 1}, {2, 1}},
	}
	
	shape := tetriminoShapes[tetType]
	positions := make([]Position, len(shape))
	scorePotential := 0
	
	// 色分けが確実にまばらになるように、5段階の色レベルから選択
	// score-very-low: 0-4, score-low: 5-19, score-medium: 20-49, score-high: 50-99, score-very-high: 100+
	scoreRanges := []struct {
		min int
		max int
		description string
	}{
		{0, 4, "very-low"},     // 最暗色
		{5, 19, "low"},         // 暗色
		{20, 49, "medium"},     // 中色
		{50, 99, "high"},       // 明色
		{100, 150, "very-high"}, // 最明色
	}
	
	// 各ブロックに異なる色レベルのスコアを割り当て（まばらになるように）
	// 同じピース内でも色の多様性を確保するため、ブロックごとに異なる色レベルを推奨
	usedRanges := make(map[int]bool) // 使用済み色レベルを追跡
	
	for i, block := range shape {
		var rangeIndex int
		var selectedRange struct {
			min int
			max int
			description string
		}
		
		// 最初の2ブロックは確実に異なる色レベルを選択
		if i < 2 && len(usedRanges) < len(scoreRanges) {
			// まだ使っていない色レベルから選択
			availableRanges := []int{}
			for idx := range scoreRanges {
				if !usedRanges[idx] {
					availableRanges = append(availableRanges, idx)
				}
			}
			if len(availableRanges) > 0 {
				rangeIndex = availableRanges[rand.Intn(len(availableRanges))]
			} else {
				rangeIndex = rand.Intn(len(scoreRanges))
			}
		} else {
			// 3ブロック目以降は70%の確率で未使用、30%の確率で完全ランダム
			if rand.Float32() < 0.7 && len(usedRanges) < len(scoreRanges) {
				// 未使用の色レベルから選択
				availableRanges := []int{}
				for idx := range scoreRanges {
					if !usedRanges[idx] {
						availableRanges = append(availableRanges, idx)
					}
				}
				if len(availableRanges) > 0 {
					rangeIndex = availableRanges[rand.Intn(len(availableRanges))]
				} else {
					rangeIndex = rand.Intn(len(scoreRanges))
				}
			} else {
				// 完全ランダム
				rangeIndex = rand.Intn(len(scoreRanges))
			}
		}
		
		selectedRange = scoreRanges[rangeIndex]
		usedRanges[rangeIndex] = true
		
		// 選択された範囲内でランダムスコアを生成
		score := rand.Intn(selectedRange.max-selectedRange.min+1) + selectedRange.min
		
		positions[i] = Position{
			X:     block[0],
			Y:     block[1],
			Score: score,
		}
		scorePotential += score
		
		// デバッグ用ログ
		log.Printf("[generateRandomTetriminoPlacement] %s Block %d: score=%d (%s level)", 
			tetType, i, score, selectedRange.description)
	}
	
	positionsJSON, _ := json.Marshal(positions)
	
	return TetriminoPlacementAPI{
		ID:            uuid.New().String(),
		TetriminoType: tetType,
		Rotation:      0, // 常に0度
		StartDate:     time.Now().Format("2006-01-02"),
		Positions:     positionsJSON,
		ScorePotential: scorePotential,
	}
}

// generateRandomTetriminoPlacementWithMainColor はランダムなまばらスコアテトリミノ配置を生成します
// 色がまばらになるように幅広いスコア範囲を使用します
// 主要色レベルを指定して配置を生成
func generateRandomTetriminoPlacementWithMainColor(deckID, tetType string, mainColorLevel int) TetriminoPlacementAPI {
	log.Printf("[generateRandomTetriminoPlacementWithMainColor] Starting generation for %s with main color level %d", tetType, mainColorLevel)
	// テトリミノの形状定義（簡易版）
	tetriminoShapes := map[string][][2]int{
		"I": {{0, 0}, {1, 0}, {2, 0}, {3, 0}},
		"O": {{0, 0}, {1, 0}, {0, 1}, {1, 1}},
		"T": {{1, 0}, {0, 1}, {1, 1}, {2, 1}},
		"S": {{1, 0}, {2, 0}, {0, 1}, {1, 1}},
		"Z": {{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		"J": {{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		"L": {{2, 0}, {0, 1}, {1, 1}, {2, 1}},
	}
	
	shape := tetriminoShapes[tetType]
	positions := make([]Position, len(shape))
	scorePotential := 0
	
	// 色分けが確実にまばらになるように、5段階の色レベルから選択
	// score-very-low: 0-4, score-low: 5-19, score-medium: 20-49, score-high: 50-99, score-very-high: 100+
	scoreRanges := []struct {
		min int
		max int
		description string
	}{
		{0, 4, "very-low"},     // 最暗色
		{5, 19, "low"},         // 暗色
		{20, 49, "medium"},     // 中色
		{50, 99, "high"},       // 明色
		{100, 150, "very-high"}, // 最明色
	}
	
	// 各ブロックに異なる色レベルのスコアを割り当て（まばらになるように）
	// 同じピース内でも色の多様性を確保するため、ブロックごとに異なる色レベルを推奨
	usedRanges := make(map[int]bool) // 使用済み色レベルを追跡
	
	for i, block := range shape {
		var rangeIndex int
		var selectedRange struct {
			min int
			max int
			description string
		}
		
		// 最初の2ブロックは確実に異なる色レベルを選択
		if i < 2 && len(usedRanges) < len(scoreRanges) {
			// まだ使っていない色レベルから選択
			availableRanges := []int{}
			for idx := range scoreRanges {
				if !usedRanges[idx] {
					availableRanges = append(availableRanges, idx)
				}
			}
			if len(availableRanges) > 0 {
				rangeIndex = availableRanges[rand.Intn(len(availableRanges))]
			} else {
				rangeIndex = rand.Intn(len(scoreRanges))
			}
		} else {
			// 3ブロック目以降は70%の確率で未使用、30%の確率で完全ランダム
			if rand.Float32() < 0.7 && len(usedRanges) < len(scoreRanges) {
				// 未使用の色レベルから選択
				availableRanges := []int{}
				for idx := range scoreRanges {
					if !usedRanges[idx] {
						availableRanges = append(availableRanges, idx)
					}
				}
				if len(availableRanges) > 0 {
					rangeIndex = availableRanges[rand.Intn(len(availableRanges))]
				} else {
					rangeIndex = rand.Intn(len(scoreRanges))
				}
			} else {
				// 完全ランダム
				rangeIndex = rand.Intn(len(scoreRanges))
			}
		}
		
		selectedRange = scoreRanges[rangeIndex]
		usedRanges[rangeIndex] = true
		
		// 選択された範囲内でランダムスコアを生成
		score := rand.Intn(selectedRange.max-selectedRange.min+1) + selectedRange.min
		
		positions[i] = Position{
			X:     block[0],
			Y:     block[1],
			Score: score,
		}
		scorePotential += score
		
		// デバッグ用ログ
		log.Printf("[generateRandomTetriminoPlacementWithMainColor] %s Block %d: score=%d (%s level)", 
			tetType, i, score, selectedRange.description)
	}
	
	positionsJSON, _ := json.Marshal(positions)
	
	return TetriminoPlacementAPI{
		ID:            uuid.New().String(),
		TetriminoType: tetType,
		Rotation:      0, // 常に0度
		StartDate:     time.Now().Format("2006-01-02"),
		Positions:     positionsJSON,
		ScorePotential: scorePotential,
	}
}