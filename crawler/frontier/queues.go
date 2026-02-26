package frontier

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const VisibilityTimeout = 30 * time.Second
const MaxRetries = 5
const BaseBackoff = 5 * time.Second

type JobStatus uint8

const (
	JobReady JobStatus = iota
	JobInFlight
	JobDone
	JobDead
)

type PriorityStatus uint8

const (
	P0_IMP PriorityStatus = iota
	P1_HIGH
	P2_NORMAL
	P3_LOW
	NumPriorities
)

var queueSizes = map[uint8]uint64{
	uint8(P0_IMP):    256,  // rare, urgent
	uint8(P1_HIGH):   1024, // discovery
	uint8(P2_NORMAL): 4096, // main workload
	uint8(P3_LOW):    8192, // recrawls / background
}

var maxDeadJobs = 1024
var delayedStoreSize = 1024

type Job struct {
	ID              string
	Priority        PriorityStatus
	Payload         string
	Status          JobStatus
	VisibilityUntil time.Time
	RetryCount      uint64
}

type queueCell struct {
	seqNo uint64
	value *Job
}

type DataQ struct {
	mask   uint64
	buffer []queueCell
	head   uint64
	tail   uint64
}

type PriorityQ struct {
	queues      []*DataQ
	deadLetterQ []*deadJob
	delayed     *DelayedStore
}

type deadJob struct {
	job *Job
	err *error
}

type DelayedStore struct {
	jobs []*Job
	mu   sync.Mutex
}

func NewDataQ(size uint64) *DataQ {
	if size&(size-1) != 0 {
		panic("Size must be of power 2")
	}

	q := &DataQ{
		mask:   size - 1,
		buffer: make([]queueCell, size),
	}

	for i := range size {
		q.buffer[i].seqNo = i
	}

	return q
}

func NewDelayedStore() *DelayedStore {
	return &DelayedStore{
		jobs: make([]*Job, 0, delayedStoreSize),
	}
}

func NewPriorityQ() *PriorityQ {
	dataQueues := make([]*DataQ, NumPriorities)
	for i := range NumPriorities {
		size := queueSizes[uint8(i)]
		dataQueues[i] = NewDataQ(size)
	}
	deadQ := make([]*deadJob, maxDeadJobs)
	pq := &PriorityQ{
		queues:      dataQueues,
		deadLetterQ: deadQ,
		delayed:     NewDelayedStore(),
	}

	pq.delayed.Start(pq.Enqueue)

	return pq
}

func (q *DataQ) enqueue(job *Job) bool {
	for {
		tail := atomic.LoadUint64(&q.tail)
		cell := &q.buffer[tail&q.mask]
		seqno := atomic.LoadUint64(&cell.seqNo)
		diff := int64(seqno) - int64(tail)

		if diff == 0 {
			if atomic.CompareAndSwapUint64(&q.tail, tail, tail+1) {
				cell.value = job
				atomic.StoreUint64(&cell.seqNo, tail+1) // available to consume
				return true
			}
		} else if diff < 0 {
			return false // slot is full so cannot write to it
		}
	}
}

func (q *DataQ) dequeue() (*Job, bool) {
	for {
		head := atomic.LoadUint64(&q.head)
		cell := &q.buffer[head&q.mask]
		seqno := atomic.LoadUint64(&cell.seqNo)
		diff := int64(seqno) - int64(head+1)

		if diff == 0 {
			if atomic.CompareAndSwapUint64(&q.head, head, head+1) {
				job := cell.value
				atomic.StoreUint64(&cell.seqNo, head+q.mask+1) // slot is available for reuse after reading
				return job, true
			}
		} else if diff < 0 {
			return nil, false // slot is empty so cannot read from it
		}
	}
}

func (ds *DelayedStore) Add(job *Job) {
	ds.mu.Lock()
	ds.jobs = append(ds.jobs, job)
	ds.mu.Unlock()
}

func (ds *DelayedStore) Start(reEnqueue func(*Job) bool) {
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for range ticker.C {
			now := time.Now()

			ds.mu.Lock()
			remaining := ds.jobs[:0]

			for _, job := range ds.jobs {
				if now.After(job.VisibilityUntil) {
					reEnqueue(job)
				} else {
					remaining = append(remaining, job)
				}
			}

			ds.jobs = remaining
			ds.mu.Lock()
		}
	}()
}

func (pq *PriorityQ) Enqueue(job *Job) bool {
	return pq.queues[job.Priority].enqueue(job)
}

func (pq *PriorityQ) Pull() (*Job, JobStatus) {
	for p := range NumPriorities {
		if job, ok := pq.queues[p].dequeue(); ok {
			job.Status = JobInFlight
			job.VisibilityUntil = time.Now().Add(VisibilityTimeout)
			return job, JobInFlight
		}
	}
	return nil, JobReady
}

type HTTPError struct {
	Status int
	Err    error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("status %d: %v", e.Status, e.Err)
}

func isPermanentFailure(err error) bool {
	var httpErr *HTTPError

	if errors.As(err, &httpErr) {
		if httpErr.Status == http.StatusNotFound ||
			httpErr.Status == http.StatusGone {
			return true
		}
	}
	return false
}

func (pq *PriorityQ) HandleFailure(job *Job, err *error, score int) {

	if isPermanentFailure(*err) {
		job.Status = JobDead
		jobOver := &deadJob{
			job: job,
			err: err,
		}
		pq.deadLetterQ = append(pq.deadLetterQ, jobOver)
		return
	}

	job.RetryCount++

	if job.RetryCount > MaxRetries {
		job.Status = JobDead
		jobOver := &deadJob{
			job: job,
			err: err,
		}
		pq.deadLetterQ = append(pq.deadLetterQ, jobOver)
		return
	}
	delay := BaseBackoff * time.Duration(1<<job.RetryCount)
	jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(delay/2)))
	jitterTime := time.Duration(jitter.Int64())
	delay = delay - delay/4 + jitterTime

	job.VisibilityUntil = time.Now().Add(delay)
	job.Status = JobReady

	score -= int(job.RetryCount * 10)

	job.Priority = ScoreToPriority(score)
	if job.Priority < P3_LOW {
		job.Priority++
	}

	pq.delayed.Add(job)
}
