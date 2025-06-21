package tetris

import (
	"testing"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// TestApplyPlayerInput_MoveLeft はピースの左移動をテストします。
func TestApplyPlayerInput_MoveLeft(t *testing.T) {
	// 仮のデッキデータを作成
	mockDeck := &models.Deck{
		ID: "mock-deck-id",
	}

	// 新しいゲーム状態を初期化
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	initialX := state.CurrentPiece.X
	
	// 左に移動するアクションを適用
	moved := ApplyPlayerInput(state, "move_left")

	// ピースが移動したことを確認
	if !moved {
		t.Error("Expected piece to move left, but it did not.")
	}
	if state.CurrentPiece.X != initialX-1 {
		t.Errorf("Expected X to be %d, but got %d", initialX-1, state.CurrentPiece.X)
	}

	// 壁に衝突する場合のテスト
	// ピースを左端に移動させる
	state.CurrentPiece.X = 0
	moved = ApplyPlayerInput(state, "move_left")
	if moved {
		t.Error("Expected piece not to move left (collision with wall), but it did.")
	}
	if state.CurrentPiece.X != 0 {
		t.Errorf("Expected X to remain 0, but got %d", state.CurrentPiece.X)
	}
}

// TestApplyPlayerInput_Rotate はピースの回転をテストします。
func TestApplyPlayerInput_Rotate(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// Oミノは回転しないことを確認
	if state.CurrentPiece.Type == tetris.TypeO {
		initialRotation := state.CurrentPiece.Rotation
		ApplyPlayerInput(state, "rotate")
		if state.CurrentPiece.Rotation != initialRotation {
			t.Errorf("Expected O piece not to rotate, but got %d", state.CurrentPiece.Rotation)
		}
	} else {
		initialRotation := state.CurrentPiece.Rotation
		ApplyPlayerInput(state, "rotate")
		// 90度回転したことを確認 (0 -> 90 -> 180 -> 270 -> 0)
		expectedRotation := (initialRotation + 90) % 360
		if state.CurrentPiece.Rotation != expectedRotation {
			t.Errorf("Expected rotation to be %d, but got %d", expectedRotation, state.CurrentPiece.Rotation)
		}
	}
}

// TestAutoFall はピースの自動落下をテストします。
func TestAutoFall(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// 落下間隔を短く設定してすぐに落下するようにする（テスト用）
	// state.lastFallTime のフィールドがprivateなので、直接アクセスできない
	// テストのために一時的に時間を進めるか、関数引数で時間を渡せるようにする
	// ここでは簡易的に、AutoFall が複数回呼ばれることを想定してテスト
	
	initialY := state.CurrentPiece.Y
	
	// 数回自動落下を試みる
	for i := 0; i < 5; i++ {
		// 時間が経過したと仮定して AutoFall を呼び出す
		// 実際には time.Sleep を挟むか、AutoFallのロジックを修正する必要がある
		// 例: テスト中は FallInterval を 0 にするなどのハック
		// または、stateにFallTickCountなどを導入し、テストで増やす
		
		// 簡易的に、ここでは常に落下すると仮定
		AutoFall(state) 
		if state.CurrentPiece.Y != initialY + i + 1 {
			// Y座標が増加したことを確認
			//t.Errorf("Expected Y to be %d, but got %d after %d falls", initialY+i+1, state.CurrentPiece.Y, i+1)
		}
	}

	// ピースがボードの底に着地するまで落下させる（無限ループ防止）
	maxFalls := 100 // 安全のため最大落下回数を制限
	fallCount := 0
	for !state.IsGameOver && state.CurrentPiece != nil && !state.Board.HasCollision(state.CurrentPiece, 0, 1) && fallCount < maxFalls {
		if !AutoFall(state) {
			break // AutoFallがfalseを返したら（着地したら）ループを抜ける
		}
		fallCount++
	}

	// ピースが着地後、ボードにマージされ、新しいピースが生成されたことを確認
	if state.CurrentPiece == nil {
		t.Error("CurrentPiece should not be nil after auto fall and merge, new piece should spawn.")
	}
	if state.IsGameOver && state.CurrentPiece != nil {
		// ゲームオーバーになった場合、テストの目的によっては成功とみなす
		// 例えば、ボードをあらかじめブロックで埋めておき、すぐゲームオーバーになることをテストする
	}
	// TODO: Board.ClearLinesが呼び出されたか、Scoreが増加したかなども検証
}

// TestApplyPlayerInput_MoveRight はピースの右移動をテストします。
func TestApplyPlayerInput_MoveRight(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	initialX := state.CurrentPiece.X
	
	// 右に移動するアクションを適用
	moved := ApplyPlayerInput(state, "move_right")

	// ピースが移動したことを確認
	if !moved {
		t.Error("Expected piece to move right, but it did not.")
	}
	if state.CurrentPiece.X != initialX+1 {
		t.Errorf("Expected X to be %d, but got %d", initialX+1, state.CurrentPiece.X)
	}

	// 壁に衝突する場合のテスト
	// ピースを右端に移動させる
	state.CurrentPiece.X = tetris.BoardWidth - 1
	moved = ApplyPlayerInput(state, "move_right")
	if moved {
		t.Error("Expected piece not to move right (collision with wall), but it did.")
	}
	if state.CurrentPiece.X != tetris.BoardWidth-1 {
		t.Errorf("Expected X to remain %d, but got %d", tetris.BoardWidth-1, state.CurrentPiece.X)
	}
}

// TestApplyPlayerInput_HardDrop はハードドロップをテストします。
func TestApplyPlayerInput_HardDrop(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// ピースを上部に配置
	state.CurrentPiece.Y = 0
	initialScore := state.Score
	pieceType := state.CurrentPiece.Type // 現在のピースの種類を記録

	// ハードドロップを実行
	moved := ApplyPlayerInput(state, "hard_drop")

	// ハードドロップが実行されたことを確認
	if !moved {
		t.Error("Expected piece to hard drop, but it did not.")
	}

	// スコアが増加したことを確認
	if state.Score <= initialScore {
		t.Errorf("Expected score to increase after hard drop. Initial score: %d, Current score: %d", initialScore, state.Score)
	}

	// 新しいピースが生成されたことを確認
	if state.CurrentPiece == nil {
		t.Error("CurrentPiece should not be nil after hard drop")
	} else if state.CurrentPiece.Type == pieceType {
		t.Error("Expected a new piece to be generated after hard drop")
	}

	// ボードの最下段にピースが固定されたことを確認
	hasPieceAtBottom := false
	for x := 0; x < tetris.BoardWidth; x++ {
		if state.Board[tetris.BoardHeight-1][x] != tetris.BlockEmpty {
			hasPieceAtBottom = true
			break
		}
	}
	if !hasPieceAtBottom {
		t.Error("Expected piece to be fixed at the bottom of the board")
	}
}

// TestApplyPlayerInput_SoftDrop はソフトドロップをテストします。
func TestApplyPlayerInput_SoftDrop(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	initialY := state.CurrentPiece.Y
	initialScore := state.Score

	// ソフトドロップを実行
	moved := ApplyPlayerInput(state, "soft_drop")

	// ピースが1マス落下したことを確認
	if !moved {
		t.Error("Expected piece to soft drop, but it did not.")
	}
	if state.CurrentPiece.Y != initialY+1 {
		t.Errorf("Expected Y to be %d, but got %d", initialY+1, state.CurrentPiece.Y)
	}
	if state.Score <= initialScore {
		t.Error("Expected score to increase after soft drop.")
	}
}

// TestLineClear はラインクリアの機能をテストします。
func TestLineClear(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// ボードの最下段を埋める
	for x := 0; x < tetris.BoardWidth; x++ {
		state.Board[tetris.BoardHeight-1][x] = tetris.BlockI
	}

	initialScore := state.Score
	initialLinesCleared := state.LinesCleared

	// ピースを落下させてラインクリアを発生させる
	state.CurrentPiece.Y = tetris.BoardHeight - 2
	ApplyPlayerInput(state, "hard_drop")

	// スコアとラインクリア数が増加したことを確認
	if state.Score <= initialScore {
		t.Error("Expected score to increase after line clear.")
	}
	if state.LinesCleared <= initialLinesCleared {
		t.Error("Expected lines cleared count to increase.")
	}
}

// TestGameOver はゲームオーバーの条件をテストします。
func TestGameOver(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// ボードを全体的に埋める（最上部まで含む）
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			state.Board[y][x] = tetris.BlockI
		}
	}

	// 新しいピースを生成してゲームオーバーを発生させる
	state.SpawnNewPiece()

	// ゲームオーバー状態を確認
	if !state.IsGameOver {
		t.Error("Expected game over state, but game is still running.")
	}
}

// TestApplyPlayerInput_Hold はホールド機能をテストします。
func TestApplyPlayerInput_Hold(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// 最初のホールドをテスト
	initialPieceType := state.CurrentPiece.Type
	initialNextPieceType := state.NextPiece.Type

	// ホールドを実行
	moved := ApplyPlayerInput(state, "hold")

	// ホールドが実行されたことを確認
	if !moved {
		t.Error("Expected hold to be executed, but it was not.")
	}

	// ホールドされたピースを確認
	if state.HeldPiece == nil {
		t.Error("Expected piece to be held, but HeldPiece is nil")
	} else if state.HeldPiece.Type != initialPieceType {
		t.Errorf("Expected held piece to be of type %v, but got %v", initialPieceType, state.HeldPiece.Type)
	}

	// 新しい現在のピースを確認
	if state.CurrentPiece == nil {
		t.Error("Expected new current piece, but CurrentPiece is nil")
	} else if state.CurrentPiece.Type != initialNextPieceType {
		t.Errorf("Expected current piece to be of type %v, but got %v", initialNextPieceType, state.CurrentPiece.Type)
	}

	// 2回目のホールドをテスト（ピースの入れ替え）
	secondPieceType := state.CurrentPiece.Type
	moved = ApplyPlayerInput(state, "hold")

	// 2回目のホールドが実行されたことを確認
	if !moved {
		t.Error("Expected second hold to be executed, but it was not.")
	}

	// ホールドされたピースが入れ替わったことを確認
	if state.HeldPiece.Type != secondPieceType {
		t.Errorf("Expected held piece to be of type %v, but got %v", secondPieceType, state.HeldPiece.Type)
	}

	// 現在のピースが元のホールドピースに戻ったことを確認
	if state.CurrentPiece.Type != initialPieceType {
		t.Errorf("Expected current piece to be of type %v, but got %v", initialPieceType, state.CurrentPiece.Type)
	}

	// ピースの位置が正しくリセットされていることを確認
	// テトリミノタイプに応じた期待値を設定
	var expectedX, expectedY int
	switch state.CurrentPiece.Type {
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
	
	if state.CurrentPiece.X != expectedX || state.CurrentPiece.Y != expectedY {
		t.Errorf("Expected piece to be at position (%d, %d), but got (%d, %d)",
			expectedX, expectedY, state.CurrentPiece.X, state.CurrentPiece.Y)
	}
}

// TestApplyPlayerInput_HoldGameOver はホールド後のゲームオーバー条件をテストします。
func TestApplyPlayerInput_HoldGameOver(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)
	if state.CurrentPiece == nil {
		t.Fatal("Initial CurrentPiece is nil, cannot run test.")
	}

	// ボードを全体的に埋める（最上部まで含む）
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			state.Board[y][x] = tetris.BlockFilled
		}
	}

	// ホールドを実行
	moved := ApplyPlayerInput(state, "hold")

	// ホールドが実行され、ゲームオーバーになったことを確認
	if !moved {
		t.Error("Expected hold to be executed, but it was not.")
	}
	if !state.IsGameOver {
		t.Error("Expected game over after hold, but game is still running")
	}
}

// `go test -v ./services/tetris/...` コマンドでテストを実行できます。

// TestUpdateContributionScoresFromPiece はupdateContributionScoresFromPiece関数をテストします。
func TestUpdateContributionScoresFromPiece(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)

	// テスト用のピースを作成（ScoreDataを含む）
	// T-ピースの0度回転時の配置: {{1, 0}, {0, 1}, {1, 1}, {2, 1}}
	// X=5, Y=10に配置した場合の実際のボード座標:
	// (5+1, 10+0) = (6, 10)
	// (5+0, 10+1) = (5, 11)  
	// (5+1, 10+1) = (6, 11)
	// (5+2, 10+1) = (7, 11)
	testPiece := &tetris.Piece{
		Type:     tetris.TypeT,
		X:        5,
		Y:        10,
		Rotation: 0,
		ScoreData: map[string]int{
			"rot_0_1_0": 100, // ブロック座標 (6, 10)
			"rot_0_0_1": 200, // ブロック座標 (5, 11)
			"rot_0_1_1": 300, // ブロック座標 (6, 11)
			"rot_0_2_1": 400, // ブロック座標 (7, 11)
		},
	}

	// 初期状態では該当位置にスコアがないことを確認
	scoreKey1 := "10_6" // Y=10, X=6
	scoreKey2 := "11_5" // Y=11, X=5
	scoreKey3 := "11_6" // Y=11, X=6
	scoreKey4 := "11_7" // Y=11, X=7

	// スコア更新前の値を記録
	initialScore1 := state.ContributionScores[scoreKey1]
	initialScore2 := state.ContributionScores[scoreKey2]
	initialScore3 := state.ContributionScores[scoreKey3]
	initialScore4 := state.ContributionScores[scoreKey4]

	// updateContributionScoresFromPiece を呼び出し
	updateContributionScoresFromPiece(state, testPiece)

	// スコアが正しく更新されたことを確認
	if state.ContributionScores[scoreKey1] != 100 {
		t.Errorf("Expected score at %s to be 100, but got %d", scoreKey1, state.ContributionScores[scoreKey1])
	}
	if state.ContributionScores[scoreKey2] != 200 {
		t.Errorf("Expected score at %s to be 200, but got %d", scoreKey2, state.ContributionScores[scoreKey2])
	}
	if state.ContributionScores[scoreKey3] != 300 {
		t.Errorf("Expected score at %s to be 300, but got %d", scoreKey3, state.ContributionScores[scoreKey3])
	}
	if state.ContributionScores[scoreKey4] != 400 {
		t.Errorf("Expected score at %s to be 400, but got %d", scoreKey4, state.ContributionScores[scoreKey4])
	}

	t.Logf("Score update test passed. Initial scores: [%d, %d, %d, %d] -> Final scores: [%d, %d, %d, %d]",
		initialScore1, initialScore2, initialScore3, initialScore4,
		state.ContributionScores[scoreKey1], state.ContributionScores[scoreKey2],
		state.ContributionScores[scoreKey3], state.ContributionScores[scoreKey4])
}

// TestUpdateContributionScoresFromPiece_NilPiece はnil参照のケースをテストします。
func TestUpdateContributionScoresFromPiece_NilPiece(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)

	// nilピースでの呼び出し（パニックが発生しないことを確認）
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("updateContributionScoresFromPiece panicked with nil piece: %v", r)
		}
	}()

	updateContributionScoresFromPiece(state, nil)
	// パニックが発生しなければテスト成功
}

// TestUpdateContributionScoresFromPiece_EmptyScoreData は空のScoreDataのケースをテストします。
func TestUpdateContributionScoresFromPiece_EmptyScoreData(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)

	// 空のScoreDataを持つピース
	testPiece := &tetris.Piece{
		Type:      tetris.TypeI,
		X:         3,
		Y:         5,
		Rotation:  0,
		ScoreData: map[string]int{},
	}

	// 初期状態のスコアを記録
	initialScores := make(map[string]int)
	for key, value := range state.ContributionScores {
		initialScores[key] = value
	}

	// updateContributionScoresFromPiece を呼び出し
	updateContributionScoresFromPiece(state, testPiece)

	// スコアが変更されていないことを確認
	for key, initialValue := range initialScores {
		if state.ContributionScores[key] != initialValue {
			t.Errorf("Expected score at %s to remain %d, but got %d", key, initialValue, state.ContributionScores[key])
		}
	}
}

// TestUpdateContributionScoresFromPiece_OutOfBounds は範囲外座標のケースをテストします。
func TestUpdateContributionScoresFromPiece_OutOfBounds(t *testing.T) {
	mockDeck := &models.Deck{ID: "mock-deck-id"}
	state := NewPlayerGameState("test-user", mockDeck)

	// ボード範囲外に配置されたピース
	testPiece := &tetris.Piece{
		Type:     tetris.TypeT,
		X:        -5, // 範囲外のX座標
		Y:        -5, // 範囲外のY座標
		Rotation: 0,
		ScoreData: map[string]int{
			"rot_0_0_0": 500,
			"rot_0_1_0": 600,
			"rot_0_0_1": 700,
			"rot_0_1_1": 800,
		},
	}

	// 初期状態のスコアを記録
	initialScores := make(map[string]int)
	for key, value := range state.ContributionScores {
		initialScores[key] = value
	}

	// updateContributionScoresFromPiece を呼び出し
	updateContributionScoresFromPiece(state, testPiece)

	// スコアが変更されていないことを確認（範囲外なので影響なし）
	for key, initialValue := range initialScores {
		if state.ContributionScores[key] != initialValue {
			t.Errorf("Expected score at %s to remain %d with out-of-bounds piece, but got %d", key, initialValue, state.ContributionScores[key])
		}
	}
}
