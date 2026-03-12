package deduplication

import (
	"strings"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const ShingleSize = 3
const MaxHammingDist = 3

type SimhashIndex struct {
	Buckets [4]map[uint16][]uint64
	mu      sync.RWMutex
}

func NewSimIndex() *SimhashIndex {

	idx := &SimhashIndex{}

	for i := 0; i < 4; i++ {
		idx.Buckets[i] = make(map[uint16][]uint64)
	}

	return idx
}

func (idx *SimhashIndex) Add(hash uint64) {

	idx.mu.Lock()
	defer idx.mu.Unlock()

	parts := partitionHash(hash)

	for i := 0; i < 4; i++ {
		key := parts[i]
		idx.Buckets[i][key] = append(idx.Buckets[i][key], hash)
	}
}

func (idx *SimhashIndex) IsNearDuplicate(hash uint64, maxDist int) bool {

	idx.mu.Lock()
	defer idx.mu.Unlock()

	parts := partitionHash(hash)

	candidates := make(map[uint64]struct{})

	for i := range 4 {

		key := parts[i]

		for _, h := range idx.Buckets[i][key] {
			candidates[h] = struct{}{}
		}
	}

	for c := range candidates {
		if hammingDistance(hash, c) <= maxDist {
			return true
		}
	}

	// insert hash since it is not duplicate
	for i := range 4 {
		key := parts[i]
		idx.Buckets[i][key] = append(idx.Buckets[i][key], hash)
	}

	return false
}

func Tokenize(data string) []string {
	return strings.Fields(data)
}

func Shingles(tokens []string, n int) []string {
	if len(tokens) < 2 {
		return nil
	}

	shingles := make([]string, 0, len(tokens)-n+1)

	for i := 0; i <= len(tokens)-n; i++ {
		shingle := strings.Join(tokens[i:i+n], " ")
		shingles = append(shingles, shingle)
	}

	return shingles
}

func SimHash(shingles []string) uint64 {
	var vector [64]int
	for _, sh := range shingles {
		hash := xxhash.Sum64String(sh)

		for i := range 64 {

			bit := (uint(hash) >> uint(i)) & 1

			if bit == 1 {
				vector[i]++
			} else {
				vector[i]--
			}
		}
	}

	var fingerPrint uint64

	for i := range 64 {
		if vector[i] > 0 {
			fingerPrint |= 1 << uint(i)
		}
	}

	return fingerPrint
}

func partitionHash(hash uint64) [4]uint16 {
	return [4]uint16{
		uint16(hash >> 48),
		uint16(hash >> 32),
		uint16(hash >> 16),
		uint16(hash),
	}
}

func hammingDistance(a, b uint64) int {
	x := a ^ b

	dist := 0

	for x != 0 {
		dist++
		x &= x - 1
	}

	return dist
}
