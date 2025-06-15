package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"os"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services" // your_module_name は go.mod で定義するモジュール名に置き換えてください
)

// ContributionHandler handles HTTP requests related to GitHub contributions.
type ContributionHandler struct {
	GitHubService *services.GitHubService
}

// NewContributionHandler creates a new instance of ContributionHandler.
func NewContributionHandler(ghService *services.GitHubService) *ContributionHandler {
	return &ContributionHandler{
		GitHubService: ghService,
	}
}

// GetDailyContributionsHandler handles the request to fetch a user's daily contributions.
// GET /api/contributions/{username}
func (h *ContributionHandler) GetDailyContributionsHandler(w http.ResponseWriter, r *http.Request) {
	// ここでは簡単化のため、クエリパラメータからユーザー名を取得します。
	// 実際にはURLパスパラメータや認証情報から取得することが多いでしょう。
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "ユーザー名が指定されていません (例: /api/contributions?username=your_username)", http.StatusBadRequest)
		return
	}

	// 環境変数からGitHub Personal Access Tokenを取得する
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		fmt.Println("警告: GITHUB_TOKEN 環境変数が設定されていません。レート制限に引っかかる可能性があります。")
		// GitHub Tokenがない場合は、エラーとするか、非認証リクエストとして続行するか、プロジェクトの要件による
		// ここではエラーとしています。
		http.Error(w, "サーバーサイドにGitHub Personal Access Tokenが設定されていません。", http.StatusInternalServerError)
		return
	}

	// 現在の日付から過去8週間を計算
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -8*7) // 8週間 = 56日前

	// サービス層を呼び出してデータを取得
	dailyContributions, err := h.GitHubService.GetDailyContributions(username, githubToken, startDate, endDate)
	if err != nil {
		fmt.Printf("貢献データの取得に失敗しました: %v\n", err)
		http.Error(w, fmt.Sprintf("貢献データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	// レスポンスをJSON形式で返す
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dailyContributions); err != nil {
		fmt.Printf("レスポンスのJSONエンコードに失敗しました: %v\n", err)
		http.Error(w, "レスポンスのJSONエンコードに失敗しました", http.StatusInternalServerError)
	}
}
