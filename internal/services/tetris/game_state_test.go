package tetris

import (
	"testing"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
	"github.com/stretchr/testify/assert"
)

func TestNewPlayerGameState(t *testing.T) {
	// テスト用のデッキデータを作成
	now := time.Now()
	deck := &models.Deck{
		ID:          "test-deck-1",
		// Name:        "Test Deck",
		// Description: "Test deck for unit testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 新しいゲーム状態を作成
	state := NewPlayerGameState("test-user-1", deck)

	// 基本的なフィールドの検証
	assert.Equal(t, "test-user-1", state.UserID)
	assert.Equal(t, deck, state.Deck)
	assert.Equal(t, 0, state.Score)
	assert.Equal(t, 0, state.LinesCleared)
	assert.Equal(t, 1, state.Level)
	assert.False(t, state.IsGameOver)


	// ボードの初期化を確認
	assert.NotNil(t, state.Board)
	assert.Equal(t, tetris.BoardWidth, len(state.Board[0]))
	assert.Equal(t, tetris.BoardHeight, len(state.Board))

	// ピースの初期化を確認
	assert.NotNil(t, state.CurrentPiece)
	assert.NotNil(t, state.NextPiece)
	assert.Nil(t, state.HeldPiece)

	// 乱数生成器の初期化を確認
	assert.NotNil(t, state.randGenerator)

	// 時間関連フィールドの初期化を確認
	assert.True(t, time.Since(state.lastFallTime) < time.Second)

	// Contributionスコアの初期化を確認
	assert.NotNil(t, state.ContributionScores)
	assert.Equal(t, tetris.BoardHeight*tetris.BoardWidth, len(state.ContributionScores))

	// ピースキューの初期化を確認
	assert.NotNil(t, state.pieceQueue)
	assert.GreaterOrEqual(t, len(state.pieceQueue), 7) // 7-bag systemの確認
}

func TestGeneratePieceQueue(t *testing.T) {
	now := time.Now()
	deck := &models.Deck{
		ID:          "test-deck-2",
		// Name:        "Test Deck 2",
		// Description: "Test deck for piece queue testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	state := NewPlayerGameState("test-user-2", deck)

	// キューをクリアして新しいバッグを生成
	state.pieceQueue = nil
	state.generatePieceQueue()

	// ピースキューの長さを確認
	assert.Equal(t, 7, len(state.pieceQueue))

	// 7-bag systemの確認
	pieceTypes := make(map[tetris.PieceType]int)
	for _, pieceType := range state.pieceQueue {
		pieceTypes[pieceType]++
	}

	// 各ピースタイプが1回ずつ出現することを確認
	for _, pieceType := range []tetris.PieceType{
		tetris.TypeI,
		tetris.TypeO,
		tetris.TypeT,
		tetris.TypeS,
		tetris.TypeZ,
		tetris.TypeJ,
		tetris.TypeL,
	} {
		assert.Equal(t, 1, pieceTypes[pieceType], "Piece type %v should appear exactly once", pieceType)
	}
}

func TestGetNextPieceFromQueue(t *testing.T) {
	now := time.Now()
	deck := &models.Deck{
		ID:          "test-deck-3",
		// Name:        "Test Deck 3",
		// Description: "Test deck for next piece testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	state := NewPlayerGameState("test-user-3", deck)

	// 最初のピースを取得
	firstPiece := state.GetNextPieceFromQueue()
	assert.NotNil(t, firstPiece)

	// キューの長さが減少したことを確認
	originalLength := len(state.pieceQueue)
	state.GetNextPieceFromQueue()
	assert.Equal(t, originalLength-1, len(state.pieceQueue))

	// キューが7個未満になった時に新しいバッグが生成されることを確認
	for i := 0; i < 7; i++ {
		state.GetNextPieceFromQueue()
	}
	assert.GreaterOrEqual(t, len(state.pieceQueue), 7)
}

func TestSpawnNewPiece(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)

	// SpawnNewPieceは NewPlayerGameState 内で一度自動実行される
	// 初期の CurrentPiece を取得
	initialPiece := state.CurrentPiece
	assert.NotNil(t, initialPiece)

	// テトリミノのタイプに応じた期待値を設定
	var expectedX, expectedY int
	switch initialPiece.Type {
	case tetris.TypeI:
		expectedX = tetris.BoardWidth/2 - 2 // 3
		expectedY = 1
	case tetris.TypeO:
		expectedX = tetris.BoardWidth/2 - 1 // 4
		expectedY = 1
	case tetris.TypeL:
		expectedX = tetris.BoardWidth/2 - 1 // 4
		expectedY = 1
	default:
		expectedX = tetris.BoardWidth/2 - 1 // 4
		expectedY = 1
	}

	assert.Equal(t, expectedX, initialPiece.X)
	assert.Equal(t, expectedY, initialPiece.Y)
	assert.Equal(t, 0, initialPiece.Rotation)
}

// TestNewGameSession はNewGameSession関数をテストします
func TestNewGameSession(t *testing.T) {
	// テスト用のデッキデータを作成
	now := time.Now()
	deck := &models.Deck{
		ID:          "test-deck-1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// NewGameSessionを呼び出し (deckRepoをnilで渡してランダムスコア使用)
	session, err := NewGameSession("test-room-1", "test-user-1", deck, nil)

	// エラーがないことを確認
	assert.NoError(t, err)
	assert.NotNil(t, session)

	// セッションの基本フィールドを確認
	assert.Equal(t, "test-room-1", session.ID)
	assert.Equal(t, "waiting", session.Status)
	assert.NotNil(t, session.Player1)
	assert.Nil(t, session.Player2)
	assert.Equal(t, "test-user-1", session.Player1.UserID)
}

// TestSetPlayer2 はSetPlayer2メソッドをテストします
func TestSetPlayer2(t *testing.T) {
	// テスト用のデッキデータを作成
	now := time.Now()
	deck1 := &models.Deck{
		ID:          "test-deck-1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	deck2 := &models.Deck{
		ID:          "test-deck-2",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// ゲームセッションを作成
	session, err := NewGameSession("test-room-1", "test-user-1", deck1, nil)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	// Player2を設定
	session.SetPlayer2("test-user-2", deck2, nil)

	// Player2の設定を確認
	assert.NotNil(t, session.Player2)
	assert.Equal(t, "test-user-2", session.Player2.UserID)
	assert.Equal(t, deck2, session.Player2.Deck)
} 