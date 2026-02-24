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

type Job struct {
	ID              string
	Priority        uint64
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

var noOfDataQueues = 3

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

func NewPriorityQ(size uint64) *PriorityQ {
	dataQueues := make([]*DataQ, noOfDataQueues)
	for i := range noOfDataQueues {
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
				atomic.StoreUint64(&q.head, head+q.mask+1) // available for reuse after reading
				return job, true
			}
		} else if diff < 0 {
			return nil, false // slot is empty so cannot read from it
		}
	}
}
