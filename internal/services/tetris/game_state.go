package tetris

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// PlayerGameState は単一プレイヤーのテトリスゲーム状態です。
// これはゲームセッション内で個々のプレイヤーの進行を管理するために使われます。
type PlayerGameState struct {
	UserID        string             `json:"user_id"`
	Board         tetris.Board       `json:"board"`          // 現在のゲームボード
	CurrentPiece  *tetris.Piece      `json:"current_piece"`  // 現在操作中のテトリミノ
	NextPiece     *tetris.Piece      `json:"next_piece"`     // 次に出現するテトリミノ
	HeldPiece     *tetris.Piece      `json:"held_piece"`     // ホールド中のテトリミノ (オプション機能)
	Score         int                `json:"score"`          // 現在のスコア
	LinesCleared  int                `json:"lines_cleared"`  // クリアしたライン数
	Level         int                `json:"level"`          // 現在のレベル
	IsGameOver    bool               `json:"is_game_over"`   // ゲームオーバー状態かどうか
	Deck          *models.Deck       `json:"deck"`           // このゲームで使用するデッキデータ
	pieceQueue    []tetris.PieceType // 次のピースを管理するためのキュー (7-bag systemなど)
	randGenerator *rand.Rand         // ピース生成用の乱数ジェネレータ
	lastFallTime  time.Time          // 最後の自動落下またはハードドロップの時間
	contributionScores map[string]int // GitHub草のContributionスコアをボード上の位置に紐付けるマップ
	// 例: "y_x": score, "0_0": 100, "0_1": 200
	ConsecutiveClears int            // 連続ラインクリア数 (コンボボーナス用)
	BackToBack        bool           // T-Spin, Perfect Clear 後のラインクリアでボーナス
}

// NewPlayerGameState は新しいプレイヤーのゲーム状態を初期化して返します。
//
// Parameters:
//   userID : プレイヤーのユーザーID
//   deck   : プレイヤーが選択したデッキデータ（仮データまたはDBから取得したデータ）
// Returns:
//   *PlayerGameState: 初期化されたゲーム状態のポインタ
func NewPlayerGameState(userID string, deck *models.Deck) *PlayerGameState {
	// 乱数生成器のシードを現在時刻で初期化
	seed := time.Now().UnixNano()
	source := rand.NewSource(seed)
	r := rand.New(source)

	state := &PlayerGameState{
		UserID:        userID,
		Board:         tetris.NewBoard(),
		Score:         0,
		LinesCleared:  0,
		Level:         1,
		IsGameOver:    false,
		Deck:          deck,
		randGenerator: r,
		lastFallTime:  time.Now(),
		contributionScores: make(map[string]int),
	}

	// 仮でボード全体にランダムなスコアを設定
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			state.contributionScores[fmt.Sprintf("%d_%d", y, x)] = r.Intn(400) + 100 // 100-499のスコア
		}
	}

	state.generatePieceQueue() // 最初のピースキューを生成
	state.SpawnNewPiece()      // 最初のピースを生成

	return state
}

// generatePieceQueue はテトリスで一般的な7-bagシステムに基づきピースキューを生成します。
// キューが一定数以下になったら新しい7種類のテトリミノをランダムな順序で追加します。
func (s *PlayerGameState) generatePieceQueue() {
	bag := []tetris.PieceType{tetris.TypeI, tetris.TypeO, tetris.TypeT, tetris.TypeS, tetris.TypeZ, tetris.TypeJ, tetris.TypeL}
	s.randGenerator.Shuffle(len(bag), func(i, j int) {
		bag[i], bag[j] = bag[j], bag[i]
	})
	s.pieceQueue = append(s.pieceQueue, bag...)
}

// GetNextPieceFromQueue はキューから次のピースを取得し、必要であれば新しいバッグを生成します。
//
// Returns:
//   *Piece: キューから取り出された次のテトリミノのポインタ
func (s *PlayerGameState) GetNextPieceFromQueue() *tetris.Piece {
	// キューの長さが短い場合、新しいバッグを追加
	if len(s.pieceQueue) < 7 { // 例えば、残り7個以下になったら補充
		s.generatePieceQueue()
	}

	pieceType := s.pieceQueue[0]
	s.pieceQueue = s.pieceQueue[1:] // キューから削除

	return &tetris.Piece{Type: pieceType}
}

// SpawnNewPiece は新しいテトリミノをボード上に出現させます。
// ゲームオーバーの判定も行われます。
func (s *PlayerGameState) SpawnNewPiece() {
	// 現在操作中のピースがなければ、最初のピースを生成
	if s.CurrentPiece == nil {
		s.CurrentPiece = s.GetNextPieceFromQueue()
	} else {
		// 現在のピースを次のピースに、次のピースをキューから取得
		s.CurrentPiece = s.NextPiece
	}
	s.NextPiece = s.GetNextPieceFromQueue()

	// 初期位置設定（ボードの中央上部）
	s.CurrentPiece.X = tetris.BoardWidth/2 - 2 // 中心に配置 (Iミノの場合は -2)
	s.CurrentPiece.Y = 0

	// ゲームオーバー判定: 新しいピースがスポーン位置で既に衝突している場合
	// これは通常、ボードの最上部にブロックが積み上がってしまった状態を指します。
	if s.Board.HasCollision(s.CurrentPiece, 0, 0) {
		s.IsGameOver = true
	}
}

// GameSession は2人のプレイヤーのゲーム状態とセッション情報を含みます。
// これはマルチプレイヤー対戦のためのトップレベルのゲーム状態です。
type GameSession struct {
	ID        string `json:"id"`        // セッションID (UUID)
	Player1   *PlayerGameState `json:"player1"` // プレイヤー1のゲーム状態
	Player2   *PlayerGameState `json:"player2"` // プレイヤー2のゲーム状態
	Status    string           `json:"status"`  // "waiting", "playing", "finished"
	StartedAt time.Time        `json:"started_at"` // ゲーム開始日時
	EndedAt   time.Time        `json:"ended_at"`   // ゲーム終了日時
	TimeLimit time.Duration    `json:"time_limit"` // ゲームの制限時間

	// Internal communication channels for the session manager
	InputCh  chan PlayerInputEvent // クライアントからのプレイヤー操作入力を受け取るチャネル
	OutputCh chan GameStateEvent   // ゲーム状態の更新をブロードキャストするためのチャネル
	GameLoopDone chan struct{}     // ゲームループの終了を通知するチャネル
}

// PlayerInputEvent はクライアントからの操作入力を表す構造体です。
// WebSocketを通じてサーバーに送信されます。
type PlayerInputEvent struct {
	UserID string `json:"user_id"` // 操作を行ったプレイヤーのID
	Action string `json:"action"`  // "move_left", "move_right", "rotate", "hard_drop", "hold" など
}

// GameStateEvent はゲーム状態の更新を通知するイベントです。
// WebSocketを通じてクライアントにブロードキャストされます。
type GameStateEvent struct {
	RoomID string       `json:"room_id"` // 関連するルームID
	State  *GameSession `json:"state"`   // 送信するゲームセッションの全体状態
}

// NewGameSession は新しいゲームセッションを初期化して返します。
//
// Parameters:
//   roomID      : 新しいセッションのユニークなID
//   player1ID   : プレイヤー1のユーザーID
//   player1Deck : プレイヤー1が使用するデッキデータ
// Returns:
//   *GameSession: 初期化されたゲームセッションのポインタ
func NewGameSession(roomID, player1ID string, player1Deck *models.Deck) *GameSession {
	return &GameSession{
		ID:           roomID,
		Player1:      NewPlayerGameState(player1ID, player1Deck),
		Status:       "waiting",
		TimeLimit:    GameTimeLimit,
		InputCh:      make(chan PlayerInputEvent, 100),
		OutputCh:     make(chan GameStateEvent, 100),
		GameLoopDone: make(chan struct{}),
	}
}

// SetPlayer2 はセッションに2人目のプレイヤーを設定します。
//
// Parameters:
//   player2ID   : プレイヤー2のユーザーID
//   player2Deck : プレイヤー2が使用するデッキデータ
func (gs *GameSession) SetPlayer2(player2ID string, player2Deck *models.Deck) {
	gs.Player2 = NewPlayerGameState(player2ID, player2Deck)
}

// IsTimeUp はゲームの制限時間が経過したかどうかを判定します。
func (gs *GameSession) IsTimeUp() bool {
	if gs.Status != "playing" {
		return false
	}
	return time.Since(gs.StartedAt) >= gs.TimeLimit
}
