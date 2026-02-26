package normalizer

import (
	"net/url"
	"strings"
)

func isGitHost(host string) bool {
	return host == "github.com" ||
		strings.HasSuffix(host, ".github.com") ||
		host == "gitlab.com" ||
		strings.HasSuffix(host, ".gitlab.com") ||
		host == "bitbucket.org"
}

func isForumHost(host string) bool {
	return host == "stackoverflow.com" ||
		strings.HasSuffix(host, ".stackexchange.com") ||
		host == "reddit.com" ||
		strings.HasPrefix(host, "forum.") ||
		strings.HasPrefix(host, "forums.")
}

func getPipeline(host string, path string) Normalizer {
	if isGitHost(host) {
		return GitNormalizer{}
	}
	if isForumHost(host) {
		return ForumNormalizer{}
	}
	docKeywords := []string{"/docs/", "/doc/", "/api/", "/reference/", "/guide/", "/documentation/"}
	for _, key := range docKeywords {
		if strings.Contains(path, key) {
			return DocsNormalizer{}
		}
	}

	if strings.HasPrefix(host, "docs.") || strings.HasPrefix(host, "api.") {
		return DocsNormalizer{}
	}

	return GeneralNormalizer{}
}

func RunNormalizationPipeline(rawURL string) (string, error) {
	parsedUrl, err := url.Parse(strings.TrimSpace(rawURL))

	if err != nil {
		return "", err
	}

	pipeline := &NormalizationPipeline{}
	pipeline.Rules = append(pipeline.Rules, BaseNormalizer{})

	host := strings.ToLower(parsedUrl.Host)
	path := strings.ToLower(parsedUrl.Path)

	specialRules := getPipeline(host, path)
	pipeline.Rules = append(pipeline.Rules, specialRules)

	pipeline.Run(parsedUrl)
	return parsedUrl.String(), nil
}
