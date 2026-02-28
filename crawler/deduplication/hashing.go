package deduplication

import "github.com/cespare/xxhash/v2"

func HashContent(content string) uint64 {
	return xxhash.Sum64String(content)
}
