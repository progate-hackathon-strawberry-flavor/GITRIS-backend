package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
)

// ResultHandler はゲーム結果関連のハンドラーを管理する構造体です。
type ResultHandler struct {
	resultRepo database.ResultRepository
}

// NewResultHandler は新しいResultHandlerインスタンスを作成します。
func NewResultHandler(resultRepo database.ResultRepository) *ResultHandler {
	return &ResultHandler{
		resultRepo: resultRepo,
	}
}

// GetTopResults は上位ランキングを取得するハンドラーです。
// GET /api/results?limit=50
func (h *ResultHandler) GetTopResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// limitパラメータを取得（デフォルト50）
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	results, err := h.resultRepo.GetTopResults(limit)
	if err != nil {
		log.Printf("ゲーム結果取得エラー: %v", err)
		http.Error(w, "ゲーム結果取得に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"results": results,
	})
}

// PostScore はスコアを保存するハンドラーです。
// POST /api/results
func (h *ResultHandler) PostScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authenticatedUserID, ok := GetUserIDFromContext(r.Context())
	if !ok || authenticatedUserID == "" {
		http.Error(w, "未認証です", http.StatusUnauthorized)
		return
	}

	var req models.ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "無効なリクエストボディです", http.StatusBadRequest)
		return
	}

	// JWT の userID を正として扱う。body に user_id があれば一致しているかだけ確認する。
	if req.UserID != "" && req.UserID != authenticatedUserID {
		http.Error(w, "認可されていない user_id です", http.StatusForbidden)
		return
	}
	req.UserID = authenticatedUserID
	if req.Score < 0 {
		http.Error(w, "スコアは0以上である必要があります", http.StatusBadRequest)
		return
	}

	// スコアを保存
	result, err := h.resultRepo.CreateResult(nil, authenticatedUserID, req.Score)
	if err != nil {
		log.Printf("スコア保存エラー: %v", err)
		http.Error(w, "スコア保存に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// GetUserResult は指定したユーザーのランキングを取得するハンドラーです。
// GET /api/results/user/{user_id}
func (h *ResultHandler) GetUserResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// URLからuser_idを抽出（パスパラメータ）
	userID := r.URL.Path[len("/api/results/user/"):]
	if userID == "" {
		http.Error(w, "user_idが指定されていません", http.StatusBadRequest)
		return
	}

	userResult, err := h.resultRepo.GetUserRanking(userID)
	if err != nil {
		log.Printf("ユーザー結果取得エラー: %v", err)
		http.Error(w, "ユーザー結果取得に失敗しました", http.StatusInternalServerError)
		return
	}

	if userResult == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result":  nil,
			"message": "ユーザーのスコアが見つかりません",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  userResult,
	})
}
