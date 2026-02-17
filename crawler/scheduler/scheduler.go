package scheduler

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"time"
)

// pull an url from the fetch queue
// extract the domain from the url
// check the domain's robots.txt mainly for disallowed origins and crawlDelay and if the url implies that it can be fetched or not.
// feed it to the html fetcher

func ScheduleURL(u string, crawler *CrawlerClient) error {
	start := time.Now()

	defer func() {
		fmt.Printf(">>> Request for %s took: %v\n", u, time.Since(start))
	}()

	parsed, err := url.Parse(u)
	if err != nil {
		log.Println("Error parsing the url in the scheduler: ", err)
		return err
	}

	domain := parsed.Host
	scheme := parsed.Scheme
	robotsUrl := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	robotsFile, err := crawler.FetchReq(robotsUrl)
	if err != nil {
		log.Printf("Error fetching the robots.txt file for %s: %v\n", domain, err)
		return err
	}

	defer robotsFile.Body.Close()
	robotsContent, err := io.ReadAll(robotsFile.Body)
	if err != nil {
		log.Printf("Error extracting robots content: %s\n", robotsContent)
		return err
	}

	fmt.Println(string(robotsContent))
	return nil
}
