package db

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func InitDB() {
	connStr := "postgres://dev:dev@localhost:5432/devsearch?sslmode=disable"
	log.Printf("Connecting to DB with: %s", connStr)
	var err error
	Pool, err = pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("Unable to connect to DB: %v", err)
	}

	log.Println("Connected to Postgres")
}
