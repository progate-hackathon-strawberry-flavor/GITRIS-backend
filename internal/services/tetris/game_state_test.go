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
	now := time.Now().Format(time.RFC3339)
	deck := &models.Deck{
		ID:          "test-deck-1",
		Name:        "Test Deck",
		Description: "Test deck for unit testing",
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
	assert.NotNil(t, state.contributionScores)
	assert.Equal(t, tetris.BoardHeight*tetris.BoardWidth, len(state.contributionScores))

	// ピースキューの初期化を確認
	assert.NotNil(t, state.pieceQueue)
	assert.GreaterOrEqual(t, len(state.pieceQueue), 7) // 7-bag systemの確認
}

func TestGeneratePieceQueue(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	deck := &models.Deck{
		ID:          "test-deck-2",
		Name:        "Test Deck 2",
		Description: "Test deck for piece queue testing",
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
	now := time.Now().Format(time.RFC3339)
	deck := &models.Deck{
		ID:          "test-deck-3",
		Name:        "Test Deck 3",
		Description: "Test deck for next piece testing",
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
	now := time.Now().Format(time.RFC3339)
	deck := &models.Deck{
		ID:          "test-deck-4",
		Name:        "Test Deck 4",
		Description: "Test deck for piece spawning testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	state := NewPlayerGameState("test-user-4", deck)

	// 最初のピースのスポーンを確認
	assert.NotNil(t, state.CurrentPiece)
	assert.NotNil(t, state.NextPiece)
	assert.Equal(t, tetris.BoardWidth/2-2, state.CurrentPiece.X)
	assert.Equal(t, 0, state.CurrentPiece.Y)

	// 次のピースのスポーンを確認
	originalCurrentPiece := state.CurrentPiece
	originalNextPiece := state.NextPiece
	state.SpawnNewPiece()
	assert.Equal(t, originalNextPiece, state.CurrentPiece)
	assert.NotEqual(t, originalCurrentPiece, state.CurrentPiece)
	assert.NotEqual(t, originalNextPiece, state.NextPiece)
}

func TestNewGameSession(t *testing.T) {
	roomID := "test-room"
	player1ID := "player1"
	deck := &models.Deck{ID: "test-deck"}

	session := NewGameSession(roomID, player1ID, deck)

	assert.NotNil(t, session)
	assert.Equal(t, roomID, session.ID)
	assert.Equal(t, "waiting", session.Status)
	assert.NotNil(t, session.Player1)
	assert.Nil(t, session.Player2)
	assert.NotNil(t, session.InputCh)
	assert.NotNil(t, session.OutputCh)
	assert.NotNil(t, session.GameLoopDone)
}

func TestSetPlayer2(t *testing.T) {
	roomID := "test-room"
	player1ID := "player1"
	player2ID := "player2"
	deck := &models.Deck{ID: "test-deck"}

	session := NewGameSession(roomID, player1ID, deck)
	session.SetPlayer2(player2ID, deck)

	assert.NotNil(t, session.Player2)
	assert.Equal(t, player2ID, session.Player2.UserID)

	// チャネルをクローズ
	close(session.InputCh)
	close(session.OutputCh)
	close(session.GameLoopDone)
} 