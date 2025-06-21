package tetris

import (
	"log"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// GameLoopSettings はゲームループの速度設定など、ゲーム全体に影響する定数を定義します。
const (
	// FallInterval はピースが自動落下する間隔です。レベルが上がると短縮されます。
	InitialFallInterval = 600 * time.Millisecond // 最初の自動落下間隔を0.6秒に短縮
	SoftDropMultiplier  = 5                       // ソフトドロップ時の落下速度倍率
	GameTimeLimit      = 120 * time.Second       // ゲームの制限時間（2分）
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

// ApplyPlayerInput はプレイヤーの入力（アクション）に基づいて、
// 指定されたプレイヤーのゲーム状態を更新します。
//
// Parameters:
//   state  : 更新するプレイヤーのゲーム状態のポインタ
//   action : プレイヤーが実行したアクション（例: "move_left", "rotate"）
// Returns:
//   bool: ゲーム状態が実際に変更された場合はtrue、変更されなかった場合はfalse
func ApplyPlayerInput(state *PlayerGameState, action string) bool {
	if state.IsGameOver || state.CurrentPiece == nil {
		return false // ゲームオーバーまたはピースがない場合は操作を受け付けない
	}

	// ピースの操作は、まずクローンに対して行い、衝突判定後に実際のピースに適用します。
	// これにより、衝突しない場合にのみ変更を反映できます。
	moved := false
	tempPiece := state.CurrentPiece.Clone()

	switch action {
	case "move_left":
		if !state.Board.HasCollision(tempPiece, -1, 0) {
			state.CurrentPiece.X--
			moved = true
		}
	case "move_right":
		if !state.Board.HasCollision(tempPiece, 1, 0) {
			state.CurrentPiece.X++
			moved = true
		}
	case "rotate", "rotate_right":
		tempPiece.Rotate() // 時計回りに回転
		// 回転後の衝突判定と壁蹴り (Wall Kick) ロジックをここに実装
		// SRS (Super Rotation System) は複雑なので、最初は単純な衝突判定から始めるのが良いでしょう。
		if !state.Board.HasCollision(tempPiece, 0, 0) {
			state.CurrentPiece.Rotate() // 実際のピースを回転
			moved = true
		} else {
			// TODO: SRS Wall Kick Logic を実装する場合はここに追加
			// 例えば、特定のオフセットで再試行する
			// if !state.Board.HasCollision(tempPiece, -1, 0) { state.CurrentPiece.X--; state.CurrentPiece.Rotate(); moved = true }
		}
	case "rotate_left":
		// 反時計回りに回転（3回時計回りに回転することで実現）
		tempPiece.Rotate()
		tempPiece.Rotate()
		tempPiece.Rotate()
		if !state.Board.HasCollision(tempPiece, 0, 0) {
			state.CurrentPiece.Rotate()
			state.CurrentPiece.Rotate()
			state.CurrentPiece.Rotate()
			moved = true
		}
	case "soft_drop": // 通常落下を加速
		if !state.Board.HasCollision(tempPiece, 0, 1) {
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

		// ピースが衝突するまで落下
		for !state.Board.HasCollision(state.CurrentPiece, 0, 1) {
			state.CurrentPiece.Y++
			state.Score += 2 // ハードドロップのボーナス (仮)
		}
		// ピースをボードに固定
		state.Board.MergePiece(state.CurrentPiece)
		handlePieceLock(state) // 固定後の処理
		state.lastFallTime = time.Now() // ハードドロップしたら落下タイマーをリセット
		moved = true
	case "hold": // ホールド機能
		if state.CurrentPiece == nil {
			return false
		}

		// ホールドが空の場合
		if state.HeldPiece == nil {
			// 現在のピースのコピーを作成してホールド
			state.HeldPiece = &tetris.Piece{
				Type:     state.CurrentPiece.Type,
				X:        state.CurrentPiece.X,
				Y:        state.CurrentPiece.Y,
				Rotation: state.CurrentPiece.Rotation,
			}
			// 次のピースを現在のピースとして設定
			state.CurrentPiece = state.NextPiece
			state.NextPiece = state.GetNextPieceFromQueue()
			// テトリミノの種類に応じた適切な初期位置を設定
			switch state.CurrentPiece.Type {
			case tetris.TypeI:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 2
				state.CurrentPiece.Y = -1
			case tetris.TypeO:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = 0
			case tetris.TypeL:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = 0
			default:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = -1
			}
			state.CurrentPiece.Rotation = 0
			moved = true
		} else {
			// 現在のピースのコピーを作成
			currentPieceCopy := &tetris.Piece{
				Type:     state.CurrentPiece.Type,
				X:        state.CurrentPiece.X,
				Y:        state.CurrentPiece.Y,
				Rotation: state.CurrentPiece.Rotation,
			}
			// ホールドピースを現在のピースとして設定
			state.CurrentPiece = state.HeldPiece
			// テトリミノの種類に応じた適切な初期位置を設定
			switch state.CurrentPiece.Type {
			case tetris.TypeI:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 2
				state.CurrentPiece.Y = -1
			case tetris.TypeO:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = 0
			case tetris.TypeL:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = 0
			default:
				state.CurrentPiece.X = tetris.BoardWidth/2 - 1
				state.CurrentPiece.Y = -1
			}
			state.CurrentPiece.Rotation = 0
			// 現在のピースのコピーをホールドピースとして設定
			state.HeldPiece = currentPieceCopy
			moved = true
		}

		// ホールド後のピースが衝突する場合はゲームオーバー
		if state.Board.HasCollision(state.CurrentPiece, 0, 0) {
			state.IsGameOver = true
		}
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

	// 落下間隔が経過していない場合は何もしない
	if time.Since(state.lastFallTime) < GetFallInterval(state.Level) {
		return false
	}

	// ピースが下に衝突するかどうかをチェック
	if !state.Board.HasCollision(state.CurrentPiece, 0, 1) {
		state.CurrentPiece.Y++ // 衝突しないので下に移動
		state.lastFallTime = time.Now() // 落下時間を更新
		return true // 落下した
	} else {
		// ピースが着地した
		state.Board.MergePiece(state.CurrentPiece) // ボードに固定
		handlePieceLock(state)                     // 固定後の処理
		state.lastFallTime = time.Now()            // 落下時間をリセット
		return false // 着地した
	}
}

// handlePieceLock はピースがボードに固定された後の処理をすべて行います。
// ラインクリア判定、スコア加算、レベルアップ、次のピース生成、ゲームオーバー判定などが含まれます。
//
// Parameters:
//   state : 更新するプレイヤーのゲーム状態のポインタ
func handlePieceLock(state *PlayerGameState) {
	// ラインクリア判定とスコア加算
	clearedLines, lineClearScore := state.Board.ClearLines(state.contributionScores)
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
