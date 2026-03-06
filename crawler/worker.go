package crawler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/google/uuid"
)

const (
	UserAgent   = "Dev_Search/1.0"
	TaskTimeout = 30 * time.Second
)

type Worker struct {
	ID       string
	frontier *queues.Queue
	queues   []string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	concurrency int
	timeout     time.Duration
}

func NewWorker(frontier *queues.Queue, queues []string, concurrency int) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	workerID := fmt.Sprintf("%s-%s", UserAgent, uuid.New().String())
	return &Worker{
		ID:          workerID,
		frontier:    frontier,
		queues:      queues,
		ctx:         ctx,
		cancel:      cancel,
		concurrency: concurrency,
		timeout:     TaskTimeout,
	}
}

func FetchReq(ctx context.Context, rawUrl string) error {
	client := &http.Client{}

	request, err := http.NewRequestWithContext(ctx, "GET", rawUrl, nil)
	if err != nil {
		return fmt.Errorf("Error while creating request %s", rawUrl)
	}
	request.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Error fetching request for %s", rawUrl)
	}

	defer resp.Body.Close()

	return nil
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
			job, err := w.frontier.Dequeue(w.queues, w.ID, 5*time.Second)
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
		done <- FetchReq(ctx, job.URL)
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
	result := &queues.Result{
		JobID:      job.ID,
		Success:    success,
		Error:      errorMsg,
		Duration:   duration,
		WorkerID:   w.ID,
		FinishedAt: time.Now(),
	}

	if success {
		log.Printf("Worker %s: Job %s for %s completed successfully in %v", w.ID, job.ID, job.URL, duration)
	} else {
		log.Printf("Worker %s: Job %s for %s failed: %s (attempt %d/%d)",
			w.ID, job.ID, job.URL, errorMsg, job.RetryCount+1, queues.MAX_RETRIES+1)
	}

	if err := w.frontier.CompleteJob(job, result, w.ID); err != nil {
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

			w.frontier.Redis.HSet(w.ctx, key, heartbeat)
			w.frontier.Redis.Expire(w.ctx, key, 2*time.Minute)
		}
	}
}
