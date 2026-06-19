package tetris

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisGameMessage はRedis Pub/Subで送受信するメッセージの型です。
type RedisGameMessage struct {
	Type       string                  `json:"type"` // player_joined, player_ready, player_state, game_start, game_over
	Passcode   string                  `json:"passcode"`
	PlayerSlot string                  `json:"player_slot,omitempty"` // "player1" or "player2"
	UserID     string                  `json:"user_id,omitempty"`
	State      *LightweightPlayerState `json:"state,omitempty"`
}

func redisRoomKey(passcode string) string     { return "room:" + passcode }
func redisGameChannel(passcode string) string { return "game:" + passcode }

// CreateRoomInRedis はRedisにルーム情報を保存し、Pub/Sub購読を開始します。
func (sm *SessionManager) CreateRoomInRedis(passcode, playerID string) error {
	if sm.redisClient == nil {
		return nil
	}
	ctx := context.Background()
	if err := sm.redisClient.HSet(ctx, redisRoomKey(passcode),
		"player1_id", playerID,
		"player2_id", "",
		"status", "waiting",
	).Err(); err != nil {
		return err
	}
	sm.redisClient.Expire(ctx, redisRoomKey(passcode), 24*time.Hour)
	// ホストのPodがplayer_joinedなどを受け取れるよう、ルーム作成時に購読を開始する
	sm.subscribeToRoom(passcode)
	return nil
}

// getRoomFromRedis はRedisからルーム情報を取得します。
func (sm *SessionManager) getRoomFromRedis(passcode string) (map[string]string, error) {
	if sm.redisClient == nil {
		return nil, nil
	}
	ctx := context.Background()
	return sm.redisClient.HGetAll(ctx, redisRoomKey(passcode)).Result()
}

// joinRoomInRedis はRedisのルームにPlayer2として参加します。
func (sm *SessionManager) joinRoomInRedis(passcode, playerID string) error {
	if sm.redisClient == nil {
		return nil
	}
	ctx := context.Background()
	return sm.redisClient.HSet(ctx, redisRoomKey(passcode), "player2_id", playerID).Err()
}

// subscribeToRoom はRedisのPub/Subチャンネルを購読します（重複防止付き）。
func (sm *SessionManager) subscribeToRoom(passcode string) {
	if sm.redisClient == nil {
		return
	}

	sm.pubsubMu.Lock()
	if _, exists := sm.pubsubCancels[passcode]; exists {
		sm.pubsubMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	sm.pubsubCancels[passcode] = cancel
	sm.pubsubMu.Unlock()

	go func() {
		defer func() {
			sm.pubsubMu.Lock()
			delete(sm.pubsubCancels, passcode)
			sm.pubsubMu.Unlock()
		}()

		pubsub := sm.redisClient.Subscribe(ctx, redisGameChannel(passcode))
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var gameMsg RedisGameMessage
				if err := json.Unmarshal([]byte(msg.Payload), &gameMsg); err != nil {
					log.Printf("[Redis] parse error: %v", err)
					continue
				}
				sm.handleRedisMessage(gameMsg)
			}
		}
	}()
}

// publishMessage はRedis Pub/Subチャンネルにメッセージを発行します。
func (sm *SessionManager) publishMessage(passcode string, msg RedisGameMessage) {
	if sm.redisClient == nil {
		return
	}
	msg.Passcode = passcode
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	sm.redisClient.Publish(context.Background(), redisGameChannel(passcode), string(data))
}

// handleRedisMessage はRedisから受信したメッセージを処理します。
func (sm *SessionManager) handleRedisMessage(msg RedisGameMessage) {
	switch msg.Type {

	case "player_joined":
		// 他PodでPlayerが参加 → ローカルセッションにスタブとして追加
		sm.mu.Lock()
		session, ok := sm.sessions[msg.Passcode]
		var shouldRespond bool
		var localSlot, localUserID string
		if ok {
			slotWasEmpty := false
			if msg.PlayerSlot == "player2" && session.Player2 == nil {
				session.Player2 = &PlayerGameState{UserID: msg.UserID, IsStub: true}
				slotWasEmpty = true
			} else if msg.PlayerSlot == "player1" && session.Player1 == nil {
				session.Player1 = &PlayerGameState{UserID: msg.UserID, IsStub: true}
				slotWasEmpty = true
			}
			// 相手が初めて参加してきた場合、自分の存在を返信（ハンドシェイク）
			if slotWasEmpty {
				if session.Player1 != nil && msg.PlayerSlot != "player1" {
					shouldRespond = true
					localSlot = "player1"
					localUserID = session.Player1.UserID
				} else if session.Player2 != nil && msg.PlayerSlot != "player2" {
					shouldRespond = true
					localSlot = "player2"
					localUserID = session.Player2.UserID
				}
			}
		}
		sm.mu.Unlock()
		if shouldRespond {
			go sm.publishMessage(msg.Passcode, RedisGameMessage{
				Type:       "player_joined",
				PlayerSlot: localSlot,
				UserID:     localUserID,
			})
		}
		go sm.BroadcastGameState(msg.Passcode)

	case "player_ready":
		// 他PodでWebSocket接続が確立 → readyフラグを立ててゲーム開始チェック
		key := msg.Passcode + ":" + msg.PlayerSlot
		sm.remoteReadyMu.Lock()
		sm.remoteReady[key] = true
		sm.remoteReadyMu.Unlock()
		go sm.CheckAndStartGame(msg.Passcode)

	case "player_state":
		// 他Podのゲーム状態更新
		if msg.State == nil {
			return
		}
		key := msg.Passcode + ":" + msg.PlayerSlot
		sm.remoteStatesMu.Lock()
		sm.remoteStates[key] = msg.State
		sm.remoteStatesMu.Unlock()
		go sm.broadcastWithRemoteState(msg.Passcode)

	case "game_start":
		// ゲーム開始シグナル（Player1がいるPodから発行される）
		sm.mu.Lock()
		session, ok := sm.sessions[msg.Passcode]
		if ok && session.Status == "waiting" {
			session.Status = "playing"
			session.StartedAt = time.Now()
		}
		sm.mu.Unlock()
		sm.resetBroadcastThrottle(msg.Passcode)
		go sm.BroadcastGameState(msg.Passcode)

	case "game_over":
		// ゲーム終了シグナル
		sm.mu.RLock()
		session, ok := sm.sessions[msg.Passcode]
		sm.mu.RUnlock()
		if ok && session.Status == "playing" {
			go sm.EndGameSession(msg.Passcode)
		}

	case "delete_session":
		// 他PodでセッションDELETE要求 → ローカルセッションも削除
		sm.mu.Lock()
		if _, ok := sm.sessions[msg.Passcode]; ok {
			delete(sm.sessions, msg.Passcode)
			log.Printf("[Redis] delete_session received, removed local session: %s", msg.Passcode)
		}
		sm.mu.Unlock()
		go sm.cancelRoomSubscription(msg.Passcode)
	}
}

// broadcastWithRemoteState はリモートプレイヤーの状態を補完してブロードキャストします。
func (sm *SessionManager) broadcastWithRemoteState(passcode string) {
	sm.mu.RLock()
	session, ok := sm.sessions[passcode]
	if !ok {
		sm.mu.RUnlock()
		return
	}
	lightweight := session.ToLightweight()

	sm.remoteStatesMu.RLock()
	if lightweight.Player1 == nil || (session.Player1 != nil && session.Player1.IsStub) {
		if remote, exists := sm.remoteStates[passcode+":player1"]; exists {
			lightweight.Player1 = remote
		}
	}
	if lightweight.Player2 == nil || (session.Player2 != nil && session.Player2.IsStub) {
		if remote, exists := sm.remoteStates[passcode+":player2"]; exists {
			lightweight.Player2 = remote
		}
	}
	sm.remoteStatesMu.RUnlock()

	stateJSON, err := json.Marshal(lightweight)
	if err != nil {
		sm.mu.RUnlock()
		return
	}
	for _, client := range sm.clients {
		if client.RoomID == passcode {
			client.SafeSend(stateJSON)
		}
	}
	sm.mu.RUnlock()
}

// publishLocalPlayerState はローカルプレイヤーの状態をRedisに発行します。
func (sm *SessionManager) publishLocalPlayerState(passcode string) {
	if sm.redisClient == nil {
		return
	}
	sm.mu.RLock()
	session, ok := sm.sessions[passcode]
	if !ok {
		sm.mu.RUnlock()
		return
	}
	lightweight := session.ToLightweight()
	player1IsStub := session.Player1 != nil && session.Player1.IsStub
	player2IsStub := session.Player2 != nil && session.Player2.IsStub
	sm.mu.RUnlock()

	// スタブ（他Podのプレイヤー）の状態は発行しない：空データで上書きしてしまうため
	if lightweight.Player1 != nil && !player1IsStub {
		sm.publishMessage(passcode, RedisGameMessage{
			Type:       "player_state",
			PlayerSlot: "player1",
			State:      lightweight.Player1,
		})
	}
	if lightweight.Player2 != nil && !player2IsStub {
		sm.publishMessage(passcode, RedisGameMessage{
			Type:       "player_state",
			PlayerSlot: "player2",
			State:      lightweight.Player2,
		})
	}
}

// DeleteSessionViaPubSub は他ポッドにセッション削除を通知します。
// ローカルにセッションがない場合でも他ポッドの部分セッションを削除できます。
// Redisが有効な場合はtrue、無効な場合はfalseを返します。
func (sm *SessionManager) DeleteSessionViaPubSub(passcode string) bool {
	if sm.redisClient == nil {
		return false
	}
	sm.redisClient.Del(context.Background(), redisRoomKey(passcode))
	go sm.publishMessage(passcode, RedisGameMessage{
		Type: "delete_session",
	})
	return true
}

// cancelRoomSubscription はルームのPub/Sub購読をキャンセルします。
func (sm *SessionManager) cancelRoomSubscription(passcode string) {
	sm.pubsubMu.Lock()
	if cancel, exists := sm.pubsubCancels[passcode]; exists {
		cancel()
		delete(sm.pubsubCancels, passcode)
	}
	sm.pubsubMu.Unlock()
}

// getLocalPlayerSlot はローカルセッション内でuserIDが占めるスロットを返します。
func (sm *SessionManager) getLocalPlayerSlot(passcode, userID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[passcode]
	if !ok {
		return ""
	}
	if session.Player1 != nil && session.Player1.UserID == userID {
		return "player1"
	}
	if session.Player2 != nil && session.Player2.UserID == userID {
		return "player2"
	}
	return ""
}

// NewRedisClient はRedisクライアントを初期化して接続確認します。
func NewRedisClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
