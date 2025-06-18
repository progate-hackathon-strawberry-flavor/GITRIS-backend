package models

type Contribution struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	DeckID    string `json:"deck_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ContributionResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	DeckID    string `json:"deck_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type DailyContribution struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
} 