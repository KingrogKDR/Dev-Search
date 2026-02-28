package deduplication

import (
	"bytes"
	"net/url"
	"strings"
	"unicode"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/unicode/norm"
)

func CleanHtmlForDedup(rawHtml []byte, parsedUrl *url.URL) (string, error) {
	article, err := readability.FromReader(bytes.NewReader(rawHtml), parsedUrl)
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(article.Node.Data))

	doc.Find("script, style, noscript, svg").Remove()
	doc.Find("nav, footer, aside, header").Remove()
	doc.Find(".sidebar, .menu, .toc, .navbar, .breadcrumbs").Remove()
	doc.Find(".copy-button, .edit-link, .version-switcher").Remove()
	doc.Find(".hash-link, .anchor").Remove()

	var builder strings.Builder

	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, s *goquery.Selection) {
		builder.WriteString(" ")
		builder.WriteString(s.Text())
	})

	doc.Find("p, li").Each(func(i int, s *goquery.Selection) {
		builder.WriteString(" ")
		builder.WriteString(s.Text())
	})

	doc.Find("pre, code").Each(func(i int, s *goquery.Selection) {
		builder.WriteString(" CODEBLOCK ")
		builder.WriteString(s.Text())
		builder.WriteString(" ENDCODE ")
	})

	cleaned := builder.String()

	// unicode normalization
	cleaned = norm.NFKC.String(cleaned)

	// remove zero-width or non-breaking spaces
	cleaned = strings.ReplaceAll(cleaned, "\u00a0", " ")
	cleaned = strings.ReplaceAll(cleaned, "\u200b", "")

	cleaned = strings.ToLower(cleaned)
	cleaned = removeControlChars(cleaned)

	cleaned = strings.Join(strings.Fields(cleaned), " ")

	return cleaned, nil
}

func removeControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
