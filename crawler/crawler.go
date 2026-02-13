package crawler

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/KingrogKDR/Dev-Search/crawler/queues/frontier"
)

func Crawler() {

	var seedUrls = []string{"HTTP://127.000.000.01:80/", "https://[2001:db8:0::01]:443//a///b", "https://www.EXAMPLE.com/a/b/../c/./d", "http://site.com?utm_source=x&z=1&a=2", "HTTP://git:password@GITHUB.COM/user/repo.git//subdir/../?utm_medium=email#L150", "https://gitlab.com/org/project.git/#not-a-number", "https://DOCS.MICROSOFT.COM/en-us/azure/v2/index.html?v=1.5#section-1", "https://site.com/FR-FR/DOC/HELP//", "https://stackoverflow.com/questions/12345/how-to-fix-go-regex/6789#6789", "https://forum.test/thread/55?tab=active&tracking=true&ref=sidebar", "HTTPS://WWW.Example.com:443/Products/Items/DEFAULT.ASPX?id=99&session=abc#top", "http://site.com/path//index.html/"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	rawQ := frontier.NewRawQueue(100)
	fetchQ := frontier.NewFetchQueue(100)

	go func() {
		for u := range fetchQ.Items {
			fmt.Println("FETCH:", u)
		}
	}()

	rawQ.AddUrls(seedUrls)
	for i := 0; i < runtime.NumCPU(); i++ {
		go frontier.NormalizerWorker(ctx, rawQ, fetchQ)
	}

	time.Sleep(1 * time.Second)
	rawQ.Close()

	fmt.Println("done")

}
