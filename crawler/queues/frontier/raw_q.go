package frontier

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type RawQueue struct {
	items     chan string
	closedCh  chan struct{}
	closeOnce sync.Once
	enqueued  atomic.Int64
	dequeued  atomic.Int64
	dropped   atomic.Int64
}

var ErrQueueClosed = errors.New("Queue is Closed")

func NewRawQueue(maxSize int) *RawQueue {
	return &RawQueue{
		items:    make(chan string, maxSize),
		closedCh: make(chan struct{}),
	}
}

func (q *RawQueue) Enqueue(ctx context.Context, rawUrl string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.closedCh:
		return ErrQueueClosed
	case q.items <- rawUrl:
		q.enqueued.Add(1)
		return nil
	}
}

func (q *RawQueue) Dequeue(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case item, ok := <-q.items:
		if ok {
			q.dequeued.Add(1)
		}
		return item, nil
	}
}

func (q *RawQueue) Close() {
	q.closeOnce.Do(func() {
		close(q.closedCh)
		close(q.items)
	})
}

func (q *RawQueue) AddUrl(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return q.Enqueue(ctx, url)
}

func (q *RawQueue) AddUrls(urls []string) (int, int) {
	var added = 0
	var failed = 0
	for _, url := range urls {
		if err := q.AddUrl(url); err != nil {
			log.Printf("Failed to add URL %s: %v\n", url, err)
			failed++
		} else {
			added++
		}
	}
	return added, failed
}
