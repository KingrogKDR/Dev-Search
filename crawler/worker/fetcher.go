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

func ProcessFromQueue(pq *frontier.PriorityQ) error {
	job, status := pq.Pull()
	// check for the job.payload = url, its domain metadata and get robots.txt if available, otherwise call the robots.txt fetch func
	// fetch url
	// if succeeds
	return nil

}

func FetchRobots(rawUrl string)

func FetchPageForUrl(rawUrl string, userAgent string, crawler *CrawlerClient) error {
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		log.Printf("Error parsing url: %v\n", err)
	}
	scheme, domain := parsedUrl.Scheme, parsedUrl.Host
	path := parsedUrl.Path
	if path == "" {
		path = "/"
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println("URL:", rawUrl)
	fmt.Println("User-Agent:", userAgent)

	ctx := context.Background()
	robots, err := GetRobotsForDomain(ctx, scheme, domain, crawler)
	if err != nil {
		log.Printf("Error getting robots for the domain: %v\n", err)
		return err
	}

	var pageData *http.Response
	if robots == nil {
		fmt.Println("robots.txt: NOT FOUND → ALLOW by default")

		pageData, err = crawler.FetchReq(rawUrl)
		if err != nil {
			log.Printf("Error fetching page: %v. Returned status: %s\n", err, pageData.Status)
			return err
		}
		defer pageData.Body.Close()
		fmt.Println("FETCHED:", pageData.Status)
		return nil
	}

	fmt.Println("robots.txt: FOUND")

	group := robots.FindGroup(userAgent)

	fmt.Println("Matched group for UA:", userAgent)
	fmt.Println("Crawl-delay:", group.CrawlDelay)

	isAllowed := group.Test(path)
	fmt.Println("Final decision for path:", path, "→", isAllowed)

	if !isAllowed {
		fmt.Println("FETCH BLOCKED BY ROBOTS\n************")
		return nil
	}

	pageData, err = crawler.FetchReq(rawUrl)
	if err != nil {
		log.Printf("Error fetching page: %v. Returned status: %s", err, pageData.Status)
		return err
	}

	defer pageData.Body.Close()
	fmt.Println("Fetched: ", pageData.Status)
	return nil
}
