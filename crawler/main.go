package crawler

import (
	"github.com/KingrogKDR/Dev-Search/queues/frontier"
)

func main() {

	var rawURLs = []string{"HTTP://127.000.000.01:80/", "https://[2001:db8:0::01]:443//a///b", "https://www.EXAMPLE.com/a/b/../c/./d", "http://site.com?utm_source=x&z=1&a=2", "HTTP://git:password@GITHUB.COM/user/repo.git//subdir/../?utm_medium=email#L150", "https://gitlab.com/org/project.git/#not-a-number", "https://DOCS.MICROSOFT.COM/en-us/azure/v2/index.html?v=1.5#section-1", "https://site.com/FR-FR/DOC/HELP//", "https://stackoverflow.com/questions/12345/how-to-fix-go-regex/6789#6789", "https://forum.test/thread/55?tab=active&tracking=true&ref=sidebar", "HTTPS://WWW.Example.com:443/Products/Items/DEFAULT.ASPX?id=99&session=abc#top", "http://site.com/path//index.html/"}

	for _, url := range rawURLs {
		frontier.Raw
	}

}
