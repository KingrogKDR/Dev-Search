package worker

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/KingrogKDR/Dev-Search/crawler/frontier"
)

// update status of the url in queue and turn it hidden from other workers.
// if worker crashes before the visibility timer, retry count is added in the url metadata and a backoff algorithm is used which reduces the score of this url and pushes it towards lower priority
// fetch the url's HTML
// check for content-seen parameters (hash / similarity parameters)
// store it in S3 if the check for content deduplication is
// store domain information in domain metadata
// store the url metadata in url metadata

func ProcessFromQueue(ctx context.Context, pq *frontier.PriorityQ, userAgent string, crawler *CrawlerClient) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	job := pq.Dequeue()
	if job == nil {
		return nil
	}
	fmt.Printf("Job %d Status: %s\n", job.ID, job.Status)
	fmt.Printf("Fetching URL %s---------\n", job.URL)
	response, err := fetchUrl(ctx, job.URL, userAgent, crawler)
	if err != nil {
		log.Printf("Error while processing the url: %s\n", job.URL)
		return err
	}
	// extract the html data, hash the data and check for content similarity or duplication
	// if succeeds
	return nil

}

func fetchUrl(ctx context.Context, rawUrl string, userAgent string, crawler *CrawlerClient) (*http.Response, error) {
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		log.Printf("Error parsing url: %v\n", err)
		return nil, err
	}
	scheme, domain := parsedUrl.Scheme, parsedUrl.Host
	path := parsedUrl.Path

	fmt.Println("--------------------------------------------------")
	fmt.Println("URL:", rawUrl)
	fmt.Println("User-Agent:", userAgent)

	robots, err := GetRobotsForDomain(ctx, scheme, domain, crawler)
	if err != nil {
		return nil, err
	}

	var pageData *http.Response
	if robots == nil {
		fmt.Println("robots.txt: NOT FOUND → ALLOW by default")

		pageData, err = crawler.FetchReq(ctx, rawUrl)
		if err != nil {
			log.Printf("Error fetching page: %v. Returned status: %s\n", err, pageData.Status)
			return nil, err
		}
		defer pageData.Body.Close()
		fmt.Println("FETCHED: ", pageData.Status)
		return pageData, nil
	}

	fmt.Println("robots.txt: FOUND")

	group := robots.FindGroup(userAgent)

	fmt.Println("Matched group for UA:", userAgent)
	fmt.Println("Crawl-delay:", group.CrawlDelay)

	isAllowed := group.Test(path)
	fmt.Println("Final decision for path:", path, "→", isAllowed)

	if !isAllowed {
		fmt.Println("FETCH BLOCKED BY ROBOTS\n************")
		return nil, nil
	}

	pageData, err = crawler.FetchReq(ctx, rawUrl)
	if err != nil {
		log.Printf("Error fetching page: %v. Returned status: %s", err, pageData.Status)
		return nil, err
	}

	defer pageData.Body.Close()
	fmt.Println("Fetched: ", pageData.Status)
	return pageData, nil
}
