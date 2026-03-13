package queues

import (
	"time"
)

type UrlMeta struct {
	Depth          int       `json:"depth"`
	HasQueryParams bool      `json:"has_query_params"`
	IsDocs         bool      `json:"is_docs"`
	IsApi          bool      `json:"is_api"`
	IsSpec         bool      `json:"is_spec"`
	HasCodeBlocks  bool      `json:"has_code_blocks"`
	InboundLinks   int       `json:"inbound_links"`
	ContentType    string    `json:"content_type"`
	IsBlog         bool      `json:"is_blog"`
	IsRecrawl      bool      `json:"is_recrawl"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
}

func NewUrlMeta(depth int, inbound_links int, contentType string) *UrlMeta {
	return &UrlMeta{
		Depth: depth,
		HasQueryParams: false,
		IsDocs: false,
		IsApi: false,
		IsSpec: false,
		HasCodeBlocks: false,
		InboundLinks: inbound_links,
		ContentType: contentType,
		IsBlog: false,
		IsRecrawl: false,
		FirstSeenAt: time.Now(),
	}
}

func ScoreDevURL(u *UrlMeta) int {
	var score int
	score = 0

	age := time.Since(u.FirstSeenAt).Minutes()
	if age < 30 {
		score += int(30-age) / 2
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
	if u.IsApi {
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

func ScoreToPriority(score int) PriorityStatus {
	switch {
	case score >= 90:
		return P0_CRITICAL
	case score >= 60:
		return P1_HIGH
	case score >= 30:
		return P2_NORMAL
	default:
		return P3_LOW
	}
}

func ApplyAging(baseScore int, waited time.Duration) int {
	agingFactor := min(int(waited.Seconds()/30), 30)
	return baseScore + agingFactor
}
