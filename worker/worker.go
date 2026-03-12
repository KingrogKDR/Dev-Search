package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/KingrogKDR/Dev-Search/internal/stats"
	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/google/uuid"
)

const (
	TaskTimeout = 30 * time.Second
)

type ExecFunc func(ctx context.Context, job *queues.Job) error

type Worker struct {
	ID       string
	queue    *queues.Queue
	execFunc ExecFunc

	queues []string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	concurrency int
	timeout     time.Duration
}

func NewWorker(workerName string, queue *queues.Queue, queues []string, concurrency int, exec ExecFunc) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	workerID := fmt.Sprintf("%s-%s", workerName, uuid.New().String())
	return &Worker{
		ID:          workerID,
		queue:       queue,
		execFunc:    exec,
		queues:      queues,
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
		go w.processTasks()
	}

	w.wg.Add(1)
	go w.healthCheck()

	log.Printf("Worker %s started successfully", w.ID)

}

func (w *Worker) Stop() {
	log.Printf("Worker %s stopping...", w.ID)
	w.cancel()  // cancels context
	w.wg.Wait() // waits for all goroutines/ tasks to complete
	log.Printf("Worker %s stopped", w.ID)
}

func (w *Worker) processTasks() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			job, err := w.queue.Dequeue(w.queues, w.ID, 5*time.Second)
			if err != nil {
				log.Printf("Worker %s: Error dequeuing job: %v", w.ID, err)
				time.Sleep(time.Second)
				continue

			}

			if job == nil {
				continue
			}

			w.processTask(job)
		}
	}

}

func (w *Worker) processTask(job *queues.Job) {
	start := time.Now()

	log.Printf("Worker %s: Processing job %s (url: %s, attempt: %d/%d)",
		w.ID, job.ID, job.URL, job.RetryCount+1, queues.MAX_RETRIES+1)

	ctx, cancel := context.WithTimeout(w.ctx, w.timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- w.execFunc(ctx, job)
	}()

	var err error
	select {
	case err = <-done: // job completed
	case <-ctx.Done(): // job timed out
		err = fmt.Errorf("Job processing timed out after %v", w.timeout)
	}

	duration := time.Since(start)

	success := err == nil

	var errorMsg string
	if err != nil {
		errorMsg = err.Error()
	}
	w.completeTask(job, success, errorMsg, duration)
}

func (w *Worker) completeTask(job *queues.Job, success bool, errorMsg string, duration time.Duration) {
	stats.Increment()
	result := &queues.Result{
		JobID:      job.ID,
		Success:    success,
		Error:      errorMsg,
		Duration:   duration,
		WorkerID:   w.ID,
		FinishedAt: time.Now(),
	}

	if success {
		log.Printf("Worker %s: Finished job %s in %v success=%v", w.ID, job.URL, duration, success)
	} else {
		log.Printf("Worker %s: Job %s for %s failed: %s (attempt %d/%d)",
			w.ID, job.ID, job.URL, errorMsg, job.RetryCount+1, queues.MAX_RETRIES+1)
	}

	if err := w.queue.CompleteJob(job, result, w.ID); err != nil {
		log.Printf("Worker %s: Error completing task %s: %v", w.ID, job.ID, err)
	}
}

func (w *Worker) healthCheck() {
	defer w.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			key := fmt.Sprintf("taskqueue:workers:%s", w.ID)
			heartbeat := map[string]interface{}{
				"id":        w.ID,
				"queues":    w.queues,
				"last_seen": time.Now().Unix(),
				"status":    "active",
			}

			w.queue.Redis.HSet(w.ctx, key, heartbeat)
			w.queue.Redis.Expire(w.ctx, key, 2*time.Minute)
		}
	}
}
