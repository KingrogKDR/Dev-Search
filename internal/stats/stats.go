package stats

import (
	"log"
	"sync/atomic"
	"time"
)

var processedPages int64
var startTime = time.Now()

func Increment() {
	atomic.AddInt64(&processedPages, 1)
}

func Report() {

	count := atomic.LoadInt64(&processedPages)

	elapsed := time.Since(startTime).Seconds()

	if elapsed == 0 {
		return
	}

	pps := float64(count) / elapsed

	log.Printf(
		"[BENCHMARK] processed=%d elapsed=%.2fs throughput=%.2f pages/sec",
		count,
		elapsed,
		pps,
	)
}
