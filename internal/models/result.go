package models

import (
	"time"
)

// Result はresultsテーブルのレコードに対応する構造体です。
type Result struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`    // UUID
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// ResultResponse はAPI レスポンス用の構造体です。
type ResultResponse struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
	Rank      int       `json:"rank"` // ランキング順位
}

// ResultRequest はリザルト保存リクエスト用の構造体です。
type ResultRequest struct {
	UserID string `json:"user_id"`
	Score  int    `json:"score"`
} 