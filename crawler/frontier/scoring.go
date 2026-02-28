package frontier

import (
	"time"

	"github.com/KingrogKDR/Dev-Search/crawler/metadatas"
)

func ScoreDevURL(u metadatas.URLMeta) int64 {
	var score int64
	score = 0

	age := time.Since(u.FirstSeenAt).Minutes()
	if age < 30 {
		score += int64(30-age) / 2
	}

	if u.Depth <= 2 {
		score += 30
	}

	if !u.HasQueryParams {
		score += 10
	}

	if u.IsDocs {
		score += 40
	}
	if u.IsAPI {
		score += 35
	}
	if u.IsSpec {
		score += 50
	}

	if u.HasCodeBlocks {
		score += 30
	}
	if u.ContentType == "md" {
		score += 20
	}

	score += min(u.InboundLinks*3, 30)

	if u.IsBlog {
		score -= 15
	}
	if u.IsRecrawl {
		score -= 25
	}

	return score
}

func ScoreToPriority(score int64) PriorityStatus {
	switch {
	case score >= 90:
		return P0_IMP
	case score >= 60:
		return P1_HIGH
	case score >= 30:
		return P2_NORMAL
	default:
		return P3_LOW
	}
}

func ApplyAging(baseScore int64, waited time.Duration) int64 {
	agingFactor := int64(waited.Seconds() / 30) // +1 every 30s
	return baseScore + agingFactor
}
