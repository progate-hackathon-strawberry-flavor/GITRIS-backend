package models

import (
	"encoding/json" // encoding/json をインポート
	"time"
)

// Position はtetrimino_placementsテーブルのpositions JSONBカラム内の構造を定義します。
type Position struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Score int `json:"score"`
}

// tetriminoPlacement はtetrimino_placementsテーブルのレコードに対応する構造体です。
type TetriminoPlacement struct {
	ID           string          `json:"id"`             // UUID
	DeckID       string          `json:"deckId"`         // UUID
	TetriminoType string          `json:"type"`           // 'I', 'O', 'T', 'S', 'Z', 'J', 'L'
	Rotation     int             `json:"rotation"`       // 0, 90, 180, 270
	StartDate    time.Time       `json:"startDate"`      // 配置基準となる日付 (YYYY-MM-DD)
	Positions    json.RawMessage `json:"positions"`      // JSONBとしてDBに保存される (json.RawMessageでRaw JSONを扱う)
	ScorePotential int             `json:"scorePotential"` // このテトリミノ単体での獲得可能スコア
	CreatedAt    time.Time       `json:"createdAt"`      // レコード作成日時
}

// tetriminoPlacementRequest はデッキ保存APIへのリクエストボディのtetriminos配列内の要素を定義します。
type TetriminoPlacementRequest struct {
	Type         string     `json:"type"`
	Rotation     int        `json:"rotation"`
	StartDate    string     `json:"startDate"` // McClellan-MM-DD形式の文字列
	Positions    []Position `json:"positions"` // JSONBに保存されるデータ構造
	ScorePotential int        `json:"scorePotential"`
}

// DeckSaveRequest はデッキ保存APIへのリクエストボディ全体を定義します。
type DeckSaveRequest struct {
	UserID    string                      `json:"userId"`    // 認証されたユーザーのID。フロントエンドから渡されるが、バックエンドで検証済みIDを優先
	Tetriminos []TetriminoPlacementRequest `json:"tetriminos"`
}