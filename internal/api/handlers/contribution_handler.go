package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"log"

	"github.com/gorilla/mux"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/github"
)

// ContributionHandler handles HTTP requests related to GitHub contributions.
type ContributionHandler struct {
	GitHubService   *github.GitHubService
	DatabaseService *database.DatabaseService
}

// NewContributionHandler creates a new instance of ContributionHandler.
func NewContributionHandler(ghService *github.GitHubService, dbService *database.DatabaseService) *ContributionHandler {
	return &ContributionHandler{
		GitHubService:   ghService,
		DatabaseService: dbService,
	}
}

// GetDailyContributionsAndSaveHandler fetches a user's daily contributions from GitHub and saves them to the database.
// POST /api/contributions/refresh/{userID} (推奨されるエンドポイント)
// 現在の GET /api/contributions/{userID} の機能をこちらに移動
func (h *ContributionHandler) GetDailyContributionsAndSaveHandler(w http.ResponseWriter, r *http.Request) {
	// OPTIONS メソッドの処理
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	authenticatedUserID, ok := GetUserIDFromContext(r.Context())
	if !ok || authenticatedUserID == "" {
		http.Error(w, "未認証です", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	requestedUserID := vars["userID"]
	if requestedUserID != "" && requestedUserID != authenticatedUserID {
		http.Error(w, "認可されていない操作です", http.StatusForbidden)
		return
	}
	userID := authenticatedUserID

	// ユーザーの GitHub Access Token をデータベースから取得
	githubToken, err := h.DatabaseService.GetGitHubAccessToken(userID)
	if err != nil {
		log.Printf("GetGitHubAccessToken エラー: %v", err)
		http.Error(w, fmt.Sprintf("ユーザーの GitHub Access Token が見つかりません: %v", err), http.StatusUnauthorized)
		return
	}

	// データベースサービスを使って、userID (UUID) からGitHubユーザー名を取得
	githubUsername, err := h.DatabaseService.GetGitHubUsernameByUserID(userID)
	if err != nil {
		log.Printf("GetGitHubUsernameByUserID エラー: %v", err)
		http.Error(w, fmt.Sprintf("ユーザーID '%s' に対応するGitHubユーザー名が見つからないか、データベースエラーが発生しました: %v", userID, err), http.StatusInternalServerError)
		return
	}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -8*7+1) // 8週間 = 56日前

	// 取得したgithubUsernameとユーザーのトークンを使ってGitHub APIを呼び出す
	dailyContributions, err := h.GitHubService.GetDailyContributions(githubUsername, githubToken, startDate, endDate)
	if err != nil {
		fmt.Printf("GitHub貢献データの取得に失敗しました: %v\n", err)
		http.Error(w, fmt.Sprintf("GitHub貢献データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	// 取得したデータをデータベースに保存
	if h.DatabaseService != nil {
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

// GetSavedContributionsHandler fetches saved daily contributions from the database.
// GET /api/contributions/{userID}
func (h *ContributionHandler) GetSavedContributionsHandler(w http.ResponseWriter, r *http.Request) {
	// OPTIONS メソッドの処理
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	authenticatedUserID, ok := GetUserIDFromContext(r.Context())
	if !ok || authenticatedUserID == "" {
		http.Error(w, "未認証です", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	requestedUserID := vars["userID"]
	if requestedUserID != "" && requestedUserID != authenticatedUserID {
		http.Error(w, "認可されていない操作です", http.StatusForbidden)
		return
	}
	userID := authenticatedUserID

	if h.DatabaseService == nil {
		http.Error(w, "DatabaseServiceが初期化されていません。", http.StatusInternalServerError)
		return
	}

	// データベースから保存済みの貢献データを取得
	dailyContributions, err := h.DatabaseService.GetContributionsByUserID(userID)
	if err != nil {
		fmt.Printf("保存済み貢献データの取得に失敗しました: %v\n", err)
		http.Error(w, fmt.Sprintf("保存済み貢献データの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dailyContributions); err != nil {
		fmt.Printf("レスポンスのJSONエンコードに失敗しました: %v\n", err)
		http.Error(w, "レスポンスのJSONエンコードに失敗しました", http.StatusInternalServerError)
	}
}
