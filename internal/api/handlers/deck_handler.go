// backend/internal/handlers/deck_handler.go (保護されたハンドラの例)
package handlers

// ログインが必要なAPI検証のために一旦おいてる。
// 実際のDeckではない

import (
	"fmt"
	"log"
	"net/http"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/api/middleware"
	// "your_project_name/backend/internal/services" // ビジネスロジックを担うサービス層
	// ここに Supabase と連携するためのコードや、デッキデータを扱うためのロジックを実装します。
)

// GetDecksForUser は、認証されたユーザーのデッキリストを返すハンドラ関数です。
func GetDecksForUser(w http.ResponseWriter, r *http.Request) {
	// ContextからユーザーIDを取得
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		// ミドルウェアが正しく動作していれば、ここには到達しないはずですが、念のため
		log.Println("Error: User ID not found in context for GetDecksForUser")
		http.Error(w, "ログインしてから出直してこい！", http.StatusUnauthorized)
		return
	}

	log.Printf("Request from authenticated user: %s", userID)

	// ここで userID を使用して、データベースからそのユーザーのデッキを取得するなどの
	// ビジネスロジックを実行します。
	// 例:
	// decks, err := services.DeckService.GetDecksByUserID(userID)
	// if err != nil {
	//     http.Error(w, "Failed to retrieve decks", http.StatusInternalServerError)
	//     return
	// }
	// json.NewEncoder(w).Encode(decks) // デッキをJSONとして返す

	// 現時点では、簡易的なレスポンスを返します。
	w.Header().Set("Content-Type", "text/plain") // Content-Type を設定
	fmt.Fprintf(w, "やあ。ここはJWTトークン持っている人専用のAPIだよ。貴様、ログインしているな？ユーザーID: %s", userID)
}

