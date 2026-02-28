package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/KingrogKDR/Dev-Search/storage"
	"github.com/temoto/robotstxt"
)

type DomainRobots struct {
	Data      *robotstxt.RobotsData
	FetchedAt time.Time
}

func GetRobotsForDomain(ctx context.Context, scheme string, domain string, crawler *CrawlerClient) (*robotstxt.RobotsData, error) {
	RobotsCache, err := storage.GetRedisClient()
	if err != nil {
		return nil, err
	}
	existingData, err := RobotsCache.Get(ctx, domain).Result()
	if err == nil {
		var cachedRobots DomainRobots
		if err := json.Unmarshal([]byte(existingData), &cachedRobots); err == nil {
			return cachedRobots.Data, nil
		}
	}

	robotsUrl := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	robotsFile, err := crawler.FetchReq(ctx, robotsUrl)
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

	RobotsCache.Set(ctx, domain, &DomainRobots{
		Data:      robotsData,
		FetchedAt: time.Now(),
	}, 1*time.Hour)

	return robotsData, nil
}
