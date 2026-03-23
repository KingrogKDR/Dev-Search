package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/KingrogKDR/Dev-Search/internal/indexer"
	"github.com/KingrogKDR/Dev-Search/internal/indexer/worker"
	"github.com/KingrogKDR/Dev-Search/internal/storage"
	"github.com/KingrogKDR/Dev-Search/internal/storage/db"
	"github.com/KingrogKDR/Dev-Search/internal/streams"
)

func main() {
	db.InitDB()

	log.Println("Running DB migrations...")
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	defer indexer.FinalReport()
	rdb := storage.GetRedisClient()
	parserStream := streams.NewMsgStream(rdb, "parser", "indexer")

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

	indexExec := func(ctx context.Context, msg *streams.Msg) error {
		return indexer.CreateAndStoreIndexedDocument(ctx, msg, store)
	}

	indexerWorker := worker.NewWorker("Indexer", parserStream, 2, indexExec)

	indexerWorker.Start()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down worker...")
	indexerWorker.Stop()

}
