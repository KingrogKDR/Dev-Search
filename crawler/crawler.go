package crawler

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/KingrogKDR/Dev-Search/crawler/queues/frontier"
	"github.com/KingrogKDR/Dev-Search/crawler/scheduler"
)

func Crawler() {

	var seedUrls = []string{"https://www.github.com", "https://stackoverflow.com"}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	rawQ := frontier.NewRawQueue(100)
	fetchQ := frontier.NewFetchQueue(100)
	crawler := scheduler.NewCrawlerClient(10 * time.Second)

	rawQ.AddUrls(seedUrls)
	for i := 0; i < runtime.NumCPU(); i++ {
		go frontier.NormalizerWorker(ctx, rawQ, fetchQ)
	}

	var wg sync.WaitGroup

	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			fmt.Printf("[Worker %d] Waiting for URL...\n", workerID)
			for {
				select {
				case u, ok := <-fetchQ.Items:
					if !ok {
						fmt.Printf("[Worker %d] channel closed, exiting...\n", workerID)
						return
					}

					err := scheduler.ScheduleURL(u, crawler)
					if err != nil {
						log.Printf("[Worker %d] Error: %v\n", workerID, err)
						return
					}

				case <-ctx.Done():
					fmt.Printf("[Worker %d] context cancelled, shutting down...\n", workerID)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	rawQ.Close()

	fmt.Println("done")

}
