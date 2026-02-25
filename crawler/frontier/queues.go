package frontier

import (
	"sync/atomic"
	"time"
)

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
	queues []*DataQ
}

// dead letter queue

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

func NewPriorityQ() *PriorityQ {
	dataQueues := make([]*DataQ, NumPriorities)
	for i := range NumPriorities {
		size := queueSizes[uint8(i)]
		dataQueues[i] = NewDataQ(size)
	}

	return &PriorityQ{
		queues: dataQueues,
	}
}

func (q *DataQ) Enqueue(job *Job) bool {
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

func (q *DataQ) Dequeue() (*Job, bool) {
	for {
		head := atomic.LoadUint64(&q.head)
		cell := &q.buffer[head&q.mask]
		seqno := atomic.LoadUint64(&cell.seqNo)
		diff := int64(seqno) - int64(head+1)

		if diff == 0 {
			if atomic.CompareAndSwapUint64(&q.head, head, head+1) {
				job := cell.value
				atomic.StoreUint64(&q.head, head+q.mask+1) // slot is available for reuse after reading
				return job, true
			}
		} else if diff < 0 {
			return nil, false // slot is empty so cannot read from it
		}
	}
}

func (pq *PriorityQ) Enqueue(job *Job) bool {
	return pq.queues[job.Priority].Enqueue(job)
}

func (pq *PriorityQ) Dequeue() (*Job, bool) {
	for p := range NumPriorities {
		if job, ok := pq.queues[p].Dequeue(); ok {
			return job, true
		}
	}
	return nil, false
}
