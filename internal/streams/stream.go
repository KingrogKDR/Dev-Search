package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	MAX_RETRIES = 5
)

type MsgStream struct {
	namespace  string
	streamName string
	groupName  string
	client     *redis.Client
	ctx        context.Context
}

func NewMsgStream(client *redis.Client, namespace string, consumerGroup string) *MsgStream {
	streamName := fmt.Sprintf("%s-events", namespace)
	groupName := fmt.Sprintf("%s-group", consumerGroup)

	ms := &MsgStream{
		namespace:  namespace,
		streamName: streamName,
		groupName:  groupName,
		client:     client,
		ctx:        context.Background(),
	}
	ms.ensureGroup(streamName, groupName)

	return ms
}

func (ms *MsgStream) ensureGroup(streamName, groupName string) {
	err := ms.client.XGroupCreateMkStream(ms.ctx, streamName, groupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		panic(fmt.Errorf("failed to create consumer group: %w", err))
	}
}

func (ms *MsgStream) AddMsg(msg *Msg) error {
	msg.AddedAt = time.Now()

	msgJson, err := json.Marshal(msg)

	if err != nil {
		return fmt.Errorf("Can't marshal message in stream: %w", err)
	}
	publishCtx := context.Background()
	err = ms.client.XAdd(publishCtx, &redis.XAddArgs{
		Stream: ms.streamName,
		Values: map[string]any{
			"data": msgJson,
		},
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to XADD: %w", err)
	}

	return nil
}

func (ms *MsgStream) GetMsg(consumer string) ([]Msg, error) {
	log.Printf("Reading from stream %s as consumer %s", ms.streamName, consumer)
	read := func(id string) ([]redis.XStream, error) {
		log.Printf("Calling XREADGROUP with '%s'", id)
		return ms.client.XReadGroup(ms.ctx, &redis.XReadGroupArgs{
			Group:    ms.groupName,
			Consumer: consumer,
			Streams:  []string{ms.streamName, id},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
	}

	var streams []redis.XStream

	// try pending msgs first

	streams, err := read(">")
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("Can't read from stream: %w", err)
	}

	// if not pending -> try new
	if len(streams) == 0 {
		streams, err = read("0")
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("Can't read from stream: %w", err)
		}
	}

	// if still empty -> try autoclaim
	if len(streams) == 0 {
		res, _, err := ms.client.XAutoClaim(ms.ctx, &redis.XAutoClaimArgs{
			Stream:   ms.streamName,
			Group:    ms.groupName,
			Consumer: consumer,
			MinIdle:  time.Minute,
			Start:    "0",
			Count:    10,
		}).Result()

		if err != nil && err != redis.Nil {
			return nil, err
		}

		// convert claimed messages into XStream-like structure
		if len(res) > 0 {
			streams = []redis.XStream{
				{
					Stream:   ms.streamName,
					Messages: res,
				},
			}
		}
	}

	log.Printf("Fetched %d streams", len(streams))
	var result []Msg

	for _, stream := range streams {
		for _, message := range stream.Messages {
			raw, ok := message.Values["data"]
			if !ok {
				continue
			}

			var bytes []byte
			switch v := raw.(type) {
			case string:
				bytes = []byte(v)
			case []byte:
				bytes = v
			default:
				continue
			}

			var msg Msg
			if err := json.Unmarshal(bytes, &msg); err != nil {
				continue
			}
			msg.StreamID = message.ID
			msg.LastFetchedAt = time.Now()
			result = append(result, msg)

		}
	}

	log.Printf("Fetched %d messages", len(result))

	return result, nil
}

func (ms *MsgStream) CompleteMessage(msg *Msg, success bool, streamName string, groupName string) error {
	if err := ms.client.XAck(ms.ctx, streamName, groupName, msg.StreamID).Err(); err != nil {
		return fmt.Errorf("failed to XACK message %s: %w", msg.StreamID, err)
	}
	if !success {
		msg.RetryCount++
		time.Sleep(time.Duration(msg.RetryCount) * time.Second)

		if msg.RetryCount <= MAX_RETRIES {
			data, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal retry msg: %w", err)
			}

			err = ms.client.XAdd(ms.ctx, &redis.XAddArgs{
				Stream: streamName,
				Values: map[string]interface{}{
					"data": data,
				},
			}).Err()
			if err != nil {
				return fmt.Errorf("failed to requeue message: %w", err)
			}

			return nil
		}

		msg.Status = DEAD
		log.Printf("Message %s is dead!", msg.ID)
		return nil
	}

	msg.Status = DONE
	log.Printf("Message %s successfully processed!", msg.ID)

	return nil

}
