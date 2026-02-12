package frontier

import (
	"net/url"
	"strings"
)

func getPipeline(host string, path string) Normalizer {
	if strings.Contains(host, "github.com") || strings.Contains(host, "gitlab.com") {
		return GitNormalizer{}
	}
	if strings.Contains(host, "stackoverflow.com") || strings.Contains(host, "stackexchange.com") {
		return ForumNormalizer{}
	}
	docKeywords := []string{"/docs/", "/api/", "/reference/", "/guide/"}

	for _, key := range docKeywords {
		if strings.Contains(path, key) {
			return DocsNormalizer{}
		}
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
