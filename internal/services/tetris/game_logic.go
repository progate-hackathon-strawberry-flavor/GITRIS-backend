package tetris

import (
	"fmt"
	"log"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// GameLoopSettings はゲームループの速度設定など、ゲーム全体に影響する定数を定義します。
const (
	// FallInterval はピースが自動落下する間隔です。レベルが上がると短縮されます。
	InitialFallInterval = 600 * time.Millisecond // 最初の自動落下間隔を0.6秒に短縮
	SoftDropMultiplier  = 5                       // ソフトドロップ時の落下速度倍率
	GameTimeLimit      = 100 * time.Second       // ゲームの制限時間（100秒）
	LevelUpLines       = 5                       // レベルアップに必要なライン数（5ラインごとにレベルアップ）
	// LockDelay           = 500 * time.Millisecond // ピースが着地してから固定されるまでの猶予時間 (オプション)
)

// GetFallInterval は現在のレベルに基づいた自動落下間隔を計算して返します。
func GetFallInterval(level int) time.Duration {
	// レベルが上がるごとに落下間隔が短くなるロジック
	interval := InitialFallInterval - time.Duration(level-1)*40*time.Millisecond
	if interval < 100*time.Millisecond { // 最小値を設定
		interval = 100 * time.Millisecond
	}
	return interval
}

// ApplyPlayerInput はプレイヤーの入力をゲーム状態に適用します。
//
// Parameters:
//   state : 更新するプレイヤーのゲーム状態のポインタ
//   action : プレイヤーが実行したアクション（"left", "right", "rotate_left", "rotate_right", "soft_drop", "hard_drop", "hold"）
// Returns:
//   bool: ピースが移動・回転・固定されたかどうか（描画更新の判定に使用）
func ApplyPlayerInput(state *PlayerGameState, action string) bool {
	if state.CurrentPiece == nil || state.IsGameOver {
		return false // ゲームオーバーまたは現在のピースがない場合は何もしない
	}

	moved := false

	switch action {
	case "left", "move_left":
		if !state.Board.HasCollision(state.CurrentPiece, -1, 0) {
			state.CurrentPiece.X--
			moved = true
		}
	case "right", "move_right":
		if !state.Board.HasCollision(state.CurrentPiece, 1, 0) {
			state.CurrentPiece.X++
			moved = true
		}
	case "rotate", "rotate_right":
		// 時計回り（右回転）
		originalRotation := state.CurrentPiece.Rotation
		state.CurrentPiece.Rotate() // Pieceのメソッドを使用してO-ミノの回転制御を適用
		if state.Board.HasCollision(state.CurrentPiece, 0, 0) {
			// 回転に失敗した場合は元に戻す
			state.CurrentPiece.Rotation = originalRotation
		} else {
			moved = true
		}
	case "rotate_left":
		// 反時計回り（左回転）
		originalRotation := state.CurrentPiece.Rotation
		state.CurrentPiece.RotateCounterClockwise() // Pieceのメソッドを使用してO-ミノの回転制御を適用
		if state.Board.HasCollision(state.CurrentPiece, 0, 0) {
			// 回転に失敗した場合は元に戻す
			state.CurrentPiece.Rotation = originalRotation
		} else {
			moved = true
		}
	case "soft_drop": // 通常落下を加速
		if !state.Board.HasCollision(state.CurrentPiece, 0, 1) {
			state.CurrentPiece.Y++
			state.Score += 1 // ソフトドロップのボーナス (仮)
			moved = true
		} else {
			// 着地した場合はピースを固定
			state.Board.MergePiece(state.CurrentPiece) // ← この行が欠落していた！
			handlePieceLock(state)
		}
		state.lastFallTime = time.Now() // ソフトドロップしたら落下タイマーをリセット
	case "hard_drop":
		if state.CurrentPiece == nil { return false } // 念のため

		// ハードドロップ距離を最適化された方法で計算
		dropDistance := 0
		maxDropDistance := tetris.BoardHeight - state.CurrentPiece.Y // 最大落下可能距離
		for dropDistance < maxDropDistance && !state.Board.HasCollision(state.CurrentPiece, 0, dropDistance+1) {
			dropDistance++
		}
		
		// 一度にY座標を更新（ループなし）
		state.CurrentPiece.Y += dropDistance
		state.Score += dropDistance * 2 // ハードドロップのボーナス（一度に計算）
		
		// ピースをボードに固定
		state.Board.MergePiece(state.CurrentPiece)
		handlePieceLock(state) // 固定後の処理
		state.lastFallTime = time.Now() // ハードドロップしたら落下タイマーをリセット
		
		// ハードドロップ後はスコア更新を強制スキップ（高速化）
		// 新しいピースが生成されてからクライアント側で更新される
		moved = true
	case "hold":
		// ホールド機能の安全性チェック
		if state.CurrentPiece == nil {
			log.Printf("[WARN] Hold action attempted with nil CurrentPiece for user %s", state.UserID)
			break // 現在のピースがない場合は何もしない
		}
		
		if state.HeldPiece == nil {
			// 初回ホールド: 現在のピースをホールドし、次のピースを現在のピースにする
			state.HeldPiece = &tetris.Piece{
				Type:     state.CurrentPiece.Type,
				X:        0,
				Y:        0,
				Rotation: 0,
				ScoreData: make(map[string]int), // ScoreDataマップを初期化
			}
			
			// ScoreDataを深いコピー
			if state.CurrentPiece.ScoreData != nil {
				for key, value := range state.CurrentPiece.ScoreData {
					state.HeldPiece.ScoreData[key] = value
				}
			}
			
			state.CurrentPiece = state.NextPiece
			if state.CurrentPiece == nil {
				log.Printf("[ERROR] NextPiece is nil during hold for user %s", state.UserID)
				state.CurrentPiece = state.GetNextPieceFromQueue()
			}
			
			state.NextPiece = state.GetNextPieceFromQueue()
			
			// テトリミノの種類に応じた適切な初期位置を設定
			if state.CurrentPiece != nil {
				switch state.CurrentPiece.Type {
				case tetris.TypeI:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 2
					state.CurrentPiece.Y = 1
				case tetris.TypeO:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				case tetris.TypeL:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				default:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				}
				state.CurrentPiece.Rotation = 0
			}
			moved = true
		} else {
			// 2回目以降のホールド: 現在のピースとホールドピースを交換
			currentPieceCopy := &tetris.Piece{
				Type:     state.CurrentPiece.Type,
				X:        state.CurrentPiece.X,
				Y:        state.CurrentPiece.Y,
				Rotation: state.CurrentPiece.Rotation,
				ScoreData: make(map[string]int), // ScoreDataマップを初期化
			}
			
			// ScoreDataを深いコピー
			if state.CurrentPiece.ScoreData != nil {
				for key, value := range state.CurrentPiece.ScoreData {
					currentPieceCopy.ScoreData[key] = value
				}
			}
			
			// ホールドピースを現在のピースとして設定
			state.CurrentPiece = state.HeldPiece
			
			// 安全性チェック
			if state.CurrentPiece == nil {
				log.Printf("[ERROR] HeldPiece is nil during hold swap for user %s", state.UserID)
				state.CurrentPiece = state.GetNextPieceFromQueue()
				state.NextPiece = state.GetNextPieceFromQueue()
			} else {
				// テトリミノの種類に応じた適切な初期位置を設定
				switch state.CurrentPiece.Type {
				case tetris.TypeI:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 2
					state.CurrentPiece.Y = 1
				case tetris.TypeO:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				case tetris.TypeL:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				default:
					state.CurrentPiece.X = tetris.BoardWidth/2 - 1
					state.CurrentPiece.Y = 1
				}
				state.CurrentPiece.Rotation = 0
			}
			
			// 現在のピースのコピーをホールドピースとして設定
			state.HeldPiece = currentPieceCopy
			moved = true
		}

		// ホールド後のピースが衝突する場合はゲームオーバー
		if state.CurrentPiece != nil && state.Board.HasCollision(state.CurrentPiece, 0, 0) {
			log.Printf("[INFO] Game over after hold for user %s - piece collision", state.UserID)
			state.IsGameOver = true
		}
	}

	// スコア更新を軽量化: ハードドロップ以外のみ更新（頻度削減）
	if moved && state.CurrentPiece != nil && action != "hard_drop" {
		state.updateCurrentPieceScores()
	}

	return moved
}

// AutoFall は自動落下処理を行います。
// GameSessionManagerのメインループから定期的に呼び出されます。
//
// Parameters:
//   state : 更新するプレイヤーのゲーム状態のポインタ
// Returns:
//   bool: ピースが落下した場合はtrue、着地した場合はfalse、ゲームオーバーの場合はfalse
func AutoFall(state *PlayerGameState) bool {
	if state.IsGameOver || state.CurrentPiece == nil {
		return false
	}

	// 落下間隔の計算（レベルに基づく）
	fallInterval := GetFallInterval(state.Level)
	
	// テスト環境では時間チェックをスキップ（無限ループ防止）
	timePassed := time.Since(state.lastFallTime)
	if timePassed >= fallInterval || timePassed == 0 {
		// 下に移動可能かチェック
		if !state.Board.HasCollision(state.CurrentPiece, 0, 1) {
			// 落下
			state.CurrentPiece.Y++
			state.lastFallTime = time.Now()
			
			// 自動落下時はスコア更新をスキップ（パフォーマンス優先）
			// クライアント側で補間されるため問題なし
			// state.updateCurrentPieceScores()
			
			return true
		} else {
			// 着地：ピースを固定して次のピースをスポーン
			state.Board.MergePiece(state.CurrentPiece)
			handlePieceLock(state)
			state.lastFallTime = time.Now()
			return false
		}
	}
	return false
}

// handlePieceLock はピースがボードに固定された後の処理をすべて行います。
// ラインクリア判定、スコア加算、レベルアップ、次のピース生成、ゲームオーバー判定などが含まれます。
//
// Parameters:
//   state : 更新するプレイヤーのゲーム状態のポインタ
func handlePieceLock(state *PlayerGameState) {
	// ピースのスコアデータをContributionScoresに反映
	updateContributionScoresFromPiece(state, state.CurrentPiece)

	// ラインクリア判定とスコア加算
	clearedLines, lineClearScore := state.Board.ClearLines(state.ContributionScores)
	state.LinesCleared += clearedLines
	state.Score += lineClearScore // ラインクリアによるスコア加算

	if clearedLines > 0 {
		// コンボやBack-to-Backなどのボーナス計算をここに実装
		state.Score += CalculateScore(clearedLines, state.Level, state.ConsecutiveClears, state.BackToBack)

		// 連続ラインクリアの更新
		state.ConsecutiveClears++
		state.BackToBack = (clearedLines == 4) // テトリス（4ラインクリア）でB2Bをセット

		// レベルアップのロジック (5ラインクリアごとにレベルアップ)
		state.Level = state.LinesCleared/LevelUpLines + 1

		// TODO: マルチプレイの場合、お邪魔ブロック送信ロジックを SessionManager に通知
	} else {
		// ラインクリアがない場合、連続クリアカウンターをリセット
		state.ConsecutiveClears = 0
		state.BackToBack = false
	}

	state.SpawnNewPiece() // 次のピースを生成

	// 新しいピースがスポーン位置で既に衝突（ボードの最上部が埋まっている）したらゲームオーバー
	if state.IsGameOver {
		log.Printf("Player %s Game Over! Final Score: %d, Lines Cleared: %d", state.UserID, state.Score, state.LinesCleared)
		// TODO: GameSessionManager にゲームオーバーを通知し、セッションを終了する
		// 例: sessionManager.EndGameSession(state.RoomID)
	}
}

// updateContributionScoresFromPiece はピースのスコアデータをPlayerGameStateのContributionScoresに反映します。
//
// Parameters:
//   state : 更新するプレイヤーのゲーム状態
//   piece : スコアデータを含むピース
func updateContributionScoresFromPiece(state *PlayerGameState, piece *tetris.Piece) {
	// 早期リターンでパフォーマンス向上
	if piece == nil || piece.ScoreData == nil || len(piece.ScoreData) == 0 {
		return
	}

	// ピースの各ブロックについて、ボード上の位置にスコアを設定（最適化版）
	blocks := piece.Blocks() // 一度だけ取得
	for _, block := range blocks {
		boardX := piece.X + block[0]
		boardY := piece.Y + block[1]

		// ボードの有効な範囲内のみ処理
		if boardX >= 0 && boardX < tetris.BoardWidth && boardY >= 0 && boardY < tetris.BoardHeight {
			// 文字列作成の最適化
			scoreKey := fmt.Sprintf("%d_%d", boardY, boardX)
			rotationKey := fmt.Sprintf("rot_%d_%d_%d", piece.Rotation, block[0], block[1])
			
			// スコア存在チェックを効率化
			if score, exists := piece.ScoreData[rotationKey]; exists && score > 0 {
				state.ContributionScores[scoreKey] = score
			}
		}
	}
}

// CalculateScore はラインクリア数、レベル、コンボなどに基づいて追加スコアを計算します。
// GITRIS固有の「草の濃さ」によるスコアは Board.ClearLines で加算されるため、
// ここは一般的なテトリスルールでのボーナススコアを計算する場所です。
//
// Parameters:
//   clearedLines      : クリアされたライン数 (1-4)
//   level             : 現在のレベル
//   consecutiveClears : 連続ラインクリア数
//   backToBack        : 前回のラインクリアがT-SpinまたはTetrisだったか
// Returns:
//   int: 計算されたボーナススコア
func CalculateScore(clearedLines int, level int, consecutiveClears int, backToBack bool) int {
	baseScore := 0
	switch clearedLines {
	case 1: // Single
		baseScore = 100
	case 2: // Double
		baseScore = 300
	case 3: // Triple
		baseScore = 500
	case 4: // Tetris
		baseScore = 800
	}

	// レベルボーナス
	score := baseScore * level

	// コンボボーナス (連続クリア)
	if consecutiveClears > 1 {
		score += 50 * (consecutiveClears - 1) * level // 例: 2コンボ目からボーナス
	}

	// Back-to-Backボーナス (T-SpinやTetris後にすぐT-Spin/Tetris)
	if backToBack && clearedLines > 0 { // T-SpinとTetrisの場合のみB2Bが適用されるのが一般的
		score = int(float64(score) * 1.5) // 例: 1.5倍
	}

	// TODO: T-Spin判定やPerfect Clear判定があれば、ここに追加ボーナスを実装
	return score
}
