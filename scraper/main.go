package scraper

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KingrogKDR/Dev-Search/scraper/crawler"
	"github.com/KingrogKDR/Dev-Search/scraper/deduplication"
	"github.com/KingrogKDR/Dev-Search/scraper/internal/stats"
	"github.com/KingrogKDR/Dev-Search/scraper/parsing"
	"github.com/KingrogKDR/Dev-Search/scraper/queues"
	"github.com/KingrogKDR/Dev-Search/scraper/worker"
	"github.com/KingrogKDR/Dev-Search/storage"
)

var priorityQueues = []string{
	string(queues.P0_CRITICAL),
	string(queues.P1_HIGH),
	string(queues.P2_NORMAL),
	string(queues.P3_LOW),
}

var seedUrls = []string{
	"https://go.dev/doc/",
	"https://doc.rust-lang.org/book/",
	"https://docs.python.org/3/",
	"https://developer.mozilla.org/en-US/docs/Web/JavaScript",
	"https://docs.docker.com/",
	"https://kubernetes.io/docs/",
	"https://redis.io/docs/",
	"https://github.com/donnemartin/system-design-primer",
	"https://github.com/codecrafters-io/build-your-own-x",
	"https://github.com/public-apis/public-apis",
}

var Store *storage.MinioStore

func main() {
	rdb := storage.GetRedisClient()
	frontier := queues.NewQueue(rdb, "frontier")
	parseQ := queues.NewQueue(rdb, "parser")

	simIndex := deduplication.NewSimIndex()

	store, err := storage.NewMinioStore(
		"localhost:9000",
		"minioadmin",
		"minioadmin",
		"devsearch-data",
		false,
	)

	if err != nil {
		log.Fatal(err)
	}

	err = store.EnsureBucket(context.Background())
	if err != nil {
		log.Fatal("Bucket doesn't exist in s3:", err)
	}

	for _, u := range seedUrls {
		job := queues.NewJob(u)
		job.Type = string(queues.JOB_CRAWL)

		job.BaseScore += 100

		if err := frontier.Enqueue(job); err != nil {
			log.Printf("Failed to enqueue job: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

	}

	crawlExec := func(ctx context.Context, job *queues.Job) error {
		return crawler.FetchAndStoreRaw(ctx, job, simIndex, store, parseQ)
	}

	parseExec := func(ctx context.Context, job *queues.Job) error {
		return parsing.ExtractTextAndStore(ctx, job, store, frontier)
	}

	crawlerWorker := worker.NewWorker(crawler.UserAgent, frontier, priorityQueues, 8, crawlExec)
	parserWorker := worker.NewWorker("Parser", parseQ, priorityQueues, 6, parseExec)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if err := frontier.ProcessRetryJobs(); err != nil {
				log.Printf("Error processing retry jobs: %v", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if err := parseQ.ProcessRetryJobs(); err != nil {
				log.Printf("Error processing retry jobs: %v", err)
			}
		}
	}()

	crawlerWorker.Start()
	parserWorker.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down worker...")
	parserWorker.Stop()
	crawlerWorker.Stop()

	stats.FinalReport()
}
