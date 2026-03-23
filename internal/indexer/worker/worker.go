package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/KingrogKDR/Dev-Search/internal/indexer"
	"github.com/KingrogKDR/Dev-Search/internal/streams"
	"github.com/google/uuid"
)

const (
	TaskTimeout = 15 * time.Minute
	StreamName  = "parser-events"
	GroupName   = "indexer-group"
)

type ExecFunc func(ctx context.Context, msg *streams.Msg) error

type Worker struct {
	ID       string
	stream   *streams.MsgStream
	execFunc ExecFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	concurrency int
	timeout     time.Duration
}

func NewWorker(workerName string, stream *streams.MsgStream, concurrency int, exec ExecFunc) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	workerID := fmt.Sprintf("%s-%s", workerName, uuid.New().String())
	return &Worker{
		ID:          workerID,
		stream:      stream,
		execFunc:    exec,
		ctx:         ctx,
		cancel:      cancel,
		concurrency: concurrency,
		timeout:     TaskTimeout,
	}
}

func (w *Worker) Start() {
	log.Printf("Worker %s starting with %d concurrent processors", w.ID, w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.processMessages()
	}

	log.Printf("Worker %s started successfully", w.ID)

}

func (w *Worker) Stop() {
	log.Printf("Worker %s stopping...", w.ID)
	w.cancel()
	w.wg.Wait()
	log.Printf("Worker %s stopped", w.ID)
}

func (w *Worker) processMessages() {
	defer w.wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		messages, err := w.stream.GetMsg(w.ID)
		if err != nil {
			log.Printf("Worker %s: Error getting message from stream: %v", w.ID, err)
			time.Sleep(time.Second)
			continue
		}

		for _, message := range messages {
			w.processMessage(&message)
		}
	}
}

func (w *Worker) processMessage(message *streams.Msg) {
	start := time.Now()
	message.Status = streams.PROCESSING
	log.Printf("Worker %s: Processing message %s (attempt: %d:%d)", w.ID, message.StreamID, message.RetryCount+1, streams.MAX_RETRIES+1)

	ctx, cancel := context.WithTimeout(w.ctx, w.timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- w.execFunc(ctx, message)
	}()

	var err error
	select {
	case err = <-done: // job completed
	case <-ctx.Done(): // job timed out
		err = fmt.Errorf("Msg processing timed out after %v", w.timeout)
	}

	duration := time.Since(start)
	indexer.AddIndexLatency(duration)

	success := err == nil

	var errMsg string

	if err != nil {
		errMsg = err.Error()
	}

	w.completeMsgProcessing(message, success, errMsg, duration)
	indexer.IncrementProcessed()

}

func (w *Worker) completeMsgProcessing(msg *streams.Msg, success bool, errMsg string, duration time.Duration) {
	if success {
		indexer.IncrementSuccess()
		log.Printf("Worker %s: Finished processing message %s in %v success=%v", w.ID, msg.ID, duration, success)
	} else {
		indexer.IncrementFailure()
		if msg.RetryCount > 0 {
			indexer.IncrementRetry()
		}

		log.Printf("Worker %s: Processing msg %s failed: %s (attempt %d/%d)",
			w.ID, msg.ID, errMsg, msg.RetryCount+1, streams.MAX_RETRIES+1)
	}

	queueLag := time.Since(msg.AddedAt)
	log.Printf("Worker %s: Queue lag for msg %s: %v", w.ID, msg.ID, queueLag)

	if err := w.stream.CompleteMessage(msg, success, StreamName, GroupName); err != nil {
		log.Printf("Worker %s: Error completing message processing %s: %v", w.ID, msg.ID, err)
	}
}
