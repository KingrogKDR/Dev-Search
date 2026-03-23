package db

import (
	"context"
	_ "embed"
	"log"
)

//go:embed schema.sql
var schema string

func RunMigrations() error {
	log.Println("Schema length:", len(schema))
	_, err := Pool.Exec(context.Background(), schema)
	return err
}
