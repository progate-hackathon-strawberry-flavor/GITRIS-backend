package tetris

import "fmt" // デバッグ用

// PieceType はテトリミノの種類を表します。
type PieceType int

const (
	TypeI PieceType = iota // 0: I-ミノ (シアン)
	TypeO                  // 1: O-ミノ (黄色)
	TypeT                  // 2: T-ミノ (紫)
	TypeS                  // 3: S-ミノ (緑)
	TypeZ                  // 4: Z-ミノ (赤)
	TypeJ                  // 5: J-ミノ (青)
	TypeL                  // 6: L-ミノ (オレンジ)
)

// Piece はテトリミノの現在の状態（種類、ボード上の基準点座標、回転角度）を表します。
type Piece struct {
	Type     PieceType `json:"type"`      // テトリミノの種類
	X        int       `json:"x"`         // ボード上のX座標
	Y        int       `json:"y"`         // ボード上のY座標
	Rotation int       `json:"rotation"`  // 回転角度 (0, 90, 180, 270 度)
	ScoreData map[string]int `json:"-"`  // 各ブロックのスコア情報 "relativeX_relativeY": score - JSONシリアライズから除外
	// TODO: GITRISのデッキシステムを考慮すると、ピース内の各ブロックに
	// Contributionスコアや元々のGitHub草の座標を紐付ける必要があるかもしれません。
	// 現状では Board.ClearLines で仮のスコアを使用していますが、
	// ここに BlockData などの構造体を持つように拡張することも考えられます。
}

// pieceShapes は各PieceTypeの各回転状態におけるブロックの相対座標を定義します。
// [PieceType][RotationIndex][BlockIndex][Coordinate (x or y)]
// 座標はテトリミノの基準点からの相対値です。
// Super Rotation System (SRS) に完全に準拠するためには、
// キックテーブル（回転時の壁蹴りルール）も考慮する必要があります。
var pieceShapes = map[PieceType][][][2]int{
	TypeI: { // I-ミノ (長方形の中心が回転軸に近い)
		{{0, 1}, {1, 1}, {2, 1}, {3, 1}}, // 0度 (横)
		{{2, 0}, {2, 1}, {2, 2}, {2, 3}}, // 90度 (縦)
		{{0, 2}, {1, 2}, {2, 2}, {3, 2}}, // 180度 (横) - SRSでは異なる場合もある
		{{1, 0}, {1, 1}, {1, 2}, {1, 3}}, // 270度 (縦) - SRSでは異なる場合もある
	},
	TypeO: { // O-ミノ (正方形、回転しない)
		{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, // 全ての回転で同じ
	},
	TypeT: { // T-ミノ
		{{1, 0}, {0, 1}, {1, 1}, {2, 1}}, // 0度
		{{1, 0}, {1, 1}, {2, 1}, {1, 2}}, // 90度
		{{0, 1}, {1, 1}, {2, 1}, {1, 2}}, // 180度
		{{0, 1}, {1, 0}, {1, 1}, {1, 2}}, // 270度
	},
	TypeS: { // S-ミノ
		{{1, 0}, {2, 0}, {0, 1}, {1, 1}}, // 0度
		{{1, 0}, {1, 1}, {2, 1}, {2, 2}}, // 90度
		{{1, 1}, {2, 1}, {0, 2}, {1, 2}}, // 180度 (0度をy+1シフト)
		{{0, 0}, {0, 1}, {1, 1}, {1, 2}}, // 270度 (90度をx+1シフト)
	},
	TypeZ: { // Z-ミノ
		{{0, 0}, {1, 0}, {1, 1}, {2, 1}}, // 0度
		{{2, 0}, {1, 1}, {2, 1}, {1, 2}}, // 90度
		{{0, 1}, {1, 1}, {1, 2}, {2, 2}}, // 180度 (0度をy+1シフト)
		{{1, 0}, {0, 1}, {1, 1}, {0, 2}}, // 270度 (90度をx+1シフト)
	},
	TypeJ: { // J-ミノ
		{{0, 0}, {0, 1}, {1, 1}, {2, 1}}, // 0度
		{{1, 0}, {2, 0}, {1, 1}, {1, 2}}, // 90度
		{{0, 1}, {1, 1}, {2, 1}, {2, 2}}, // 180度
		{{1, 0}, {1, 1}, {0, 2}, {1, 2}}, // 270度
	},
	TypeL: { // L-ミノ
		{{2, 0}, {0, 1}, {1, 1}, {2, 1}}, // 0度
		{{1, 0}, {1, 1}, {1, 2}, {2, 2}}, // 90度
		{{0, 1}, {1, 1}, {2, 1}, {0, 2}}, // 180度
		{{0, 0}, {1, 0}, {1, 1}, {1, 2}}, // 270度
	},
}

// Blocks は現在のPieceの回転状態に基づいて、構成するブロックの相対座標の配列を返します。
//
// Returns:
//   [][2]int: 各ブロックの相対座標の配列。例: {{x1, y1}, {x2, y2}, ...}
func (p *Piece) Blocks() [][2]int {
	return p.GetBlocksAtRotation(p.Rotation)
}

// GetBlocksAtRotation は指定された回転角度でのブロックの相対座標の配列を返します。
//
// Parameters:
//   rotation : 回転角度 (0, 90, 180, 270)
// Returns:
//   [][2]int: 各ブロックの相対座標の配列
func (p *Piece) GetBlocksAtRotation(rotation int) [][2]int {
	shapeData := pieceShapes[p.Type]
	rotIdx := rotation / 90 // 0, 1, 2, 3 のインデックスに変換

	// Oミノは回転しないので常に0番目の形状を使用
	if p.Type == TypeO {
		return shapeData[0]
	}

	// 念のためインデックスが範囲外にならないようにチェック
	if rotIdx < 0 || rotIdx >= len(shapeData) {
		fmt.Printf("Warning: Invalid rotation index %d for piece type %d, falling back to 0-degree shape.\n", rotIdx, p.Type)
		return shapeData[0] // デフォルトの形状を返す
	}
	return shapeData[rotIdx]
}

// Rotate はピースを時計回りに90度回転させます。
func (p *Piece) Rotate() {
	if p.Type == TypeO { // Oミノは回転しない
		return
	}
	p.Rotation = (p.Rotation + 90) % 360
}

// RotateCounterClockwise はピースを反時計回りに90度回転させます。
func (p *Piece) RotateCounterClockwise() {
	if p.Type == TypeO {
		return
	}
	p.Rotation = (p.Rotation - 90 + 360) % 360 // 負の値にならないように +360
}

// Clone は現在のPieceオブジェクトのディープコピーを返します。
// これにより、操作前のピースの状態を保持しつつ、操作後の状態を仮に試すことができます。
//
// Returns:
//   *Piece: コピーされたPieceオブジェクトのポインタ
func (p *Piece) Clone() *Piece {
	newP := *p // ポインタが指す先の値をコピー
	return &newP
}

// StringToPieceType は文字列のテトリミノタイプ（"I", "O", "T"など）をPieceTypeに変換します。
func StringToPieceType(s string) (PieceType, bool) {
	switch s {
	case "I":
		return TypeI, true
	case "O":
		return TypeO, true
	case "T":
		return TypeT, true
	case "S":
		return TypeS, true
	case "Z":
		return TypeZ, true
	case "J":
		return TypeJ, true
	case "L":
		return TypeL, true
	default:
		return TypeI, false // デフォルト値とfalseを返す
	}
}

// PieceTypeToString はPieceTypeを文字列表現に変換します。
func PieceTypeToString(t PieceType) string {
	switch t {
	case TypeI:
		return "I"
	case TypeO:
		return "O"
	case TypeT:
		return "T"
	case TypeS:
		return "S"
	case TypeZ:
		return "Z"
	case TypeJ:
		return "J"
	case TypeL:
		return "L"
	default:
		return "I" // デフォルト値
	}
}
