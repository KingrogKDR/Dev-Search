package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KingrogKDR/Dev-Search/crawler"
	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/KingrogKDR/Dev-Search/storage"
)

var priorityQueues = []string{
	string(queues.P0_CRITICAL),
	string(queues.P1_HIGH),
	string(queues.P2_NORMAL),
	string(queues.P3_LOW),
}

var seedUrls = []string{
	"https://github.com",
}

func main() {
	rdb := storage.GetRedisClient()
	frontier := queues.NewQueue(rdb, "frontierqueue")

	for _, u := range seedUrls {
		job := queues.NewJob(u)

		job.BaseScore += 100

		if err := frontier.Enqueue(job); err != nil {
			log.Printf("Failed to enqueue job: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

	}
	worker := crawler.NewWorker(frontier, priorityQueues, 2)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if err := frontier.ProcessRetryJobs(); err != nil {
				log.Printf("Error processing retry jobs: %v", err)
			}
		}
	}()

	worker.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down worker...")
	worker.Stop()

}
