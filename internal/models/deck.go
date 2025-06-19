package models

import "time"

// Deck はdecksテーブルのレコードに対応する構造体です。
type Deck struct {
    ID          string    `json:"id"`
    UserID      string    `json:"userId"`      // ユーザーごとに1つのデッキを保証
    TotalScore  int       `json:"totalScore"`  // このデッキに含まれる全ブロックの合計ポテンシャルスコア
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

// DeckResponse はAPIレスポンスでデッキ情報を返す際に使用できます。
// 現時点ではDeck構造体と同じですが、将来的にAPI固有のフィールドを追加する可能性があります。
type DeckResponse struct {
    ID          string    `json:"id"`
    UserID      string    `json:"userId"`
    TotalScore  int       `json:"totalScore"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}