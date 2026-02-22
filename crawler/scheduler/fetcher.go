package scheduler

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
)

// fetch the url's HTML
// check for content-seen parameters (hash / similarity parameters)
// store it in S3 if the check for content deduplication is
// store domain information in domain metadata

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

	robots, err := GetRobotsForDomain(scheme, domain, crawler)
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
		fmt.Println("FETCH BLOCKED BY ROBOTS")
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
