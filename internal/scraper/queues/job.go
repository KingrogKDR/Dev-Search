package queues

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PriorityStatus string
type JobStatus string
type JobType string

const (
	P0_CRITICAL PriorityStatus = "critical"
	P1_HIGH     PriorityStatus = "high"
	P2_NORMAL   PriorityStatus = "normal"
	P3_LOW      PriorityStatus = "low"
)

const (
	JOB_READY    JobStatus = "ready"
	JOB_INFLIGHT JobStatus = "inflight"
	JOB_DONE     JobStatus = "done"
	JOB_DEAD     JobStatus = "dead"
)
const (
	JOB_CRAWL JobType = "crawl"
	JOB_PARSE JobType = "parse"
)

const MAX_RETRIES = 5
const DEFAULT_RETRY_DELAY = 10 * time.Second

type Job struct {
	ID              string          `json:"id"`
	URL             string          `json:"url"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload"`
	Status          JobStatus       `json:"status"`
	Priority        PriorityStatus  `json:"priority"`
	RetryCount      int             `json:"retry_count"`
	BaseScore       int             `json:"base_score"`
	VisibilityStart time.Time       `json:"visibility_start"`
	CreatedAt       time.Time       `json:"created_at"`
	LastEnqueuedAt  time.Time       `json:"last_enqueued_at"`
	ErrorMsg        string          `json:"err_msg"`
}

func NewJob(rawUrl string) *Job {
	return &Job{
		ID:         uuid.NewString(),
		URL:        rawUrl,
		Status:     JOB_READY,
		RetryCount: 0,
		BaseScore:  10,
		CreatedAt:  time.Now(),
	}
}

type Result struct {
	JobID      string        `json:"job_id"`
	Success    bool          `json:"success"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
	WorkerID   string        `json:"worker_id"`
	FinishedAt time.Time     `json:"finished_at"`
}
