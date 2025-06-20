package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"         // プロジェクトのルートパスに合わせて修正
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"                 // プロジェクトのルートパスに合わせて修正
	services "github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/services/deck" // プロジェクトのルートパスに合わせて修正
)

// DeckSaveHandler はデッキ保存APIのエンドポイントを処理します。
type DeckSaveHandler struct {
	DeckService services.DeckService
}

// NewDeckSaveHandler はDeckSaveHandlerの新しいインスタンスを作成します。
func NewDeckSaveHandler(s services.DeckService) *DeckSaveHandler {
	return &DeckSaveHandler{DeckService: s}
}

// ServeHTTP は http.Handler インターフェースを実装します。
// これにより、http.Handle() 関数で直接使用できます。
func (h *DeckSaveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// POSTメソッドのみを受け入れます
	if r.Method != http.MethodPost {
		http.Error(w, "許可されていないメソッド", http.StatusMethodNotAllowed)
		return
	}

	// ContextからユーザーIDを取得します (AuthMiddlewareが設定されている前提)
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		log.Println("エラー: デッキ保存ハンドラでユーザーIDがコンテキストに見つかりませんでした。認証ミドルウェアが正しく動作していることを確認してください。")
		http.Error(w, "未認証: ユーザーIDが見つかりません", http.StatusUnauthorized)
		return
	}
	log.Printf("認証済みユーザーID: %s がデッキ保存リクエストを送信しました。", userID)


	// リクエストボディをパースします
	var req models.DeckSaveRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("リクエストボディのパースに失敗しました: %v", err)
		http.Error(w, "不正なリクエスト: 無効なリクエストボディです", http.StatusBadRequest)
		return
	}

	// セキュリティ検証: リクエストボディのユーザーIDと認証済みユーザーIDが一致するか確認します。
	// クライアントから送られてくるuserIDはあくまで参考とし、JWTから取得した認証済みuserIDを信頼すべきです。
	if req.UserID != userID {
		log.Printf("不正なデッキ保存試行: リクエストユーザーID %s vs 認証済みユーザーID %s", req.UserID, userID)
		http.Error(w, "未認証: ユーザーIDが一致しません", http.StatusUnauthorized)
		return
	}

	// デッキ保存のビジネスロジックを実行します
	err = h.DeckService.SaveDeck(userID, req.Tetriminos)
	if err != nil {
		log.Printf("ユーザー %s のデッキ保存に失敗しました: %v", userID, err)
		// エラーの種類に応じて適切なHTTPステータスを返すように改善可能
		http.Error(w, "内部サーバーエラー: デッキの保存に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "デッキが正常に保存されました"})
}