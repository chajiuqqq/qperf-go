package rl

import (
	"context"
	"fmt"
	"github.com/apernet/quic-go/congestion"
	"math/rand"
	"testing"
	"time"
)

func TestQuicMqManager_PublishState(t *testing.T) {

	r, err := NewRedisManager("localhost", "6379", "")
	if err != nil {
		fmt.Println("Failed to create Redis manager:", err)
		return
	}

	qm := NewQuicMqManager(r, "test1234")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// rl redis part
	go func() {
		sub := r.Subscribe(qm.pubChannel)
		ch := sub.Channel()
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				fmt.Println("listen", msg.Payload)
			}
		}
	}()

	// go qm.ListenAction(ctx)

	for {
		select {
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		case <-time.Tick(time.Second):
			err = qm.PublishState(&StateMsg{
				Cwnd: congestion.ByteCount(rand.Intn(1000)),
				Rtt:  int64(rand.Intn(100)),
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}

}

func TestQuicMqManager_GetActionCh(t *testing.T) {

	r, err := NewRedisManager("localhost", "6379", "")
	if err != nil {
		fmt.Println("Failed to create Redis manager:", err)
		return
	}

	qm := NewQuicMqManager(r, "test1234")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// rl redis part
	// go func() {
	// 	sub := r.Subscribe(qm.pubChannel)
	// 	ch := sub.Channel()
	// 	defer sub.Close()
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case msg := <-ch:
	// 			fmt.Println("rl: listen state", msg.Payload)
	// 			// read state
	// 			state := StateMsg{}
	// 			err = json.Unmarshal([]byte(msg.Payload), &state)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				continue
	// 			}
	//
	// 			// gen action
	// 			action := ActionMsg{
	// 				Seq:    state.Seq,
	// 				Action: rand.Intn(5),
	// 			}
	// 			actionMsg, err := json.Marshal(action)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				continue
	// 			}
	// 			time.Sleep(100 * time.Millisecond) // delay
	// 			// time.Sleep(500 * time.Millisecond) // delay
	//
	// 			fmt.Println("rl: publish action", string(actionMsg))
	// 			// publish action
	// 			err = r.Publish(qm.subChannel, string(actionMsg))
	// 			if err != nil {
	// 				fmt.Println(err)
	// 			}
	// 		}
	// 	}
	// }()

	// quic listen action
	go qm.ListenAction(ctx)
	// quic publish state
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Println(ctx.Err())
				err = qm.PublishState(&StateMsg{
					FIN: true,
				})
				if err != nil {
					fmt.Println(err)
				}
				return
			case <-time.Tick(time.Second):
				msg := StateMsg{
					Cwnd: congestion.ByteCount(rand.Intn(1000)),
					Rtt:  int64(rand.Intn(100)),
				}
				err = qm.PublishState(&msg)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println("quic: publish state", msg)
			}
		}
	}()

	// quic apply action
	cwnd := 1
	actionCh := qm.GetActionCh()
	actionMap := map[int]int{
		0: -3,
		1: -1,
		2: 0,
		3: 1,
		4: 3,
	}
	for action := range actionCh {
		fmt.Println("quic: apply action", action)
		cwnd += actionMap[action.Action]
		fmt.Println("quic: new cwnd", cwnd)
	}

}
