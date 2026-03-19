package streams

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type MsgStream struct {
	namespace string
	client    *redis.Client
	ctx       context.Context
}

func NewMsgStream(client *redis.Client, namespace string) *MsgStream {
	return &MsgStream{
		namespace: namespace,
		client:    client,
		ctx:       context.Background(),
	}
}

func (ms *MsgStream) AddMsg(msg *Msg) error {

	return nil
}

func (ms *MsgStream) GetMsg() (*Msg, error) {

	return nil, nil
}
