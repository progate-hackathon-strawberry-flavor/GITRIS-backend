package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log" // log パッケージを追加
	"net/http"
	"time"
)

// DailyContribution represents a single day's contribution data.
type DailyContribution struct {
	Date            string
	ContributionCount int
}

// GitHubService provides methods for interacting with the GitHub API.
type GitHubService struct {
	githubAPIURL string
	httpClient   *http.Client
}

// NewGitHubService creates a new instance of GitHubService.
func NewGitHubService() *GitHubService {
	return &GitHubService{
		githubAPIURL: "https://api.github.com/graphql",
		httpClient:   &http.Client{Timeout: 30 * time.Second}, // タイムアウトを少し長くする
	}
}

// GraphQLQuery represents the structure of the GraphQL request body.
type GraphQLQuery struct {
	Query     string    `json:"query"`
	Variables Variables `json:"variables"`
}

// Variables represents the variables for the GraphQL query.
type Variables struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// GitHubGraphQLResponse represents the top-level structure of the GitHub GraphQL API response.
type GitHubGraphQLResponse struct {
	Data struct {
		User *struct { // user が null になる可能性があるのでポインタにする
			ContributionsCollection *struct { // contributionsCollection が null になる可能性があるのでポインタにする
				ContributionCalendar *struct { // contributionCalendar が null になる可能性があるのでポインタにする
					Weeks []struct {
						ContributionDays []struct {
							Date            string `json:"date"`
							ContributionCount int    `json:"contributionCount"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message   string `json:"message"`
		Locations []struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"locations"`
		Path []interface{} `json:"path"`
	} `json:"errors"`
}

// GetDailyContributions fetches daily contribution data for a given GitHub user.
func (s *GitHubService) GetDailyContributions(username, githubToken string, startDate, endDate time.Time) ([]DailyContribution, error) {
	log.Printf("GitHubService: ユーザー '%s' の貢献データを取得開始。期間: %s から %s", username, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// GraphQLクエリの定義: 日ごとのContribution数を取得するためのクエリ
	query := `
		query ($name: String!, $from: DateTime!, $to: DateTime!) {
			user(login: $name) {
				contributionsCollection(from: $from, to: $to) {
					contributionCalendar {
						weeks {
							contributionDays {
								date
								contributionCount
							}
						}
					}
				}
			}
		}
	`

	// 変数の準備
	variables := Variables{
		Name: username,
		From: startDate.Format(time.RFC3339), // ISO 8601フォーマットに変換
		To:   endDate.Format(time.RFC3339),   // ISO 8601フォーマットに変換
	}

	// GraphQLリクエストボディの構築
	graphqlQuery := GraphQLQuery{
		Query:     query,
		Variables: variables,
	}

	requestBody, err := json.Marshal(graphqlQuery)
	if err != nil {
		log.Printf("GitHubService Error: リクエストボディのJSONエンコードに失敗しました: %v", err)
		return nil, fmt.Errorf("リクエストボディのJSONエンコードに失敗しました: %w", err)
	}
	log.Printf("GitHubService Debug: リクエストボディ: %s", string(requestBody))


	// HTTPリクエストの作成
	req, err := http.NewRequest("POST", s.githubAPIURL, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Printf("GitHubService Error: HTTPリクエストの作成に失敗しました: %v", err)
		return nil, fmt.Errorf("HTTPリクエストの作成に失敗しました: %w", err)
	}

	// ヘッダーの設定
	req.Header.Set("Content-Type", "application/json")
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
		log.Println("GitHubService Debug: GitHub Personal Access Tokenがヘッダーに設定されました。")
	} else {
		log.Println("警告: GitHub Personal Access Tokenが提供されていません。レート制限に引っかかる可能性があります。")
	}

	// HTTPクライアントでリクエストを送信
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("GitHubService Error: HTTPリクエストの送信に失敗しました: %v", err)
		return nil, fmt.Errorf("HTTPリクエストの送信に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	// レスポンスボディの読み込み
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("GitHubService Error: レスポンスボディの読み込みに失敗しました: %v", err)
		return nil, fmt.Errorf("レスポンスボディの読み込みに失敗しました: %w", err)
	}
	log.Printf("GitHubService Debug: HTTPステータスコード: %d", resp.StatusCode)
	log.Printf("GitHubService Debug: 生レスポンスボディ: %s", string(body))

	// エラーレスポンスの確認
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub APIからエラーレスポンスが返されました (ステータス: %d): %s", resp.StatusCode, string(body))
	}

	// JSONレスポンスのパース
	var githubResp GitHubGraphQLResponse
	err = json.Unmarshal(body, &githubResp)
	if err != nil {
		log.Printf("GitHubService Error: JSONレスポンスのパースに失敗しました: %v", err)
		return nil, fmt.Errorf("JSONレスポンスのパースに失敗しました: %w", err)
	}
	log.Printf("GitHubService Debug: パース後データ (Errors): %+v", githubResp.Errors)
	// データフィールドがnilでないことを確認してからアクセス
	if githubResp.Data.User != nil && githubResp.Data.User.ContributionsCollection != nil && githubResp.Data.User.ContributionsCollection.ContributionCalendar != nil {
		log.Printf("GitHubService Debug: パース後データ (ContributionCalendar Weeks Count): %d", len(githubResp.Data.User.ContributionsCollection.ContributionCalendar.Weeks))
	} else {
		log.Println("GitHubService Debug: パース後データ: User, ContributionsCollection, または ContributionCalendarがnullです。")
	}

	// GraphQLエラーがある場合は表示
	if len(githubResp.Errors) > 0 {
		errMsg := "GraphQLエラー:\n"
		for _, e := range githubResp.Errors {
			errMsg += fmt.Sprintf("- %s\n", e.Message)
		}
		log.Printf("GitHubService Error: %s", errMsg)
		return nil, fmt.Errorf("%s",errMsg)
	}

	// データが取得できたか確認
	if githubResp.Data.User == nil || githubResp.Data.User.ContributionsCollection == nil || githubResp.Data.User.ContributionsCollection.ContributionCalendar == nil {
		log.Printf("GitHubService Info: ユーザーの貢献データが見つからないか、クエリの結果が空です。username: %s", username)
		return []DailyContribution{}, nil // 空のスライスを返す
	}


	// 取得したContributionデータをDailyContributionスライスに変換
	var dailyContributions []DailyContribution
	for _, week := range githubResp.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, day := range week.ContributionDays {
			dailyContributions = append(dailyContributions, DailyContribution{
				Date:            day.Date,
				ContributionCount: day.ContributionCount,
			})
		}
	}

	log.Printf("GitHubService Info: ユーザー '%s' の貢献データ %d 日分を取得しました。", username, len(dailyContributions))
	return dailyContributions, nil
}
