package tetris

import (
	"strconv"
	"testing"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// TestPieceRotationScoreConsistency はピース回転時のスコア配置の一貫性をテストします
func TestPieceRotationScoreConsistency(t *testing.T) {
	// テスト用のDeckPlacementPieceを作成（T-ピース）
	testDeckPlacement := DeckPlacementPiece{
		Type:     tetris.TypeT,
		Rotation: 0,
		Blocks: []models.Position{
			{X: 1, Y: 0, Score: 150}, // ブロック0: 高スコア（紫）
			{X: 0, Y: 1, Score: 80},  // ブロック1: 中高スコア（青）
			{X: 1, Y: 1, Score: 30},  // ブロック2: 中スコア（緑）
			{X: 2, Y: 1, Score: 10},  // ブロック3: 低スコア（黄）
		},
	}

	// PlayerGameStateを作成
	mockDeck := &models.Deck{ID: "test-deck"}
	state := NewPlayerGameState("test-user", mockDeck)
	state.DeckPlacements = []DeckPlacementPiece{testDeckPlacement}

	// テスト用のT-ピースを作成
	piece := state.GetNextPieceFromQueue()
	if piece.Type != tetris.TypeT {
		// T-ピースが出るまで繰り返し
		for piece.Type != tetris.TypeT {
			piece = state.GetNextPieceFromQueue()
		}
	}

	// 初期位置を設定
	piece.X = 4
	piece.Y = 2
	piece.Rotation = 0

	t.Logf("=== 回転テスト開始 ===")
	t.Logf("初期ピース: Type=%d, X=%d, Y=%d, Rotation=%d", piece.Type, piece.X, piece.Y, piece.Rotation)
	t.Logf("スコアデータ: %v", piece.ScoreData)

	// 各回転状態でスコアの一貫性をテスト
	rotations := []int{0, 90, 180, 270}
	expectedScores := []int{150, 80, 30, 10} // 各ブロックの期待スコア

	for _, rotation := range rotations {
		piece.Rotation = rotation
		blocks := piece.Blocks()

		t.Logf("\n--- 回転状態: %d度 ---", rotation)
		t.Logf("ブロック座標: %v", blocks)

		// 各ブロックのスコアをチェック
		for blockIndex, block := range blocks {
			boardX := piece.X + block[0]
			boardY := piece.Y + block[1]

			// ブロックインデックスベースでスコアを取得
			blockIndexKey := string(rune(blockIndex + '0'))
			score, exists := piece.ScoreData[blockIndexKey]

			if !exists {
				t.Errorf("回転%d度でブロック%dのスコアが見つからない（キー: %s）", rotation, blockIndex, blockIndexKey)
				continue
			}

			expectedScore := expectedScores[blockIndex]
			if score != expectedScore {
				t.Errorf("回転%d度でブロック%d（位置[%d,%d]）のスコアが不正: 期待値=%d, 実際=%d", 
					rotation, blockIndex, boardX, boardY, expectedScore, score)
			} else {
				t.Logf("✓ ブロック%d（位置[%d,%d]）: スコア=%d", blockIndex, boardX, boardY, score)
			}
		}
	}
}

// TestRotationScoreMapping は回転時のスコアマッピングロジックをテストします
func TestRotationScoreMapping(t *testing.T) {
	// T-ピースの各回転状態での形状
	tShapes := map[int][][2]int{
		0:   {{1, 0}, {0, 1}, {1, 1}, {2, 1}}, // 0度
		90:  {{1, 0}, {1, 1}, {2, 1}, {1, 2}}, // 90度
		180: {{0, 1}, {1, 1}, {2, 1}, {1, 2}}, // 180度
		270: {{0, 1}, {1, 0}, {1, 1}, {1, 2}}, // 270度
	}

	piece := &tetris.Piece{
		Type:     tetris.TypeT,
		X:        4,
		Y:        2,
		Rotation: 0,
		ScoreData: map[string]int{
			"0": 150, // ブロック0のスコア
			"1": 80,  // ブロック1のスコア
			"2": 30,  // ブロック2のスコア
			"3": 10,  // ブロック3のスコア
		},
	}

	t.Logf("=== スコアマッピングテスト ===")

	for rotation, expectedShape := range tShapes {
		piece.Rotation = rotation
		actualShape := piece.Blocks()

		t.Logf("\n--- 回転%d度 ---", rotation)
		t.Logf("期待形状: %v", expectedShape)
		t.Logf("実際形状: %v", actualShape)

		// 形状が一致するかチェック
		if len(actualShape) != len(expectedShape) {
			t.Errorf("回転%d度でブロック数が不正: 期待値=%d, 実際=%d", rotation, len(expectedShape), len(actualShape))
			continue
		}

		for i, expectedBlock := range expectedShape {
			if len(actualShape) <= i {
				t.Errorf("回転%d度でブロック%dが存在しない", rotation, i)
				continue
			}

			actualBlock := actualShape[i]
			if actualBlock[0] != expectedBlock[0] || actualBlock[1] != expectedBlock[1] {
				t.Errorf("回転%d度でブロック%dの座標が不正: 期待値=%v, 実際=%v", 
					rotation, i, expectedBlock, actualBlock)
			}
		}

		// スコアが正しく取得できるかチェック
		for blockIndex := range actualShape {
			blockIndexKey := string(rune(blockIndex + '0'))
			score, exists := piece.ScoreData[blockIndexKey]
			if !exists {
				t.Errorf("回転%d度でブロック%dのスコアキー'%s'が見つからない", rotation, blockIndex, blockIndexKey)
			} else {
				t.Logf("✓ ブロック%d: スコア=%d", blockIndex, score)
			}
		}
	}
}

// TestGameLogicRotation は実際のゲームロジックでの回転をテストします
func TestGameLogicRotation(t *testing.T) {
	// テスト用のゲーム状態を作成
	mockDeck := &models.Deck{ID: "test-deck"}
	state := NewPlayerGameState("test-user", mockDeck)

	// テスト用のDeckPlacementを設定
	state.DeckPlacements = []DeckPlacementPiece{
		{
			Type:     tetris.TypeT,
			Rotation: 0,
			Blocks: []models.Position{
				{X: 1, Y: 0, Score: 150},
				{X: 0, Y: 1, Score: 80},
				{X: 1, Y: 1, Score: 30},
				{X: 2, Y: 1, Score: 10},
			},
		},
	}

	// T-ピースを取得
	for state.CurrentPiece == nil || state.CurrentPiece.Type != tetris.TypeT {
		state.SpawnNewPiece()
	}

	initialRotation := state.CurrentPiece.Rotation
	t.Logf("初期回転: %d", initialRotation)
	t.Logf("初期スコアデータ: %v", state.CurrentPiece.ScoreData)

	// 回転アクションを実行
	moved := ApplyPlayerInput(state, "rotate")
	if !moved {
		t.Error("回転アクションが失敗しました")
		return
	}

	t.Logf("回転後: %d", state.CurrentPiece.Rotation)
	t.Logf("回転後スコアデータ: %v", state.CurrentPiece.ScoreData)

	// CurrentPieceScoresが正しく更新されているかチェック
	t.Logf("CurrentPieceScores: %v", state.CurrentPieceScores)

	// 各ブロックのスコアが正しく設定されているかチェック
	blocks := state.CurrentPiece.Blocks()
	for blockIndex, block := range blocks {
		boardX := state.CurrentPiece.X + block[0]
		boardY := state.CurrentPiece.Y + block[1]

		// ボード範囲内のチェック
		if boardX >= 0 && boardX < tetris.BoardWidth && boardY >= 0 && boardY < tetris.BoardHeight {
			// ブロックインデックスキーでスコアを確認
			blockIndexKey := string(rune(blockIndex + '0'))
			if score, exists := state.CurrentPiece.ScoreData[blockIndexKey]; exists {
				t.Logf("✓ 回転後ブロック%d（位置[%d,%d]）: スコア=%d", blockIndex, boardX, boardY, score)
			} else {
				t.Errorf("回転後ブロック%dのスコアが見つからない（キー: %s）", blockIndex, blockIndexKey)
			}
		}
	}
} 

// TestRotationScoreConsistencyInGame は実際のゲーム内で回転時のスコア一貫性をテストします
func TestRotationScoreConsistencyInGame(t *testing.T) {
	// テスト用のゲーム状態を作成
	mockDeck := &models.Deck{ID: "test-deck"}
	state := NewPlayerGameState("test-user", mockDeck)

	// テスト用のDeckPlacementを設定（T-ピース）
	state.DeckPlacements = []DeckPlacementPiece{
		{
			Type:     tetris.TypeT,
			Rotation: 0,
			Blocks: []models.Position{
				{X: 1, Y: 0, Score: 150}, // ブロック0: 高スコア（紫）
				{X: 0, Y: 1, Score: 80},  // ブロック1: 中高スコア（青）
				{X: 1, Y: 1, Score: 30},  // ブロック2: 中スコア（緑）
				{X: 2, Y: 1, Score: 10},  // ブロック3: 低スコア（黄）
			},
		},
	}

	// 手動でT-ピースを作成（正しいスコアデータ付き）
	state.CurrentPiece = &tetris.Piece{
		Type:     tetris.TypeT,
		X:        4,
		Y:        2,
		Rotation: 0,
		ScoreData: map[string]int{
			"0": 150, // ブロック0のスコア
			"1": 80,  // ブロック1のスコア
			"2": 30,  // ブロック2のスコア
			"3": 10,  // ブロック3のスコア
			// 相対座標キーも設定（フロントエンドとの互換性）
			"1_0": 150, // ブロック0の相対座標
			"0_1": 80,  // ブロック1の相対座標
			"1_1": 30,  // ブロック2の相対座標
			"2_1": 10,  // ブロック3の相対座標
		},
	}

	// 初期のCurrentPieceScoresを更新
	state.updateCurrentPieceScores()

	t.Logf("=== 回転時のスコア一貫性テスト ===")
	t.Logf("初期ピース: Type=%d, X=%d, Y=%d, Rotation=%d", 
		state.CurrentPiece.Type, state.CurrentPiece.X, state.CurrentPiece.Y, state.CurrentPiece.Rotation)
	t.Logf("初期スコアデータ: %v", state.CurrentPiece.ScoreData)
	t.Logf("初期CurrentPieceScores: %v", state.CurrentPieceScores)

	// 初期状態の各ブロックスコアを記録
	expectedScores := []int{150, 80, 30, 10}
	
	// 初期状態のスコアをチェック
	for blockIndex := 0; blockIndex < 4; blockIndex++ {
		blockIndexKey := strconv.Itoa(blockIndex)
		if score, exists := state.CurrentPieceScores[blockIndexKey]; exists {
			expectedScore := expectedScores[blockIndex]
			if score != expectedScore {
				t.Errorf("初期状態でブロック%dのスコアが不正: 期待値=%d, 実際=%d", 
					blockIndex, expectedScore, score)
			} else {
				t.Logf("✓ 初期ブロック%d: スコア=%d", blockIndex, score)
			}
		} else {
			t.Errorf("初期状態でブロック%dのスコア（キー: %s）が見つからない", blockIndex, blockIndexKey)
		}
	}
	
	// 4回回転して一周させながらスコアの一貫性をチェック
	for rotation := 0; rotation < 4; rotation++ {
		t.Logf("\n--- 回転 %d回目 (回転後: %d度) ---", rotation+1, (rotation+1)*90)
		
		// 回転実行
		moved := ApplyPlayerInput(state, "rotate")
		if !moved {
			t.Errorf("回転 %d回目が失敗しました", rotation+1)
			return
		}
		
		t.Logf("回転後の状態: Rotation=%d", state.CurrentPiece.Rotation)
		t.Logf("CurrentPieceScores: %v", state.CurrentPieceScores)
		
		// 各ブロックのスコア一貫性をチェック
		blocks := state.CurrentPiece.Blocks()
		for blockIndex, block := range blocks {
			boardX := state.CurrentPiece.X + block[0]
			boardY := state.CurrentPiece.Y + block[1]
			
			// ボード範囲内のチェック
			if boardX >= 0 && boardX < tetris.BoardWidth && boardY >= 0 && boardY < tetris.BoardHeight {
				// CurrentPieceScoresからブロックインデックスベースでスコア取得
				blockIndexKey := strconv.Itoa(blockIndex)
				if score, exists := state.CurrentPieceScores[blockIndexKey]; exists {
					expectedScore := expectedScores[blockIndex]
					if score != expectedScore {
						t.Errorf("回転%d回目でブロック%d（位置[%d,%d]）のスコアが不正: 期待値=%d, 実際=%d", 
							rotation+1, blockIndex, boardX, boardY, expectedScore, score)
					} else {
						t.Logf("✓ ブロック%d（位置[%d,%d]）: スコア=%d (一貫性OK)", 
							blockIndex, boardX, boardY, score)
					}
				} else {
					t.Errorf("回転%d回目でブロック%dのスコア（キー: %s）が見つからない", 
						rotation+1, blockIndex, blockIndexKey)
				}
			}
		}
	}
	
	// 4回転後、元の回転状態に戻っているかチェック
	if state.CurrentPiece.Rotation != 0 {
		t.Errorf("4回転後の回転状態が不正: 期待値=0, 実際=%d", state.CurrentPiece.Rotation)
	}
} 