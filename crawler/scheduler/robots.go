package scheduler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

type DomainRobots struct {
	Data      *robotstxt.RobotsData
	FetchedAt time.Time
}

var RobotsCache sync.Map // global cache

func GetRobotsForDomain(scheme string, domain string, crawler *CrawlerClient) (*robotstxt.RobotsData, error) {

	if existingData, ok := RobotsCache.Load(domain); ok {
		return existingData.(*DomainRobots).Data, nil
	}

	robotsUrl := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	robotsFile, err := crawler.FetchReq(robotsUrl)
	if err != nil {
		log.Printf("Error fetching the robots.txt file for %s: %v\n", domain, err)
		return nil, err
	}

	defer robotsFile.Body.Close()

	if robotsFile.StatusCode == http.StatusNotFound {
		log.Printf("No robots.txt for this %s. Returned %d\n", domain, robotsFile.StatusCode)
		return nil, nil
	}

	robotsContent, err := io.ReadAll(robotsFile.Body)
	if err != nil {
		log.Printf("Error extracting robots content: %s\n", robotsContent)
		return nil, err
	}
	robotsData, err := robotstxt.FromBytes(robotsContent)
	if err != nil {
		return nil, err
	}

	RobotsCache.Store(domain, &DomainRobots{
		Data:      robotsData,
		FetchedAt: time.Now(),
	})

	return robotsData, nil
}
