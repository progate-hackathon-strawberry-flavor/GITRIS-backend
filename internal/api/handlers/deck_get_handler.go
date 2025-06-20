package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux" // mux.Vars を使用するためインポート
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware" // プロジェクトのルートパスに合わせて修正
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/deck"  // deckサービスパッケージ
)

// DeckGetHandler はデッキ取得APIのエンドポイントを処理します。
type DeckGetHandler struct {
	DeckService services.DeckService
}

// NewDeckGetHandler はDeckGetHandlerの新しいインスタンスを作成します。
func NewDeckGetHandler(s services.DeckService) *DeckGetHandler {
	return &DeckGetHandler{DeckService: s}
}

// ServeHTTP は http.Handler インターフェースを実装します。
func (h *DeckGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// GETメソッドのみを受け入れます
	if r.Method != http.MethodGet {
		http.Error(w, "許可されていないメソッド", http.StatusMethodNotAllowed)
		return
	}

	// パスパラメータからuserIDを取得します
	vars := mux.Vars(r)
	requestedUserID := vars["userID"] // URLから取得したユーザーID
	if requestedUserID == "" {
		http.Error(w, "ユーザーIDが指定されていません。", http.StatusBadRequest)
		return
	}
	log.Printf("リクエストされたユーザーID (URL): %s", requestedUserID)

	// Contextから認証済みユーザーIDを取得します (AuthMiddlewareが設定されている前提)
	authenticatedUserID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		log.Println("エラー: デッキ取得ハンドラで認証済みユーザーIDがコンテキストに見つかりませんでした。")
		http.Error(w, "未認証: ユーザーIDが見つかりません", http.StatusUnauthorized)
		return
	}
	log.Printf("認証済みユーザーID (JWT): %s", authenticatedUserID)

	// セキュリティ検証: リクエストされたユーザーIDと認証済みユーザーIDが一致するか確認します。
	if requestedUserID != authenticatedUserID {
		log.Printf("認可エラー: リクエストユーザーID %s は認証済みユーザーID %s と一致しません。", requestedUserID, authenticatedUserID)
		http.Error(w, "認可されていない操作: 他のユーザーのデッキにはアクセスできません", http.StatusForbidden)
		return
	}

	// デッキと配置のビジネスロジックを実行します
	deckWithPlacements, err := h.DeckService.GetDeckWithPlacementsByUserID(authenticatedUserID)
	if err != nil {
		log.Printf("ユーザー %s のデッキ取得に失敗しました: %v", authenticatedUserID, err)
		http.Error(w, "内部サーバーエラー: デッキ情報の取得に失敗しました", http.StatusInternalServerError)
		return
	}

	if deckWithPlacements == nil || deckWithPlacements.Deck == nil {
		// デッキが存在しない場合、404 Not Found を返す
		http.Error(w, "デッキが見つかりませんでした", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(deckWithPlacements); err != nil {
		log.Printf("レスポンスのJSONエンコードに失敗しました: %v", err)
		http.Error(w, "内部サーバーエラー", http.StatusInternalServerError)
	}
	log.Printf("ユーザー %s のデッキが正常に取得され、返されました。", authenticatedUserID)
}