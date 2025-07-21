package tetris

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
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
	hasUsedHold       bool           `json:"-"`                  // 現在のピースでホールドが使用済みかどうか - JSONシリアライズから除外
	pieceUsageCount   map[tetris.PieceType]int `json:"-"`        // 各ピースタイプの使用回数カウンター - JSONシリアライズから除外
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
		pieceUsageCount: make(map[tetris.PieceType]int),
	}

	// 仮でボード全体にランダムなスコアを設定
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			state.ContributionScores[strconv.Itoa(y) + "_" + strconv.Itoa(x)] = r.Intn(400) + 100 // 100-499のスコア
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
		pieceUsageCount: make(map[tetris.PieceType]int),
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
				state.ContributionScores[strconv.Itoa(y) + "_" + strconv.Itoa(x)] = r.Intn(400) + 100 // 100-499のスコア
			}
		}
	}

	state.generatePieceQueue() // 最初のピースキューを生成
	state.SpawnNewPiece()      // 最初のピースを生成

	return state, nil
}

// buildContributionScoresFromDeck はデッキ配置データからContributionScoresマップを構築します。
// 各ピースのブロックが個別のスコアを持つように修正します。
func (s *PlayerGameState) buildContributionScoresFromDeck() {
	// すべての位置を初期化（デフォルトスコア100）
	for y := 0; y < tetris.BoardHeight; y++ {
		for x := 0; x < tetris.BoardWidth; x++ {
			s.ContributionScores[strconv.Itoa(y) + "_" + strconv.Itoa(x)] = 100 // デフォルトスコア
		}
	}

	// デッキ配置データから個別ブロックのスコアを設定
	// 配置されたブロックが元のピースのブロックごとスコアを保持するよう改善
	for _, deckPiece := range s.DeckPlacements {

		
		for _, block := range deckPiece.Blocks {
			// デッキ配置のx,y座標をボード座標に変換
			// 現在は単純にx,yをそのまま使用（後で調整が必要）
			if block.X >= 0 && block.X < tetris.BoardWidth && 
			   block.Y >= 0 && block.Y < tetris.BoardHeight {
				scoreKey := strconv.Itoa(block.Y) + "_" + strconv.Itoa(block.X)
				s.ContributionScores[scoreKey] = block.Score
				

			}
		}
	}
	

}

// generatePieceQueue はテトリスで一般的な7-bagシステムに基づきピースキューを生成します。
// キューが一定数以下になったら新しい7種類のテトリミノをランダムな順序で追加します。
// 連続した同じテトリミノの出現を防ぐため、前のバッグの最後のピースと新しいバッグの最初のピースが
// 同じにならないようにシャッフルを調整します。
func (s *PlayerGameState) generatePieceQueue() {
	bag := []tetris.PieceType{tetris.TypeI, tetris.TypeO, tetris.TypeT, tetris.TypeS, tetris.TypeZ, tetris.TypeJ, tetris.TypeL}
	
	// 現在のキューの最後のピースを取得（連続防止のため）
	var lastPieceType tetris.PieceType
	var hasLastPiece bool
	if len(s.pieceQueue) > 0 {
		lastPieceType = s.pieceQueue[len(s.pieceQueue)-1]
		hasLastPiece = true
	}
	
	// バッグをシャッフル
	s.randGenerator.Shuffle(len(bag), func(i, j int) {
		bag[i], bag[j] = bag[j], bag[i]
	})
	
	// 連続防止：前のバッグの最後のピースと新しいバッグの最初のピースが同じ場合、調整する
	if hasLastPiece && len(bag) > 1 && bag[0] == lastPieceType {
		// 最初のピースと2番目以降のどれかを交換
		// ランダムな位置（1から最後まで）を選んで交換
		swapIndex := s.randGenerator.Intn(len(bag)-1) + 1
		bag[0], bag[swapIndex] = bag[swapIndex], bag[0]
		

	}
	
	s.pieceQueue = append(s.pieceQueue, bag...)
	// ログ出力を削減（パフォーマンス改善） - 重要なイベントのみ残す
}

// GetNextPieceFromQueue は7-bagシステムからピースを取得します。
// デッキデータがある場合はスコア情報も設定します。
func (s *PlayerGameState) GetNextPieceFromQueue() *tetris.Piece {
	if len(s.pieceQueue) == 0 {
		s.generatePieceQueue() // 新しいキューを生成
	}

	// キューの先頭からピースタイプを取得
	pieceType := s.pieceQueue[0]
	s.pieceQueue = s.pieceQueue[1:] // 先頭を削除

	// ピースを作成
	piece := &tetris.Piece{
		Type:      pieceType,
		X:         tetris.BoardWidth/2 - 1, // ボード幅の中央付近
		Y:         0,                       // ボードの最上部
		Rotation:  0,                       // 初期回転状態
		ScoreData: make(map[string]int),
	}

	// デッキデータがある場合、該当するピースタイプのスコア情報を設定
	if len(s.DeckPlacements) > 0 {
		// 同じタイプのピースの中から最初に見つかったものを常に使用（一貫性のため）
		var selectedPiece *DeckPlacementPiece
		
		// 同じタイプのピースを探す（最初に見つかったもので固定）
		for _, deckPiece := range s.DeckPlacements {
			if deckPiece.Type == pieceType {
				selectedPiece = &deckPiece
				break // 最初に見つかったもので固定（使用回数に関係なく）
			}
		}
		
		if selectedPiece != nil {
			// デッキ配置データからスコア情報を設定 - すべての回転状態に対応

			
			// すべての回転状態（0, 90, 180, 270度）でのスコアマッピングを作成
			for rotation := 0; rotation < 4; rotation++ {
				rotationDegrees := rotation * 90
				blocks := piece.GetBlocksAtRotation(rotationDegrees)
				
				for blockIndex, block := range blocks {
					// デッキデータから対応するブロックのスコアを取得
					var score int
					if blockIndex < len(selectedPiece.Blocks) {
						score = selectedPiece.Blocks[blockIndex].Score
					} else {
						score = 100 // デフォルトスコア
					}
					
					// 複数のキー形式でスコアを設定（フロントエンドの互換性のため）
					relativeX := block[0]
					relativeY := block[1]
					
					// 1. フロントエンドが期待するメインキー（現在の回転状態での相対座標）
					if rotation == 0 { // 初期回転状態のキーをメインとして使用
						relativeKey := fmt.Sprintf("%d_%d", relativeX, relativeY)
						piece.ScoreData[relativeKey] = score

					}
					
					// 2. 回転状態別のキー（内部使用・デバッグ用）
					rotationKey := fmt.Sprintf("rot_%d_%d_%d", rotationDegrees, relativeX, relativeY)
					piece.ScoreData[rotationKey] = score
					
					// 3. ブロックインデックスキー（フォールバック用）
					if rotation == 0 { // 初期状態のみ
						blockIndexKey := strconv.Itoa(blockIndex)
						piece.ScoreData[blockIndexKey] = score
					}
				}
			}
			

		}
	}

	// デッキデータがない場合やマッチするピースタイプがない場合はデフォルトスコア
	if len(piece.ScoreData) == 0 {
		// デフォルトスコア（100）を全ブロックに設定
		blocks := piece.GetBlocksAtRotation(0)
		for blockIndex, block := range blocks {
			blockIndexKey := strconv.Itoa(blockIndex)
			piece.ScoreData[blockIndexKey] = 100 // デフォルトスコア
			
			// フロントエンドが期待する相対座標キー "X_Y" も設定
			relativeKey := fmt.Sprintf("%d_%d", block[0], block[1])
			piece.ScoreData[relativeKey] = 100
		}

	}

	return piece
}

// getPieceScoreFromDeck は指定されたピースタイプのデッキデータからスコア情報を取得します。
// 7-bagシステムで決定されたピースタイプに対応するデッキデータを探し、スコア情報を設定します。
//
// Parameters:
//   pieceType : 7-bagシステムで決定されたピースタイプ
// Returns:
//   *tetris.Piece: スコア情報が設定されたピース（デッキデータがない場合はnil）
func (s *PlayerGameState) getPieceScoreFromDeck(pieceType tetris.PieceType) *tetris.Piece {
	if len(s.DeckPlacements) == 0 {
		return nil // デッキデータがない
	}

	// 指定されたピースタイプのデッキデータを探す
	var selectedDeckPiece *DeckPlacementPiece
	for _, deckPiece := range s.DeckPlacements {
		if deckPiece.Type == pieceType {
			selectedDeckPiece = &deckPiece
			break
		}
	}

	// 指定されたピースタイプのデッキデータが見つからない場合
	if selectedDeckPiece == nil {
		return nil
	}

	// テトリスピースを作成
	piece := &tetris.Piece{
		Type:     pieceType, // 7-bagで決定されたピースタイプを使用
		ScoreData: make(map[string]int),
	}

	// すべての回転状態（0, 90, 180, 270度）に対してスコアマッピングを作成
	for rotation := 0; rotation < 4; rotation++ {
		rotationDegrees := rotation * 90 // 0, 90, 180, 270
		blocks := piece.GetBlocksAtRotation(rotationDegrees)
		
		for i, block := range blocks {
			// 回転状態別のキーを作成 "rot_rotation_x_y"
			key := "rot_" + strconv.Itoa(rotationDegrees) + "_" + strconv.Itoa(block[0]) + "_" + strconv.Itoa(block[1])
			
			// デッキデータの対応するブロックからスコアを取得
			var score int
			if i < len(selectedDeckPiece.Blocks) {
				score = selectedDeckPiece.Blocks[i].Score
			} else {
				score = 100 // デフォルトスコア
			}
			piece.ScoreData[key] = score
		}
	}

	return piece
}

// getNextPieceFromDeck はデッキ配置データからテトリミノピースを取得します。
// スコアデータも含めてピースを生成するよう修正します。
func (s *PlayerGameState) getNextPieceFromDeck() *tetris.Piece {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.DeckPlacements) == 0 {
		// デッキデータがない場合は従来の方法でランダムピースを返す
		return s.GetNextPieceFromQueue()
	}

	// ランダムにデッキからピースを選択
	randomIndex := s.randGenerator.Intn(len(s.DeckPlacements))
	selectedPiece := s.DeckPlacements[randomIndex]

	// ピースを作成
	piece := &tetris.Piece{
		Type:     selectedPiece.Type,
		X:        tetris.BoardWidth/2 - 1, // 中央付近に配置
		Y:        0,
		Rotation: 0, // 最初は0度回転
		ScoreData: make(map[string]int),
	}

	// デッキ配置データからスコア情報を設定
	for blockIndex, block := range selectedPiece.Blocks {
		// ブロックインデックスベースのキーを設定
		blockIndexKey := strconv.Itoa(blockIndex)
		piece.ScoreData[blockIndexKey] = block.Score
		
		// 回転状態0での相対位置キーも設定（将来の互換性のため）
		blocks := piece.GetBlocksAtRotation(0)
		if blockIndex < len(blocks) {
			relativeX := blocks[blockIndex][0]
			relativeY := blocks[blockIndex][1]
			rotationKey := fmt.Sprintf("rot_0_%d_%d", relativeX, relativeY)
			relativeKey := fmt.Sprintf("%d_%d", relativeX, relativeY) // 相対座標キー追加
			piece.ScoreData[rotationKey] = block.Score
			piece.ScoreData[relativeKey] = block.Score
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
	scoreKey := strconv.Itoa(boardY) + "_" + strconv.Itoa(boardX)
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
	x, y := spawnPieceAtCenter(s.CurrentPiece.Type)
	s.CurrentPiece.X = x
	s.CurrentPiece.Y = y
	s.CurrentPiece.Rotation = 0 // 必ず回転をリセット

	// ホールドフラグをリセット（新しいピースなのでホールド可能）
	s.hasUsedHold = false

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
	// プレイヤー1のデッキが存在しない場合（ゲストユーザー）はデフォルトデッキを生成
	if player1Deck == nil {
		guestDeckData := models.CreateGuestDeck(player1ID)
		player1Deck = guestDeckData.Deck
		
		// デッキ配置データを使用してゲーム状態を作成
		player1State, err := createPlayerStateWithGuestDeck(player1ID, guestDeckData)
		if err != nil {
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
	
	// プレイヤー1のゲーム状態を作成（デッキデータを使用）
	player1State, err := NewPlayerGameStateWithDeckPlacements(player1ID, player1Deck, deckRepo)
	if err != nil {
		// エラーが発生した場合は従来の方法でフォールバック
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
	// プレイヤー2のデッキが存在しない場合（ゲストユーザー）はデフォルトデッキを生成
	if player2Deck == nil {
		guestDeckData := models.CreateGuestDeck(player2ID)
		player2Deck = guestDeckData.Deck
		
		// デッキ配置データを使用してゲーム状態を作成
		player2State, err := createPlayerStateWithGuestDeck(player2ID, guestDeckData)
		if err != nil {
			player2State = NewPlayerGameState(player2ID, player2Deck)
		}
		gs.Player2 = player2State
		return
	}
	
	// プレイヤー2のゲーム状態を作成（デッキデータを使用）
	player2State, err := NewPlayerGameStateWithDeckPlacements(player2ID, player2Deck, deckRepo)
	if err != nil {
		// エラーが発生した場合は従来の方法でフォールバック
		player2State = NewPlayerGameState(player2ID, player2Deck)
	}
	gs.Player2 = player2State
}

// createPlayerStateWithGuestDeck はゲスト用デッキデータからプレイヤー状態を作成します
func createPlayerStateWithGuestDeck(userID string, guestDeckData *models.DeckWithPlacements) (*PlayerGameState, error) {
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
		Deck:          guestDeckData.Deck,
		randGenerator: r,
		lastFallTime:  time.Now(),
		ContributionScores: make(map[string]int),
		CurrentPieceScores: make(map[string]int),
		DeckPlacements: []DeckPlacementPiece{},
		pieceUsageCount: make(map[tetris.PieceType]int),
	}

	// ゲスト用デッキ配置データをDeckPlacementPieceに変換
	for _, placement := range guestDeckData.Placements {
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

	// ゲストデッキデータから実際のスコアマップを構築
	state.buildContributionScoresFromDeck()

	// デッキデータがない場合やエラーの場合はランダムスコアを設定
	if len(state.ContributionScores) == 0 {
		for y := 0; y < tetris.BoardHeight; y++ {
			for x := 0; x < tetris.BoardWidth; x++ {
				state.ContributionScores[strconv.Itoa(y) + "_" + strconv.Itoa(x)] = r.Intn(21) + 5 // 5-25点のランダムスコア
			}
		}
	}

	state.generatePieceQueue() // 最初のピースキューを生成
	state.SpawnNewPiece()      // 最初のピースを生成

	return state, nil
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
	// 残り時間を計算
	remainingTime := 0
	if gs.Status == "playing" && !gs.StartedAt.IsZero() {
		elapsed := time.Since(gs.StartedAt)
		remaining := gs.TimeLimit - elapsed
		if remaining > 0 {
			remainingTime = int(remaining.Seconds())
		}
	}

	lightweight := &LightweightGameState{
		ID:            gs.ID,
		Status:        gs.Status,
		StartedAt:     gs.StartedAt,
		EndedAt:       gs.EndedAt,
		TimeLimit:     int(gs.TimeLimit.Seconds()),
		RemainingTime: remainingTime,
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
// ピースの各ブロックが個別のスコアを持つように修正します。
func (s *PlayerGameState) updateCurrentPieceScores() {
	// 現在のピースが存在しない場合は何もしない（早期リターン）
	if s.CurrentPiece == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// マップ全削除の代わりに、新しいマップを作成（高速化）
	newScores := make(map[string]int, 4) // テトリミノは最大4ブロック

	// 現在のピースの各ブロックについて、ブロックインデックスベースでスコア情報を設定
	blocks := s.CurrentPiece.Blocks() // 一度だけ取得
	for blockIndex, block := range blocks {
		boardX := s.CurrentPiece.X + block[0]
		boardY := s.CurrentPiece.Y + block[1]

		// ボードの有効な範囲内のみ処理
		if boardX >= 0 && boardX < tetris.BoardWidth && boardY >= 0 && boardY < tetris.BoardHeight {
			// キーを設定
			blockIndexKey := strconv.Itoa(blockIndex)
			boardPosKey := strconv.Itoa(boardY) + "_" + strconv.Itoa(boardX)
			
			// スコアを取得（ブロックインデックスベースを優先）
			score := 100 // デフォルトスコア
			found := false
			
			// 1. ピースのScoreDataからブロックインデックスベースで取得（最も確実）
			if len(s.CurrentPiece.ScoreData) > 0 {
				if pieceScore, exists := s.CurrentPiece.ScoreData[blockIndexKey]; exists {
					score = pieceScore
					found = true
				}
			}
			
			// 2. フォールバック: 回転状態別キーで取得
			if !found && len(s.CurrentPiece.ScoreData) > 0 {
				relativeX := block[0]
				relativeY := block[1]
				rotationKey := "rot_" + strconv.Itoa(s.CurrentPiece.Rotation) + "_" + strconv.Itoa(relativeX) + "_" + strconv.Itoa(relativeY)
				
				if pieceScore, exists := s.CurrentPiece.ScoreData[rotationKey]; exists {
					score = pieceScore
					found = true
				}
			}
			
			// 3. フォールバック: デッキからのスコア情報を使用（古い実装との互換性）
			if !found && len(s.DeckPlacements) > 0 {
				for _, deckPiece := range s.DeckPlacements {
					if deckPiece.Type == s.CurrentPiece.Type {
						if blockIndex < len(deckPiece.Blocks) {
							score = deckPiece.Blocks[blockIndex].Score
							found = true
						}
						break
					}
				}
			}
			
			if !found {
			}
			
			// ブロックインデックスベースのキーを優先的に設定
			newScores[blockIndexKey] = score
			// ボード座標ベースのキーも設定（フォールバック用）
			newScores[boardPosKey] = score
		}
	}
	
	// 一括置換（アトミック操作）
	s.CurrentPieceScores = newScores
}
