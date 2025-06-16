package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux" // gorilla/mux をインポート
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services"
)

// ContributionHandler handles HTTP requests related to GitHub contributions.
type ContributionHandler struct {
	GitHubService *services.GitHubService
	DatabaseService *services.DatabaseService
}

// NewContributionHandler creates a new instance of ContributionHandler.
func NewContributionHandler(ghService *services.GitHubService, dbService *services.DatabaseService) *ContributionHandler {
	return &ContributionHandler{
		GitHubService: ghService,
		DatabaseService: dbService,
	}
}

// GetDailyContributionsHandler handles the request to fetch a user's daily contributions.
// GET /api/contributions/{userID}
func (h *ContributionHandler) GetDailyContributionsHandler(w http.ResponseWriter, r *http.Request) {
	// gorilla/mux の Vars 関数を使ってURLパスからuserIDを取得
	vars := mux.Vars(r)
	userID := vars["userID"] // URLパスパラメータの名前は "userID" とする

	if userID == "" {
		http.Error(w, "ユーザーIDが指定されていません (例: /api/contributions/{UUID})", http.StatusBadRequest)
		return
	}

	// SupabaseのUUIDが正しい形式か基本的なバリデーション (任意)
	// 例: if !isValidUUID(userID) { http.Error(...); return }

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Println("警告: GITHUB_TOKEN 環境変数が設定されていません。")
        http.Error(w, "サーバーサイドにGitHub Personal Access Tokenが設定されていません。", http.StatusInternalServerError)
        return
	}

	// データベースサービスを使って、userID (UUID) からGitHubユーザー名を取得
	githubUsername, err := h.DatabaseService.GetGitHubUsernameByUserID(userID)
	if err != nil {
		fmt.Printf("GetGitHubUsernameByUserID エラー: %v\n", err)
		http.Error(w, fmt.Sprintf("ユーザーID '%s' に対応するGitHubユーザー名が見つからないか、データベースエラーが発生しました: %v", userID, err), http.StatusInternalServerError)
		return
	}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -8*7) // 8週間 = 56日前

	// 取得したgithubUsernameを使ってGitHub APIを呼び出す
	dailyContributions, err := h.GitHubService.GetDailyContributions(githubUsername, githubToken, startDate, endDate)
	if err != nil {
		fmt.Printf("GitHub貢献データの取得に失敗しました: %v\n", err)
		http.Error(w, fmt.Sprintf("GitHub貢献データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	if h.DatabaseService != nil {
		// データベースへの保存には、元の userID (UUID) を使用
		err = h.DatabaseService.SaveContributions(userID, dailyContributions)
		if err != nil {
			fmt.Printf("貢献データのデータベース保存に失敗しました: %v\n", err)
			http.Error(w, fmt.Sprintf("貢献データのデータベース保存に失敗しました: %v", err), http.StatusInternalServerError)
			return
		}
		fmt.Printf("ユーザー %s (GitHub: %s) の貢献データをデータベースに保存しました。\n", userID, githubUsername)
	} else {
		fmt.Println("警告: DatabaseServiceが初期化されていません。貢献データはデータベースに保存されません。")
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dailyContributions); err != nil {
		fmt.Printf("レスポンスのJSONエンコードに失敗しました: %v\n", err)
		http.Error(w, "レスポンスのJSONエンコードに失敗しました", http.StatusInternalServerError)
	}
}
