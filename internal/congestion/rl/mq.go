package rl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"math/rand"
)

type StateMsg struct {
	Seq  int  `json:"seq"`
	Cwnd int  `json:"cwnd"`
	Rtt  int  `json:"rtt"`
	FIN  bool `json:"FIN,omitempty"`
}
type ActionMsg struct {
	Seq    int `json:"seq"`
	Action int `json:"action"`
}
type QuicMqManager struct {
	redis      *RedisManager
	pubChannel string
	subChannel string
	actionCh   chan *ActionMsg
	seq        int
}

func NewQuicMqManager(redis *RedisManager, connectionID string) *QuicMqManager {
	q := &QuicMqManager{
		redis:      redis,
		pubChannel: fmt.Sprintf("/%s/state", connectionID),
		subChannel: fmt.Sprintf("/%s/action", connectionID),
		actionCh:   make(chan *ActionMsg, 1),
		seq:        rand.Intn(100000),
	}
	return q
}

// run in go routine
func (q *QuicMqManager) ListenAction(ctx context.Context) {
	sub := q.redis.Subscribe(q.subChannel)
	ch := sub.Channel()
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			close(q.actionCh)
			return
		case msg := <-ch:
			actionMsg, err := q.readAction(msg)
			if err != nil {
				fmt.Println(err)
				continue
			}
			q.sendAction(actionMsg)
			fmt.Println("quic: save action", actionMsg)
		}
	}

}
func (q *QuicMqManager) sendAction(a *ActionMsg) {
	if a.Seq < q.seq {
		fmt.Printf("action seq %d smaller than state seq %d ,ignore action\n", a.Seq, q.seq)
		return
	} else {
		q.actionCh <- a
	}
}
func (q *QuicMqManager) nextSeq() int {
	q.seq += 1
	return q.seq
}
func (q *QuicMqManager) PublishState(msg StateMsg) error {
	msg.Seq = q.nextSeq()
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	err = q.redis.Publish(q.pubChannel, string(body))
	if err != nil {
		return err
	}
	return nil
}
func (q *QuicMqManager) GetActionCh() chan *ActionMsg {
	return q.actionCh
}

// change to QuicMqManager's actionMsg
func (q *QuicMqManager) readAction(m *redis.Message) (*ActionMsg, error) {
	msg := ActionMsg{}
	err := json.Unmarshal([]byte(m.Payload), &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

type RedisManager struct {
	client *redis.Client
}

func NewRedisManager(host, port, password string) (*RedisManager, error) {
	redisAddr := fmt.Sprintf("%s:%s", host, port)
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: password,
		DB:       0,
	})

	_, err := client.Ping(client.Context()).Result()
	if err != nil {
		return nil, err
	}

	return &RedisManager{
		client: client,
	}, nil
}

func (r *RedisManager) Publish(channel, message string) error {
	err := r.client.Publish(r.client.Context(), channel, message).Err()
	if err != nil {
		return err
	}

	return nil
}

func (r *RedisManager) Subscribe(channels ...string) *redis.PubSub {
	pubsub := r.client.Subscribe(r.client.Context(), channels...)
	return pubsub
}

// func main() {
// 	manager, err := NewRedisManager("localhost", "6379", "")
// 	if err != nil {
// 		fmt.Println("Failed to create Redis manager:", err)
// 		return
// 	}
//
// 	channel := "my-channel"
// 	message := "Hello, Redis!"
//
// 	err = manager.Publish(channel, message)
// 	if err != nil {
// 		fmt.Println("Failed to publish message:", err)
// 		return
// 	}
//
// 	pubsub := manager.Subscribe(channel)
// 	defer pubsub.Close()
//
// 	ch := pubsub.Channel()
// 	for msg := range ch {
// 		fmt.Println(msg.Channel, msg.Payload)
// 	}
// }
