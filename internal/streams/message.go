package streams

import (
	"time"

	"github.com/KingrogKDR/Dev-Search/internal/indexer"
)

type Msg struct {
	ID            string
	Record        *indexer.Record
	Status        string
	AddedAt       time.Time
	LastFetchedAt time.Time
}
