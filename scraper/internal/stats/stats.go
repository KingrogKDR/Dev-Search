package stats

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

var (
	startTime = time.Now()

	processedPages int64
	duplicates     int64
	errors         int64
	bytesFetched   int64
	fetchLatencyNs int64
	fetchCount     int64
)

func IncrementProcessed() {
	atomic.AddInt64(&processedPages, 1)
}

func IncrementDuplicate() {
	atomic.AddInt64(&duplicates, 1)
}

func IncrementError() {
	atomic.AddInt64(&errors, 1)
}

func AddBytes(n int64) {
	atomic.AddInt64(&bytesFetched, n)
}

func AddFetchLatency(d time.Duration) {
	atomic.AddInt64(&fetchLatencyNs, d.Nanoseconds())
	atomic.AddInt64(&fetchCount, 1)
}
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour

	m := d / time.Minute
	d -= m * time.Minute

	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}

	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}

	return fmt.Sprintf("%ds", s)
}
func FinalReport() {

	elapsed := time.Since(startTime)

	processed := atomic.LoadInt64(&processedPages)
	dups := atomic.LoadInt64(&duplicates)
	errs := atomic.LoadInt64(&errors)
	bytes := atomic.LoadInt64(&bytesFetched)
	latency := atomic.LoadInt64(&fetchLatencyNs)
	count := atomic.LoadInt64(&fetchCount)

	unique := processed - dups

	pps := float64(processed) / elapsed.Seconds()
	mb := float64(bytes) / (1024 * 1024)

	var avgLatency float64
	if count > 0 {
		avgLatency = float64(latency/count) / 1e6
	}

	var dupRatio float64
	if processed > 0 {
		dupRatio = float64(dups) / float64(processed)
	}

	log.Println("--------------------------------------------------")
	log.Println("Crawler Summary")
	log.Println("--------------------------------------------------")

	log.Printf("Elapsed time: %s", humanDuration(elapsed))
	log.Printf("%d pages crawled", processed)
	log.Printf("%d unique pages indexed", unique)
	log.Printf("%.2f pages/sec", pps)
	log.Printf("%.0f%% duplicates", dupRatio*100)
	log.Printf("%d errors", errs)
	log.Printf("%.2f MB downloaded", mb)
	log.Printf("%.2f ms avg fetch latency", avgLatency)

	log.Println("--------------------------------------------------")
}
