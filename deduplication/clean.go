package deduplication

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type SourceType string

const (
	SourceHTML = "html"
	SourceMD   = "md"
)

func CleanData(data string, t SourceType) (string, error) {
	switch t {
	case SourceHTML:
		return cleanHtml(data)
	case SourceMD:
		md := cleanMarkdown(data)
		return md, nil
	}

	return "", nil
}

var spaceRegex = regexp.MustCompile(`\s+`)

func cleanHtml(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}
	doc.Find("script, style, nav, footer, aside").Remove()

	text := doc.Text()

	text = spaceRegex.ReplaceAllLiteralString(text, " ")

	text = strings.ToLower(text)

	return strings.TrimSpace(text), nil

}

var (
	badgeRegex = regexp.MustCompile(`!\[.*?\]\(.*?shields\.io.*?\)`)
	imageRegex = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	mdLink     = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	mdSyntax   = regexp.MustCompile(`[>#*_` + "`" + `~-]`)
)

func cleanMarkdown(md string) string {
	md = badgeRegex.ReplaceAllString(md, "")
	md = imageRegex.ReplaceAllString(md, "")

	md = mdLink.ReplaceAllString(md, "$1 $2") // converts md links to title + url

	md = mdSyntax.ReplaceAllString(md, "")

	md = spaceRegex.ReplaceAllString(md, " ")

	md = strings.ToLower(md)

	return strings.TrimSpace(md)
}
