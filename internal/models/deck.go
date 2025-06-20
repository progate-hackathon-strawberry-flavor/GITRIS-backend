package models

import (
	"encoding/json"
	"time"
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