package queues

import (
	"net/url"
	"strings"
	"time"
)

type UrlMeta struct {
	Depth          int       `json:"depth"`
	HasQueryParams bool      `json:"has_query_params"`
	IsDocs         bool      `json:"is_docs"`
	IsApi          bool      `json:"is_api"`
	HasCodeBlocks  bool      `json:"has_code_blocks"`
	InboundLinks   int       `json:"inbound_links"`
	IsBlog         bool      `json:"is_blog"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
}

func NewUrlMeta(depth int) *UrlMeta {
	return &UrlMeta{
		Depth:          depth,
		HasQueryParams: false,
		IsDocs:         false,
		IsApi:          false,
		HasCodeBlocks:  false,
		InboundLinks:   0,
		IsBlog:         false,
		FirstSeenAt:    time.Now(),
	}
}

func ClassifyURL(u *url.URL, meta *UrlMeta) {

	path := strings.ToLower(u.Path)

	if strings.Contains(path, "/docs/") ||
		strings.Contains(path, "/guide/") ||
		strings.Contains(path, "/tutorial/") {
		meta.IsDocs = true
	}

	if strings.Contains(path, "/api/") ||
		strings.Contains(path, "/reference/") {
		meta.IsApi = true
	}

	if strings.Contains(path, "/blog/") ||
		strings.Contains(path, "/post/") {
		meta.IsBlog = true
	}

	if u.RawQuery != "" {
		meta.HasQueryParams = true
	}
}

func ScoreDevURL(u *UrlMeta) int {
	var score int
	score = 0

	age := min(int(time.Since(u.FirstSeenAt).Minutes()), 30)
	score += (30 - age) / 2

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
	if u.HasCodeBlocks {
		score += 30
	}

	score += min(u.InboundLinks*3, 30)

	if u.IsBlog {
		score -= 15
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
