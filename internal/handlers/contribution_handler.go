package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services"
)

// ContributionHandler handles HTTP requests related to GitHub contributions.
type ContributionHandler struct {
	GitHubService *services.GitHubService
	DatabaseService *services.DatabaseService // DatabaseService を追加
}

// NewContributionHandler creates a new instance of ContributionHandler.
func NewContributionHandler(ghService *services.GitHubService, dbService *services.DatabaseService) *ContributionHandler {
	return &ContributionHandler{
		GitHubService: ghService,
		DatabaseService: dbService, // 初期化時に設定
	}
}

// GetDailyContributionsHandler handles the request to fetch a user's daily contributions.
// GET /api/contributions/{username}
func (h *ContributionHandler) GetDailyContributionsHandler(w http.ResponseWriter, r *http.Request) {
	// ここでは簡単化のため、クエリパラメータからユーザー名を取得します。
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "ユーザー名が指定されていません (例: /api/contributions?username=your_github_username)", http.StatusBadRequest)
		return
	}

	// **** 修正: ここを Supabase の users テーブルに存在する有効な UUID に変更してください ****
	// これはデバッグおよびデータベース保存テスト用の一時的な対応です。
	// 最終的には、Supabase認証フローを通じて取得したユーザーの auth.uid() を使用するように変更が必要です。
	// 例: userID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11" // あなたのSupabaseユーザーIDに置き換えてください
	// ここでは例として仮のUUIDを置いていますが、必ずご自身のデータベースにあるUUIDに置き換えてください。
	userID := "ここに"

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		fmt.Println("警告: GITHUB_TOKEN 環境変数が設定されていません。")
        http.Error(w, "サーバーサイドにGitHub Personal Access Tokenが設定されていません。", http.StatusInternalServerError)
        return
	}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -8*7) // 8週間 = 56日前

	dailyContributions, err := h.GitHubService.GetDailyContributions(username, githubToken, startDate, endDate)
	if err != nil {
		fmt.Printf("貢献データの取得に失敗しました: %v\n", err)
		http.Error(w, fmt.Sprintf("貢献データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	if h.DatabaseService != nil {
		err = h.DatabaseService.SaveContributions(userID, dailyContributions)
		if err != nil {
			fmt.Printf("貢献データのデータベース保存に失敗しました: %v\n", err)
			http.Error(w, fmt.Sprintf("貢献データのデータベース保存に失敗しました: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Printf("ユーザー %s の貢献データをデータベースに保存しました。\n", username)
	} else {
		fmt.Println("警告: DatabaseServiceが初期化されていません。貢献データはデータベースに保存されません。")
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dailyContributions); err != nil {
		fmt.Printf("レスポンスのJSONエンコードに失敗しました: %v\n", err)
		http.Error(w, "レスポンスのJSONエンコードに失敗しました", http.StatusInternalServerError)
	}
}
