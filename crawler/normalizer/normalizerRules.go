package normalizer

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// GitPipeline : force https, strip .git, strip auth info(@git, user:pass), keep fragments only if they look like numbers(eg #L10), preserve path case, remove trailing slashes
// Docs Pipeline : path insensitive, do not strip query params like v=1.2 or path segments like /v2/, keep fragments, Normalize Locales: normalize site.com/en-us/doc and site.com/fr-fr/doc to a single canonical version site.com/doc, keep trailing slashes
// Forum Pipeline : path insensitive, keep fragments for answer ids only, strip slugs and only keep ids of questions, strip everything except tab (e.g., ?tab=active vs ?tab=votes can be useful for different views). remove trailing slashes
// General Pipeline : lowercase everything, remove all params, strip index.html or default.aspx., remove trailing slashes

// The Universal Base (Applied to ALL URLs)

// These fix protocol and syntax errors that are never functionally necessary.

//     Protocol: Lowercase (e.g., HTTP → http).

//     Host: Lowercase (e.g., GITHUB.com → github.com).

//     Port: Remove default ports (:80, :443).

//     IP Address: Standardize format. // need to look into it more

//     Path: Resolve dot segments (/a/b/../c → /a/c).

//     Path: Collapse duplicate slashes (// → /).

// 	   trim www.

//     Query: Strip tracking parameters (utm_*, fbclid, ref).

//     Query: Sort parameters alphabetically.

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
type ForumNormalizer struct{}
type GeneralNormalizer struct{}

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

func (n GitNormalizer) Normalize(u *url.URL) {
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
	if !strings.HasSuffix(u.Path, "/") && !strings.Contains(path.Base(u.Path), ".") {
		u.Path += "/"
	}
}

var rxIsNumeric = regexp.MustCompile(`^\d+$`)
var rxStartsNumeric = regexp.MustCompile(`^\d+`)

func (n ForumNormalizer) Normalize(u *url.URL) {
	u.Path = strings.ToLower(u.Path)
	if !rxIsNumeric.MatchString(u.Fragment) {
		u.Fragment = ""
	}

	segments := strings.Split(u.Path, "/")
	var cleanedSegments []string

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if rxStartsNumeric.MatchString(seg) {
			idOnly := rxIsNumeric.FindString(seg)
			cleanedSegments = append(cleanedSegments, idOnly)
			break
		}

		cleanedSegments = append(cleanedSegments, seg)
	}

	u.Path = "/" + strings.Join(cleanedSegments, "/")
	u.Path = path.Clean(u.Path)

	u.Path = strings.TrimRight(u.Path, "/")

	q := u.Query()

	tab := q.Get("tab")
	newQuery := url.Values{}
	if tab != "" {
		newQuery.Set("tab", tab)
	}
	u.RawQuery = newQuery.Encode()

}

func (n GeneralNormalizer) Normalize(u *url.URL) {
	u.Path = strings.ToLower(u.Path)

	u.Path = strings.TrimSuffix(u.Path, "/index.html/")
	u.Path = strings.TrimSuffix(u.Path, "/index.htm/")
	u.Path = strings.TrimSuffix(u.Path, "index.html")
	u.Path = strings.TrimSuffix(u.Path, "index.htm")
	u.Path = strings.TrimSuffix(u.Path, "default.aspx")
	u.Path = strings.TrimSuffix(u.Path, "default.asp")
	u.Path = strings.TrimSuffix(u.Path, "home.php")

	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		u.Path = "/"
	}
}
