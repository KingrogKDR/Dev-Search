package crawler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/KingrogKDR/Dev-Search/deduplication"
	"github.com/KingrogKDR/Dev-Search/internal/stats"
	"github.com/KingrogKDR/Dev-Search/parsing"
	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/KingrogKDR/Dev-Search/storage"
	"github.com/redis/go-redis/v9"
	"github.com/temoto/robotstxt"
	"golang.org/x/sync/singleflight"
)

type DomainMeta struct {
	Host            string        `json:"host"`
	RobotsRaw       []byte        `json:"robots_raw"`
	NextAllowedTime time.Time     `json:"next_allowed_time"`
	CrawlDelay      time.Duration `json:"crawl_delay"`
}

const (
	UserAgent     = "Dev_Search/1.0"
	DomainMetaKey = "domainmeta:%s"
)

var domainMetaGroup singleflight.Group
var ErrRateLimited = errors.New("rate limited")

func FetchAndStoreRaw(ctx context.Context, job *queues.Job, simIndex *deduplication.SimhashIndex, store *storage.MinioStore, parseQ *queues.Queue) error {
	log.Printf("[Crawler] Starting job %s for URL: %s", job.ID, job.URL)
	parsed, err := url.Parse(job.URL)
	if err != nil {
		return fmt.Errorf("Parsing url in processing job: %w", err)
	}

	domain := parsed.Hostname()
	rawUrl := parsed.String()

	log.Printf("[Crawler] Parsed URL domain=%s", domain)

	meta, err := getDomainMetadata(ctx, domain, parsed.Scheme)
	if err != nil {
		return fmt.Errorf("Unable to get domain meta: %w", err)
	}

	isPathAllowed, err := isAllowedByRobots(meta, rawUrl)

	if err != nil {
		return fmt.Errorf("Robots error: %w", err)
	}

	if !isPathAllowed {
		log.Printf("[Crawler] Robots.txt blocked URL: %s", rawUrl)
		return nil
	}

	wait, err := reserveDomainAccess(ctx, domain, meta)
	if err != nil {
		return fmt.Errorf("Rate limiting error: %w", err)
	}

	if wait > 0 {
		log.Printf("[Crawler] Rate limit wait %s for %s", wait, domain)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if domain == "github.com" {
		log.Printf("[Crawler] Detected GitHub repo URL: %s", rawUrl)
		return processGithubRepo(ctx, parsed, meta, simIndex, store, parseQ)
	}

	log.Printf("[Crawler] Fetching URL: %s", rawUrl)
	startFetch := time.Now()

	resp, err := fetchReq(ctx, rawUrl)

	if err != nil {
		return fmt.Errorf("Can't fetch from %s: %w", rawUrl, err)
	}

	stats.AddFetchLatency(time.Since(startFetch))

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("Can't read response body: %w", err)
	}

	stats.AddBytes(int64(len(body)))

	log.Printf("[Crawler] Fetched %d bytes from %s", len(body), rawUrl)

	cleanedText, err := deduplication.CleanData(string(body), deduplication.SourceHTML)
	if err != nil {
		return fmt.Errorf("Can't clean html: %w", err)
	}

	log.Printf("[Crawler] Cleaned text length: %d", len(cleanedText))

	tokens := deduplication.Tokenize(cleanedText)
	log.Printf("[Crawler] Tokens generated: %d", len(tokens))

	shingles := deduplication.Shingles(tokens, deduplication.ShingleSize)
	log.Printf("[Crawler] Shingles generated: %d", len(shingles))

	hash := deduplication.SimHash(shingles)
	log.Printf("[Crawler] SimHash computed: %d", hash)

	isDup := simIndex.IsNearDuplicate(hash, deduplication.MaxHammingDist)

	if isDup {
		stats.IncrementDuplicate()
		log.Printf("[Crawler] Duplicate page detected: %s (hash=%d)", job.URL, hash)
		return nil
	}

	contentHash := deduplication.ComputeHash(cleanedText)
	log.Printf("[Crawler] Page unique. Storing to MinIO (hash=%d)", contentHash)

	objectKey, err := store.StoreRawData(ctx, body, job.URL, "html", contentHash)
	if err != nil {
		return fmt.Errorf("Can't store html: %w", err)
	}

	log.Printf("[Crawler] Stored page successfully: %s", job.URL)

	parsePayload := parsing.NewParsePayload(objectKey, contentHash, "html")

	payloadBytes, err := json.Marshal(parsePayload)

	if err != nil {
		return fmt.Errorf("failed marshaling parse payload: %w", err)
	}

	parseJob := queues.NewJob(job.URL)
	parseJob.Payload = payloadBytes

	err = parseQ.Enqueue(parseJob)
	if err != nil {
		return fmt.Errorf("failed to enqueue parse job: %w", err)
	}

	log.Printf("[Crawler] Parse job queued for: %s", job.URL)

	return nil
}

func reserveDomainAccess(ctx context.Context, domain string, meta *DomainMeta) (time.Duration, error) {
	now := time.Now()

	wait := time.Duration(0)

	if now.Before(meta.NextAllowedTime) {
		wait = meta.NextAllowedTime.Sub(now)
	}

	meta.NextAllowedTime = now.Add(wait + meta.CrawlDelay)

	data, err := json.Marshal(meta)
	if err != nil {
		return 0, err
	}

	key := fmt.Sprintf(DomainMetaKey, domain)

	err = storage.GetRedisClient().Set(ctx, key, data, 24*time.Hour).Err()
	if err != nil {
		return 0, err
	}

	return wait, nil
}

func getDomainMetadata(ctx context.Context, domain string, scheme string) (*DomainMeta, error) {
	rdb := storage.GetRedisClient()
	key := fmt.Sprintf(DomainMetaKey, domain)

	data, err := rdb.Get(ctx, key).Bytes()

	if err == nil {
		var meta DomainMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, err
		}
		return &meta, nil
	}

	if err != redis.Nil {
		return nil, err
	}

	v, err, _ := domainMetaGroup.Do(domain, func() (interface{}, error) {
		robotsUrl := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)

		robotsResp, err := fetchReq(ctx, robotsUrl)

		if err != nil {
			return nil, err
		}

		defer robotsResp.Body.Close()

		body, err := io.ReadAll(robotsResp.Body)
		if err != nil {
			return nil, err
		}

		robots, _ := robotstxt.FromBytes(body)

		delay := time.Second

		if robots != nil {
			group := robots.FindGroup(UserAgent)
			if group.CrawlDelay > 0 {
				delay = group.CrawlDelay
			}
		}

		meta := DomainMeta{
			Host:            domain,
			RobotsRaw:       body,
			CrawlDelay:      delay,
			NextAllowedTime: time.Now(),
		}

		encoded, err := json.Marshal(meta)
		if err != nil {
			return nil, err
		}

		if err := rdb.Set(ctx, key, encoded, 24*time.Hour).Err(); err != nil {
			return nil, err
		}

		return &meta, nil
	})

	if err != nil {
		return nil, err
	}

	return v.(*DomainMeta), nil
}

func isAllowedByRobots(meta *DomainMeta, rawUrl string) (bool, error) {
	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return false, fmt.Errorf("invalid url: %w", err)
	}

	if len(meta.RobotsRaw) == 0 {
		log.Printf("No robots.txt found for this url: %s", rawUrl)
		return true, nil
	}

	robots, err := robotstxt.FromBytes(meta.RobotsRaw)
	if err != nil || robots == nil {
		return true, nil
	}

	group := robots.FindGroup(UserAgent)

	return group.Test(parsed.Path), nil
}

func fetchReq(ctx context.Context, rawUrl string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", rawUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", rawUrl, err)
	}
	request.Header.Set("User-Agent", UserAgent)

	resp, err := CrawlerClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Error fetching request for %s: %w", rawUrl, err)
	}

	return resp, nil
}

type githubReadme struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

func processGithubRepo(ctx context.Context, parsed *url.URL, meta *DomainMeta, simIndex *deduplication.SimhashIndex, store *storage.MinioStore, parseQ *queues.Queue) error {

	repoURL := parsed.String()
	log.Printf("[GitHub] Processing repo URL: %s", repoURL)

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	if len(parts) < 2 {
		return fmt.Errorf("Invalid repo url: %s", repoURL)
	}

	owner := parts[0]
	repo := parts[1]

	repoURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)

	log.Printf("[GitHub] Repo identified owner=%s repo=%s", owner, repo)

	api := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/readme",
		owner,
		repo,
	)

	log.Printf("[GitHub] Fetching README via API: %s", api)

	resp, err := fetchReq(ctx, api)

	if err != nil {
		return fmt.Errorf("Can't fetch repo from %s: %w", api, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("Can't read response body: %w", err)
	}

	log.Printf("[GitHub] Fetched from %s, size: %d bytes", repoURL, len(body))

	var readme githubReadme

	err = json.Unmarshal(body, &readme)
	if err != nil {
		return fmt.Errorf("Can't parse README json: %w", err)
	}

	if readme.Encoding != "base64" {
		log.Printf("[GitHub] Unknown encoding for %s", repoURL)
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(readme.Content)
	if err != nil {
		return fmt.Errorf("Can't decode README: %w", err)
	}

	log.Printf("[GitHub] README decoded size: %d bytes", len(decoded))

	cleanedText, err := deduplication.CleanData(string(decoded), deduplication.SourceMD)
	if err != nil {
		return fmt.Errorf("Can't clean markdown: %w", err)
	}

	log.Printf("[GitHub] Cleaned markdown length: %d", len(cleanedText))

	tokens := deduplication.Tokenize(cleanedText)
	log.Printf("[GitHub] Tokens generated: %d", len(tokens))

	shingles := deduplication.Shingles(tokens, deduplication.ShingleSize)
	log.Printf("[GitHub] Shingles generated: %d", len(shingles))

	hash := deduplication.SimHash(shingles)
	log.Printf("[GitHub] SimHash computed: %d", hash)

	isDup := simIndex.IsNearDuplicate(hash, deduplication.MaxHammingDist)

	if isDup {
		log.Printf("[GitHub] Duplicate repo README detected: %s (hash=%d)", repoURL, hash)
		return nil
	}

	contentHash := deduplication.ComputeHash(cleanedText)

	log.Printf("[GitHub] Repo README unique. Storing to MinIO (hash=%d)", contentHash)

	objectKey, err := store.StoreRawData(ctx, body, repoURL, "github", contentHash)
	if err != nil {
		return fmt.Errorf("Can't store markdown: %w", err)
	}

	log.Printf("[GitHub] Stored README successfully for repo: %s/%s", owner, repo)

	parsePayload := parsing.NewParsePayload(objectKey, contentHash, "md")

	payloadBytes, err := json.Marshal(parsePayload)

	if err != nil {
		return fmt.Errorf("failed marshaling parse payload: %w", err)
	}

	parseJob := queues.NewJob(repoURL)
	parseJob.Payload = payloadBytes

	err = parseQ.Enqueue(parseJob)
	if err != nil {
		return fmt.Errorf("failed to enqueue parse job: %w", err)
	}

	log.Printf("[Github] Parse job queued for: %s", repoURL)

	return nil
}
