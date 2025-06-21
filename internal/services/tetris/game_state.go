package tetris

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/database"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models"
	"github.com/progate-hackathon-strawberry-flavor/GITRIS-backend/internal/models/tetris"
)

// DeckPlacementPiece はデッキから読み込んだテトリミノ配置情報を表します。
type DeckPlacementPiece struct {
	Type     tetris.PieceType `json:"type"`
	Rotation int              `json:"rotation"`
	Blocks   []models.Position `json:"blocks"` // 各ブロックのスコア情報を含む
}

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
	pieceQueue    []tetris.PieceType `json:"-"`              // 次のピースを管理するためのキュー (7-bag systemなど) - JSONシリアライズから除外
	randGenerator *rand.Rand         `json:"-"`              // ピース生成用の乱数ジェネレータ - JSONシリアライズから除外
	lastFallTime  time.Time          `json:"-"`              // 最後の自動落下またはハードドロップの時間 - JSONシリアライズから除外
	ContributionScores map[string]int `json:"contribution_scores"` // GitHub草のContributionスコアをボード上の位置に紐付けるマップ
	// 例: "y_x": score, "0_0": 100, "0_1": 200
	CurrentPieceScores map[string]int `json:"current_piece_scores"` // 現在のピースの各ブロックのスコア情報をボード座標で送信
	// 例: "y_x": score, "5_3": 250 (現在のピースの該当ブロックのスコア)
	DeckPlacements []DeckPlacementPiece `json:"-"` // デッキから読み込んだテトリミノ配置情報 - JSONシリアライズから除外
	ConsecutiveClears int            `json:"consecutive_clears"` // 連続ラインクリア数 (コンボボーナス用)
	BackToBack        bool           `json:"back_to_back"`       // T-Spin, Perfect Clear 後のラインクリアでボーナス
	mu                sync.RWMutex   `json:"-"`                  // CurrentPieceScoresの並行アクセス保護用
}

// NewPlayerGameState は新しいプレイヤーのゲーム状態を初期化して返します（ランダムスコア版）。
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
		ContributionScores: make(map[string]int),
		CurrentPieceScores: make(map[string]int),
		DeckPlacements: []DeckPlacementPiece{},
	}

	// 仮でボード全体にランダムなスコアを設定
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			state.ContributionScores[fmt.Sprintf("%d_%d", y, x)] = r.Intn(400) + 100 // 100-499のスコア
		}
	}

	state.generatePieceQueue() // 最初のピースキューを生成
	state.SpawnNewPiece()      // 最初のピースを生成

	return state
}

// NewPlayerGameStateWithDeckPlacements は実際のデッキ配置データを使用してプレイヤーのゲーム状態を初期化します。
//
// Parameters:
//   userID : プレイヤーのユーザーID
//   deck   : プレイヤーが選択したデッキデータ
//   deckRepo : デッキリポジトリ（テトリミノ配置データを取得するため）
// Returns:
//   *PlayerGameState: 初期化されたゲーム状態のポインタ
//   error: エラーが発生した場合
func NewPlayerGameStateWithDeckPlacements(userID string, deck *models.Deck, deckRepo database.DeckRepository) (*PlayerGameState, error) {
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
		ContributionScores: make(map[string]int),
		CurrentPieceScores: make(map[string]int),
		DeckPlacements: []DeckPlacementPiece{},
	}

	// デッキからテトリミノ配置データを取得
	if deck != nil && deckRepo != nil {
		placements, err := deckRepo.GetTetriminoPlacementsByDeckID(nil, deck.ID)
		if err != nil {
			return nil, fmt.Errorf("デッキ配置データの取得に失敗しました: %w", err)
		}

		// テトリミノ配置データをDeckPlacementPieceに変換
		for _, placement := range placements {
			pieceType, ok := tetris.StringToPieceType(placement.TetriminoType)
			if !ok {
				continue // 不明なテトリミノタイプをスキップ
			}

			// JSONからPositionスライスをデコード
			var positions []models.Position
			if err := json.Unmarshal(placement.Positions, &positions); err != nil {
				continue // デコードに失敗した場合はスキップ
			}

			deckPiece := DeckPlacementPiece{
				Type:     pieceType,
				Rotation: placement.Rotation,
				Blocks:   positions,
			}
			state.DeckPlacements = append(state.DeckPlacements, deckPiece)
		}

		// デッキデータから実際のスコアマップを構築
		state.buildContributionScoresFromDeck()
	}

	// デッキデータがない場合やエラーの場合はランダムスコアを設定
	if len(state.ContributionScores) == 0 {
		for y := 0; y < tetris.BoardHeight; y++ {
			for x := 0; x < tetris.BoardWidth; x++ {
				state.ContributionScores[fmt.Sprintf("%d_%d", y, x)] = r.Intn(400) + 100 // 100-499のスコア
			}
		}
	}

	state.generatePieceQueue() // 最初のピースキューを生成
	state.SpawnNewPiece()      // 最初のピースを生成

	return state, nil
}

// buildContributionScoresFromDeck はデッキ配置データからContributionScoresマップを構築します。
func (s *PlayerGameState) buildContributionScoresFromDeck() {
	// すべての位置を初期化（デフォルトスコア100）
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			s.ContributionScores[fmt.Sprintf("%d_%d", y, x)] = 100 // デフォルトスコア
		}
	}

	// デッキ配置データからスコアを設定
	for _, deckPiece := range s.DeckPlacements {
		for _, block := range deckPiece.Blocks {
			// デッキ配置のx,y座標をボード座標に変換
			// TODO: ここでGitHub草座標からテトリスボード座標への変換ロジックが必要
			// 現在は単純にx,yをそのまま使用（後で調整が必要）
			if block.X >= 0 && block.X < tetris.BoardWidth && 
			   block.Y >= 0 && block.Y < tetris.BoardHeight {
				scoreKey := fmt.Sprintf("%d_%d", block.Y, block.X)
				s.ContributionScores[scoreKey] = block.Score
			}
		}
	}
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
	// デッキデータからピースを取得を試みる
	if deckPiece := s.getNextPieceFromDeck(); deckPiece != nil {
		return deckPiece
	}

	// デッキデータがない場合は従来の7-bagシステムを使用
	// キューの長さが短い場合、新しいバッグを追加
	if len(s.pieceQueue) < 7 { // 例えば、残り7個以下になったら補充
		s.generatePieceQueue()
	}

	pieceType := s.pieceQueue[0]
	s.pieceQueue = s.pieceQueue[1:] // キューから削除

	return &tetris.Piece{
		Type: pieceType,
		ScoreData: make(map[string]int), // 空のスコアデータで初期化
	}
}

// getNextPieceFromDeck はデッキデータから次のピースを取得します。
// デッキデータがある場合は、そこからランダムに選択します。
//
// Returns:
//   *tetris.Piece: デッキから選択されたピース（デッキデータがない場合はnil）
func (s *PlayerGameState) getNextPieceFromDeck() *tetris.Piece {
	if len(s.DeckPlacements) == 0 {
		return nil // デッキデータがない
	}

	// ランダムにデッキピースを選択
	selectedDeckPiece := s.DeckPlacements[s.randGenerator.Intn(len(s.DeckPlacements))]

	// テトリスピースを作成
	piece := &tetris.Piece{
		Type:     selectedDeckPiece.Type,
		ScoreData: make(map[string]int),
	}

	// すべての回転状態（0, 90, 180, 270度）に対してスコアマッピングを作成
	for rotation := 0; rotation <= 270; rotation += 90 {
		blocks := piece.GetBlocksAtRotation(rotation)
		
		for i, block := range blocks {
			// 回転状態別のキーを作成 "rot_rotation_x_y"
			key := fmt.Sprintf("rot_%d_%d_%d", rotation, block[0], block[1])
			
			// デッキデータの対応するブロックからスコアを取得
			var score int
			if i < len(selectedDeckPiece.Blocks) {
				score = selectedDeckPiece.Blocks[i].Score
			} else {
				score = 100 // デフォルトスコア
			}
			piece.ScoreData[key] = score
			
			log.Printf("[DEBUG] Rotation %d, Block %d at position (%d,%d) -> key %s, score %d", 
				rotation, i, block[0], block[1], key, score)
		}
	}

	return piece
}

// GetPieceScoreAtPosition は指定されたピースの指定位置でのスコアを取得します。
//
// Parameters:
//   piece : 対象のピース
//   boardX, boardY : ボード上の絶対座標
// Returns:
//   int: その位置でのスコア（デフォルト: ContributionScoresから取得、フォールバック: 100）
func (s *PlayerGameState) GetPieceScoreAtPosition(piece *tetris.Piece, boardX, boardY int) int {
	if piece == nil {
		return 100 // デフォルトスコア
	}

	// ピース内の相対位置を計算
	relativeX := boardX - piece.X
	relativeY := boardY - piece.Y

	// 現在の回転状態での位置キーを作成
	rotationKey := fmt.Sprintf("rot_%d_%d_%d", piece.Rotation, relativeX, relativeY)
	
	// ピースのスコアデータから取得を試みる
	if score, exists := piece.ScoreData[rotationKey]; exists && score > 0 {
		return score
	}

	// フォールバック: ContributionScoresから取得（読み取り専用ロック）
	s.mu.RLock()
	scoreKey := fmt.Sprintf("%d_%d", boardY, boardX)
	score, exists := s.ContributionScores[scoreKey]
	s.mu.RUnlock()

	if exists {
		return score
	}

	return 100 // 最終フォールバック
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

	// 初期位置設定（ボードの中央上部、すべてのブロックが表示範囲内）
	// テトリミノの種類に応じた適切な初期位置を設定
	switch s.CurrentPiece.Type {
	case tetris.TypeI:
		s.CurrentPiece.X = tetris.BoardWidth/2 - 2 // I-ミノは幅4なので中心から-2
		s.CurrentPiece.Y = 1 // すべてのブロックがボード内に表示される位置
	case tetris.TypeO:
		s.CurrentPiece.X = tetris.BoardWidth/2 - 1 // O-ミノは幅2なので中心から-1
		s.CurrentPiece.Y = 1 // すべてのブロックがボード内に表示される位置
	case tetris.TypeL:
		s.CurrentPiece.X = tetris.BoardWidth/2 - 1 // L-ミノは幅3なので中心から-1
		s.CurrentPiece.Y = 1 // すべてのブロックがボード内に表示される位置
	default:
		s.CurrentPiece.X = tetris.BoardWidth/2 - 1 // その他のミノは幅3なので中心から-1
		s.CurrentPiece.Y = 1 // すべてのブロックがボード内に表示される位置
	}
	s.CurrentPiece.Rotation = 0 // 必ず回転をリセット

	// 現在のピースのスコア情報を更新
	s.updateCurrentPieceScores()

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

	// Internal communication channels for the session manager (JSONシリアライズから除外)
	InputCh  chan PlayerInputEvent `json:"-"` // クライアントからのプレイヤー操作入力を受け取るチャネル
	OutputCh chan GameStateEvent   `json:"-"` // ゲーム状態の更新をブロードキャストするためのチャネル
	GameLoopDone chan struct{}     `json:"-"` // ゲームループの終了を通知するチャネル
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
//   deckRepo    : デッキリポジトリ（テトリミノ配置データ取得用）
// Returns:
//   *GameSession: 初期化されたゲームセッションのポインタ
//   error: エラーが発生した場合
func NewGameSession(roomID, player1ID string, player1Deck *models.Deck, deckRepo database.DeckRepository) (*GameSession, error) {
	// プレイヤー1のゲーム状態を作成（デッキデータを使用）
	player1State, err := NewPlayerGameStateWithDeckPlacements(player1ID, player1Deck, deckRepo)
	if err != nil {
		// エラーが発生した場合は従来の方法でフォールバック
		log.Printf("Failed to create player1 state with deck placements: %v, falling back to random scores", err)
		player1State = NewPlayerGameState(player1ID, player1Deck)
	}

	return &GameSession{
		ID:           roomID,
		Player1:      player1State,
		Status:       "waiting",
		TimeLimit:    GameTimeLimit,
		InputCh:      make(chan PlayerInputEvent, 100),
		OutputCh:     make(chan GameStateEvent, 100),
		GameLoopDone: make(chan struct{}),
	}, nil
}

// SetPlayer2 はセッションに2人目のプレイヤーを設定します。
//
// Parameters:
//   player2ID   : プレイヤー2のユーザーID
//   player2Deck : プレイヤー2が使用するデッキデータ
//   deckRepo    : デッキリポジトリ（テトリミノ配置データ取得用）
func (gs *GameSession) SetPlayer2(player2ID string, player2Deck *models.Deck, deckRepo database.DeckRepository) {
	// プレイヤー2のゲーム状態を作成（デッキデータを使用）
	player2State, err := NewPlayerGameStateWithDeckPlacements(player2ID, player2Deck, deckRepo)
	if err != nil {
		// エラーが発生した場合は従来の方法でフォールバック
		log.Printf("Failed to create player2 state with deck placements: %v, falling back to random scores", err)
		player2State = NewPlayerGameState(player2ID, player2Deck)
	}
	gs.Player2 = player2State
}

// IsTimeUp はゲームの制限時間が経過したかどうかを判定します。
func (gs *GameSession) IsTimeUp() bool {
	if gs.Status != "playing" {
		return false
	}
	return time.Since(gs.StartedAt) >= gs.TimeLimit
}

// ToLightweight はGameSessionから軽量な構造体に変換します。
func (gs *GameSession) ToLightweight() *LightweightGameState {
	lightweight := &LightweightGameState{
		ID:     gs.ID,
		Status: gs.Status,
		StartedAt: gs.StartedAt,
		EndedAt:   gs.EndedAt,
	}
	
	if gs.Player1 != nil {
		lightweight.Player1 = &LightweightPlayerState{
			UserID:             gs.Player1.UserID,
			Board:              gs.Player1.Board,
			CurrentPiece:       gs.Player1.CurrentPiece,
			NextPiece:          gs.Player1.NextPiece,
			HeldPiece:          gs.Player1.HeldPiece,
			Score:              gs.Player1.Score,
			LinesCleared:       gs.Player1.LinesCleared,
			Level:              gs.Player1.Level,
			IsGameOver:         gs.Player1.IsGameOver,
			ContributionScores: gs.Player1.ContributionScores,
			CurrentPieceScores: gs.Player1.CurrentPieceScores,
		}
	}
	
	if gs.Player2 != nil {
		lightweight.Player2 = &LightweightPlayerState{
			UserID:             gs.Player2.UserID,
			Board:              gs.Player2.Board,
			CurrentPiece:       gs.Player2.CurrentPiece,
			NextPiece:          gs.Player2.NextPiece,
			HeldPiece:          gs.Player2.HeldPiece,
			Score:              gs.Player2.Score,
			LinesCleared:       gs.Player2.LinesCleared,
			Level:              gs.Player2.Level,
			IsGameOver:         gs.Player2.IsGameOver,
			ContributionScores: gs.Player2.ContributionScores,
			CurrentPieceScores: gs.Player2.CurrentPieceScores,
		}
	}
	
	return lightweight
}

// updateCurrentPieceScores は現在のピースのスコア情報をCurrentPieceScoresマップに更新します。
// これによりクライアント側で落下中のピースも正しい色で表示されます。
func (s *PlayerGameState) updateCurrentPieceScores() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// CurrentPieceScoresマップをクリア
	for k := range s.CurrentPieceScores {
		delete(s.CurrentPieceScores, k)
	}

	// 現在のピースが存在しない場合は何もしない
	if s.CurrentPiece == nil {
		return
	}

	// 現在のピースの各ブロックについて、ボード座標でのスコア情報を設定
	for _, block := range s.CurrentPiece.Blocks() {
		boardX := s.CurrentPiece.X + block[0]
		boardY := s.CurrentPiece.Y + block[1]

		// ボードの有効な範囲内のみ処理
		if boardX >= 0 && boardX < tetris.BoardWidth && boardY >= 0 && boardY < tetris.BoardHeight {
			scoreKey := fmt.Sprintf("%d_%d", boardY, boardX)
			
			// スコア取得ロジック（デッドロック回避のため直接実装）
			score := 100 // デフォルトスコア
			
			// ピース内の相対位置を計算
			relativeX := boardX - s.CurrentPiece.X
			relativeY := boardY - s.CurrentPiece.Y
			
			// 現在の回転状態での位置キーを作成
			rotationKey := fmt.Sprintf("rot_%d_%d_%d", s.CurrentPiece.Rotation, relativeX, relativeY)
			
			if s.CurrentPiece.ScoreData != nil {
				// ピースのスコアデータから取得を試みる
				if pieceScore, exists := s.CurrentPiece.ScoreData[rotationKey]; exists && pieceScore > 0 {
					score = pieceScore
				} else if contributionScore, exists := s.ContributionScores[scoreKey]; exists {
					// フォールバック: ContributionScoresから取得（既にロック済み）
					score = contributionScore
				}
			} else if contributionScore, exists := s.ContributionScores[scoreKey]; exists {
				// フォールバック: ContributionScoresから取得（既にロック済み）
				score = contributionScore
			}
			
			s.CurrentPieceScores[scoreKey] = score
		}
	}
}
