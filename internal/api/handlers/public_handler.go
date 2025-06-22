package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
)

// PublicHandler handles public API endpoints
type PublicHandler struct {
	DatabaseService *database.DatabaseService
}

// NewPublicHandler creates a new instance of PublicHandler
func NewPublicHandler(dbService *database.DatabaseService) *PublicHandler {
	return &PublicHandler{
		DatabaseService: dbService,
	}
}

func PublicHandlerFunc(w http.ResponseWriter, r *http.Request) {
	log.Println("Request to public endpoint: /api/public")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Hello, this is public content! (From /api/public)")
}

// GetUserDisplayNameHandler fetches the display name for a given user ID.
// GET /api/user/{userID}/display-name
func (h *PublicHandler) GetUserDisplayNameHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["userID"]

	if userID == "" {
		http.Error(w, "ユーザーIDが指定されていません", http.StatusBadRequest)
		return
	}

	displayName := h.DatabaseService.GetUserDisplayNameByUserID(userID)
	
	response := map[string]string{
		"userID":      userID,
		"displayName": displayName,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("GetUserDisplayNameHandler: JSONエンコードエラー: %v", err)
		http.Error(w, "レスポンスの生成に失敗しました", http.StatusInternalServerError)
	}
}
