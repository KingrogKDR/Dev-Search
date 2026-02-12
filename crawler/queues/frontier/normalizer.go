package frontier

import (
	"net"
	"net/url"
	"path"
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

//     IP Address: Standardize format.

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

func (n BaseNormalizer) Normalize(u *url.URL) {
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	host, port, err := net.SplitHostPort(u.Host)
	if err == nil {
		if (u.Scheme == "http" && port == defaultHttpPort) || (u.Scheme == "https" && port == defaultHttpsPort) {
			u.Host = host
		}
	}
	ip := net.ParseIP(u.Host)
	if ip != nil {
		u.Host = ip.String()
	}
	if u.Path != "" {
		cleaned := path.Clean(u.Path)
		if u.Path[len(u.Path)-1] == '/' && cleaned != "/" {
			cleaned += "/"
		}
		u.Path = cleaned
	}

	u.Host = strings.TrimPrefix(u.Host, "www.")
	q := u.Query()
	trash := []string{"utm_source", "utm_medium", "utm_campaign", "ref", "fbclid"}
	for _, p := range trash {
		q.Del(p)
	}
	u.RawQuery = q.Encode()
}

func (n GitNormalizer) Normalize(u *url.URL) {

}

func (n DocsNormalizer) Normalize(u *url.URL) {

}

func (n ForumNormalizer) Normalize(u *url.URL) {

}

func (n GeneralNormalizer) Normalize(u *url.URL) {

}
