package normalizer

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var defaultHttpPort = "80"
var defaultHttpsPort = "443"

type Normalizer interface {
	Normalize(u *url.URL)
}

type NormalizationPipeline struct {
	Rules []Normalizer
}

func (p *NormalizationPipeline) Run(u *url.URL) {
	for _, rule := range p.Rules {
		rule.Normalize(u)
	}
}

type BaseNormalizer struct{}
type GitNormalizer struct{}
type DocsNormalizer struct{}

func normalizeIPv4(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return host
	}

	var normalized []string
	for _, part := range parts {
		// Convert to int to remove leading zeros, then back to string
		var num int
		if _, err := fmt.Sscanf(part, "%d", &num); err == nil && num >= 0 && num <= 255 {
			normalized = append(normalized, fmt.Sprintf("%d", num))
		} else {
			return host
		}
	}
	return strings.Join(normalized, ".")
}

func (n BaseNormalizer) Normalize(u *url.URL) {
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	normalizedHost := normalizeIPv4(host)

	if ip := net.ParseIP(normalizedHost); ip != nil {
		host = ip.String()
	}

	host = strings.TrimPrefix(host, "www.")

	if port != "" {
		if !((u.Scheme == "http" && port == defaultHttpPort) || (u.Scheme == "https" && port == defaultHttpsPort)) {
			u.Host = host + ":" + port
		} else {
			u.Host = host
		}
	} else {
		u.Host = host
	}

	if u.Path != "" {
		u.Path = path.Clean(u.Path)
	}

	q := u.Query()
	trash := []string{"utm_source", "utm_medium", "utm_campaign", "ref", "fbclid"}
	for _, p := range trash {
		q.Del(p)
	}
	u.RawQuery = q.Encode()
}

var rxGitExtension = regexp.MustCompile(`(?i)\.git/?$`)
var rxGitFragment = regexp.MustCompile(`^(L?\d+(-L?\d+)?)$`)

var bannedGitPaths = []string{
	"/issues",
	"/pulls",
	"/actions",
	"/projects",
	"/graphs",
	"/stargazers",
	"/network",
}

func (n GitNormalizer) Normalize(u *url.URL) {

	for _, p := range bannedGitPaths {
		if strings.Contains(u.Path, p) {
			u.Path = ""
		}
	}
	u.Scheme = "https"
	u.User = nil
	u.Path = rxGitExtension.ReplaceAllString(u.Path, "")

	if !rxGitFragment.MatchString(u.Fragment) {
		u.Fragment = ""
	}

	u.Path = strings.TrimRight(u.Path, "/")
}

var rxLocalePrefix = regexp.MustCompile(`(?i)^/[a-z]{2}-[a-z]{2}/`)

func (n DocsNormalizer) Normalize(u *url.URL) {
	if rxLocalePrefix.MatchString(u.Path) {
		u.Path = rxLocalePrefix.ReplaceAllString(u.Path, "/")
	}
	u.Path = strings.ToLower(u.Path)
	for _, idx := range []string{"index.html", "index.htm", "index.md"} {
		if p, ok := strings.CutSuffix(u.Path, idx); ok {
			u.Path = p
			break
		}
	}
	if !strings.HasSuffix(u.Path, "/") && !strings.Contains(path.Base(u.Path), ".") {
		u.Path += "/"
	}

	u.RawQuery = ""
}
