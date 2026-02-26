package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type CrawlerClient struct {
	client *http.Client
}

var UserAgent = "DevSearchCrawler/1.0"

func NewCrawlerClient(timeout time.Duration) *CrawlerClient {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 10,
	}
	return &CrawlerClient{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *CrawlerClient) FetchReq(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v\n", err)
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	return c.client.Do(req)
}

func (c *CrawlerClient) GetUserAgentCheck(ctx context.Context) {

	res, err := c.FetchReq(ctx, "https://httpbin.org/user-agent")
	if err != nil {
		log.Printf("Network error: %v\n", err)
		return
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		log.Printf("Server returned error code: %d\n", res.StatusCode)
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Failed to ready body: %v\n", err)
		return
	}
	fmt.Println(string(body))
}
