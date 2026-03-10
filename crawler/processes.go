package crawler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/KingrogKDR/Dev-Search/deduplication"
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
	DomainMetaKey = "domainmeta:%s"
)

var domainMetaGroup singleflight.Group

func CheckDomainRateLimit(meta *DomainMeta) (bool, error) {
	return !time.Now().Before(meta.NextAllowedTime), nil
}

func UpdateDomainAccess(ctx context.Context, domain string, meta *DomainMeta) error {
	meta.NextAllowedTime = time.Now().Add(meta.CrawlDelay)

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	key := fmt.Sprintf(DomainMetaKey, domain)

	return storage.GetRedisClient().Set(ctx, key, data, 24*time.Hour).Err()
}

func GetDomainMetadata(ctx context.Context, domain string, scheme string) (*DomainMeta, error) {
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

		robotsResp, err := FetchReq(ctx, robotsUrl)

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

func IsAllowedByRobots(ctx context.Context, meta *DomainMeta, rawUrl string) (bool, error) {
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

func FetchReq(ctx context.Context, rawUrl string) (*http.Response, error) {
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

func ProcessGithubRepo(ctx context.Context, parsed *url.URL, meta *DomainMeta, simIndex *deduplication.SimhashIndex, store *storage.MinioStore) error {

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

	resp, err := FetchReq(ctx, api)

	if err != nil {
		return fmt.Errorf("Can't fetch repo from %s: %w", api, err)
	}
	defer resp.Body.Close()

	UpdateDomainAccess(ctx, parsed.Host, meta)

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

	simIndex.Add(hash)
	log.Printf("[GitHub] Repo README unique. Storing to MinIO (hash=%d)", hash)

	err = store.StoreData(ctx, body, repoURL, "github", hash)
	if err != nil {
		return fmt.Errorf("Can't store markdown: %w", err)
	}

	log.Printf("[GitHub] Stored README successfully for repo: %s/%s", owner, repo)

	return nil
}
