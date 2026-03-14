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

func getPipeline(host string) Normalizer {
	if isGitHost(host) {
		return GitNormalizer{}
	}

	return DocsNormalizer{}
}

func RunNormalizationPipeline(rawURL string) (string, error) {
	parsedUrl, err := url.Parse(strings.TrimSpace(rawURL))

	if err != nil {
		return "", err
	}

	pipeline := &NormalizationPipeline{}
	pipeline.Rules = append(pipeline.Rules, BaseNormalizer{})

	host := strings.ToLower(parsedUrl.Host)

	specialRules := getPipeline(host)
	pipeline.Rules = append(pipeline.Rules, specialRules)

	pipeline.Run(parsedUrl)
	return parsedUrl.String(), nil
}
