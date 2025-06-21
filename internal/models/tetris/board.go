package tetris

import (
	"fmt"
	"math/rand"
)

const (
	BoardWidth  = 10 // テトリスボードの幅
	BoardHeight = 20 // テトリスボードの高さ（表示部分）
	// BoardHiddenHeight = 4 // ピースが生成される見えない領域など、必要に応じて追加
)

// BlockType はボード上のブロックの種類を表します。
// 各テトリミノの種類もブロックタイプとして扱います。
type BlockType int

const (
	BlockEmpty BlockType = iota // 0: 空のマス
	BlockI                       // 1: I-テトリミノ由来のブロック (PieceType 0 + 1)
	BlockO                       // 2: O-テトリミノ由来のブロック (PieceType 1 + 1)
	BlockT                       // 3: T-テトリミノ由来のブロック (PieceType 2 + 1)
	BlockS                       // 4: S-テトリミノ由来のブロック (PieceType 3 + 1)
	BlockZ                       // 5: Z-テトリミノ由来のブロック (PieceType 4 + 1)
	BlockJ                       // 6: J-テトリミノ由来のブロック (PieceType 5 + 1)
	BlockL                       // 7: L-テトリミノ由来のブロック (PieceType 6 + 1)
	BlockGarbage                 // 8: お邪魔ブロック
)

// Board はテトリスのゲームボードを表す2次元配列です。
// 各要素はBlockTypeで、その位置にどの種類のブロックがあるかを示します。
// Board[y][x] でアクセスします。yは行、xは列です。
type Board [BoardHeight][BoardWidth]BlockType

// NewBoard は新しい空のボードを初期化して返します。
// Goの配列はデフォルトでゼロ値（BlockEmpty）で初期化されるため、特別な初期化は不要です。
func NewBoard() Board {
	var board Board
	return board
}

// HasCollision は指定されたピースが現在のボード上の位置 (p.X, p.Y) とオフセット (dx, dy) で
// 壁や既存のブロックと衝突するかどうかを判定します。
//
// Parameters:
//   p  : 衝突判定を行うテトリミノのポインタ
//   dx : X軸方向の移動量（-1:左, 1:右, 0:移動なし）
//   dy : Y軸方向の移動量（1:下, 0:移動なし）
// Returns:
//   bool: 衝突する場合はtrue、しない場合はfalse
func (b *Board) HasCollision(p *Piece, dx, dy int) bool {
	// ピースの各ブロックについて衝突をチェック
	for _, block := range p.Blocks() {
		// ピースの現在の位置 + オフセット + ブロックの相対座標 = ボード上の絶対座標
		x := p.X + block[0] + dx
		y := p.Y + block[1] + dy

		// ボードの境界との衝突判定
		if x < 0 || x >= BoardWidth || y >= BoardHeight {
			return true // 左右の壁、または下部との衝突
		}
		// 上部（見えない領域）への衝突は通常ゲームオーバー判定で扱うため、ここでは y < 0 は許可
		// ただし、y < 0 の位置にあるブロックに対しては、既存のブロックとの衝突は発生しない

		// 既存のブロックとの衝突判定
		// y座標がボードの範囲内（0 <= y < BoardHeight）かつ、そのマスが空でない場合
		if y >= 0 && b[y][x] != BlockEmpty {
			return true // 既存のブロックとの衝突
		}
	}
	return false
}

// MergePiece は落下したピースをボードに固定します。
// ピースのブロックのタイプでボードのマスを埋めます。
//
// Parameters:
//   p : ボードに固定するテトリミノのポインタ
func (b *Board) MergePiece(p *Piece) {
	for _, block := range p.Blocks() {
		x := p.X + block[0]
		y := p.Y + block[1]

		// ボードの有効な範囲内でのみマージ
		if x >= 0 && x < BoardWidth && y >= 0 && y < BoardHeight {
			b[y][x] = BlockType(p.Type + 1) // PieceType (0-6) を BlockType (1-7) に変換
		}
	}
}

// ClearLines は揃ったラインをクリアし、上のブロックを落とします。
// この関数は、クリアされたライン数と、そのラインクリアによって獲得したスコアを返します。
//
// Parameters:
//   contributionScores : 各ボードマス（日付）に対応するContributionスコアのマップ（または2次元配列）
//                        key: "y_x" (例: "0_0"), value: score (Contribution量)
// Returns:
//   int: クリアされたライン数
//   int: ラインクリアによって獲得した合計スコア
func (b *Board) ClearLines(contributionScores map[string]int) (int, int) {
	clearedLines := 0
	totalScore := 0
	newBoard := NewBoard() // 新しいボードを作成し、クリア後の状態を構築

	destY := BoardHeight - 1 // 新しいボードにブロックをコピーする際の最も下の行

	// ボードの最下部から上に向かって各行をチェック
	for y := BoardHeight - 1; y >= 0; y-- {
		isLineFull := true
		lineScore := 0
		for x := 0; x < BoardWidth; x++ {
			if b[y][x] == BlockEmpty {
				isLineFull = false // 一つでも空のマスがあればラインは揃っていない
				break
			}
			// 各ブロックのContributionスコアを加算
			// GitHub草のグリッドは8x7だが、テトリスボードは10x20
			// ここではボード上の(y,x)に対応するcontributionScoresを仮定して加算
			// 実際のシステムでは、ゲーム開始時に読み込んだデッキのテトリミノ配置と
			// その下のGitHub草のContributionデータから、各ブロックのスコアを決定する必要がある
			scoreKey := fmt.Sprintf("%d_%d", y, x) // y_x の形式でスコアを検索
			if score, ok := contributionScores[scoreKey]; ok {
				lineScore += score
			} else {
				lineScore += 10 // マップにない場合は仮のスコア
			}
		}

		if isLineFull {
			clearedLines++
			totalScore += lineScore // 揃ったラインのスコアを加算
		} else {
			// 揃っていないラインは新しいボードのdestYにコピー
			for x := 0; x < BoardWidth; x++ {
				newBoard[destY][x] = b[y][x]
			}
			destY-- // 次のラインは一つ上にコピーされる
		}
	}
	*b = newBoard // 現在のボードを更新されたボードに置き換える
	return clearedLines, totalScore
}

// AddGarbageLines は指定された数のお邪魔ブロックのラインをボードの最下部に追加します。
// これにより、ボード上の既存のブロックは上にシフトされます。
//
// Parameters:
//   count : 追加するお邪魔ラインの数
func (b *Board) AddGarbageLines(count int) {
	if count <= 0 {
		return
	}
	if count >= BoardHeight { // ボード全体を覆う場合
		*b = NewBoard() // 全てクリア
		return
	}

	// 既存のブロックを上にシフト
	for y := 0; y < BoardHeight-count; y++ {
		for x := 0; x < BoardWidth; x++ {
			b[y][x] = b[y+count][x]
		}
	}

	// 最下部にお邪魔ブロックのラインを追加
	for y := BoardHeight - count; y < BoardHeight; y++ {
		// ランダムな位置に一つ穴を開ける（テトリスの一般的なお邪魔ブロックの動作）
		holeX := rand.Intn(BoardWidth) // TODO: 適切な乱数生成器を使用する

		for x := 0; x < BoardWidth; x++ {
			if x == holeX {
				b[y][x] = BlockEmpty // 穴
			} else {
				b[y][x] = BlockGarbage // お邪魔ブロック
			}
		}
	}
}
