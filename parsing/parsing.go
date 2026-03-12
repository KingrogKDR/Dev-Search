package parsing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/KingrogKDR/Dev-Search/normalizer"
	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/KingrogKDR/Dev-Search/storage"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type ParsePayload struct {
	ObjectKey string `json:"object_key"`
	Hash      uint64 `json:"hash"`
	Type      string `json:"type"`
}

func NewParsePayload(objectKey string, hash uint64, typ string) *ParsePayload {
	return &ParsePayload{
		ObjectKey: objectKey,
		Hash:      hash,
		Type:      typ,
	}
}

func ExtractTextAndStore(ctx context.Context, job *queues.Job, store *storage.MinioStore, frontier *queues.Queue) error {
	log.Printf("[Parser] Starting job for URL: %s", job.URL)
	parsed, err := url.Parse(job.URL)
	if err != nil {
		return fmt.Errorf("failed parsing url: %w", err)
	}

	payloadBytes := job.Payload
	var payload ParsePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("failed unmarshaling parse payload: %w", err)
	}

	rawData, err := store.GetObject(ctx, payload.ObjectKey)

	if err != nil {
		return fmt.Errorf("failed getting object from s3: %w", err)
	}

	text, urls, err := extractAccordingToType(string(rawData), payload.Type, parsed)

	if err != nil {
		return fmt.Errorf("failed extracting text from raw data: %w", err)
	}

	// deduplicate urls

	var normalizedUrls []string

	for _, u := range urls {
		normalizedUrl, err := normalizePageURL(u, string(rawData))
		if err != nil {
			return fmt.Errorf("failed normalizing page url: %w", err)
		}
		normalizedUrls = append(normalizedUrls, normalizedUrl)
	}

	// update url metadata

	if err = store.StoreTextData(ctx, text, payload.Hash); err != nil {
		return fmt.Errorf("failed storing text data: %w", err)
	}

	for _, u := range normalizedUrls {
		job := queues.NewJob(u)

		job.BaseScore += 50

		if err := frontier.Enqueue(job); err != nil {
			return fmt.Errorf("failed to enqueue new url to frontier: %w", err)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func extractAccordingToType(data string, typ string, baseUrl *url.URL) (string, []string, error) {
	switch typ {
	case "md":
		return extractMd(data, baseUrl)
	default:
		return extractHtml(data, baseUrl)
	}
}

func extractHtml(rawHtml string, baseUrl *url.URL) (string, []string, error) {

	article, err := readability.FromReader(strings.NewReader(rawHtml), baseUrl)

	if err != nil {
		return "", nil, err
	}

	mainText := article.TextContent

	links, err := extractLinks(rawHtml, baseUrl)

	if err != nil {
		return "", nil, err
	}

	return mainText, links, nil
}

func extractMd(rawMd string, baseUrl *url.URL) (string, []string, error) {

	md := goldmark.DefaultParser()

	source := []byte(rawMd)
	doc := md.Parse(text.NewReader(source))

	var textBuilder strings.Builder
	var urls []string
	var codeBlocks []string
	seenUrls := make(map[string]struct{})

	var skipSection = false

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			var headingText strings.Builder

			for c := node.FirstChild(); c != nil; c = c.NextSibling() {
				if textNode, ok := c.(*ast.Text); ok {
					headingText.Write(textNode.Segment.Value(source))
				}
			}

			title := strings.TrimSpace(headingText.String())
			lower := strings.ToLower(title)

			if lower == "license" ||
				lower == "contributors" ||
				lower == "contributing" ||
				lower == "support" {
				skipSection = true
				return ast.WalkContinue, nil
			}

			skipSection = false

			if title != "" {
				textBuilder.WriteString(title)
				textBuilder.WriteString("\n")
			}

		case *ast.Text:
			if skipSection {
				return ast.WalkContinue, nil
			}
			text := strings.TrimSpace(string(node.Segment.Value(source)))
			if text != "" {
				textBuilder.WriteString(text)
				textBuilder.WriteString("\n")
			}

		case *ast.Link:
			if skipSection {
				return ast.WalkContinue, nil
			}
			dest := strings.TrimSpace(string(node.Destination))
			if dest == "" ||
				strings.HasPrefix(dest, "#") ||
				strings.HasPrefix(dest, "mailto:") ||
				strings.HasPrefix(dest, "javascript:") {
				return ast.WalkContinue, nil
			}

			parsed, err := url.Parse(dest)
			if err != nil {
				return ast.WalkContinue, nil
			}

			resolved := baseUrl.ResolveReference(parsed)

			dest = resolved.String()

			if _, exists := seenUrls[dest]; !exists {
				urls = append(urls, dest)
				seenUrls[dest] = struct{}{}
			}

		case *ast.AutoLink:
			if skipSection {
				return ast.WalkContinue, nil
			}

			dest := strings.TrimSpace(string(node.URL(source)))

			if dest == "" {
				return ast.WalkContinue, nil
			}

			parsed, err := url.Parse(dest)
			if err != nil {
				return ast.WalkContinue, nil
			}

			resolved := baseUrl.ResolveReference(parsed)

			dest = resolved.String()

			if _, exists := seenUrls[dest]; !exists {
				urls = append(urls, dest)
				seenUrls[dest] = struct{}{}
			}
		case *ast.FencedCodeBlock:
			var code strings.Builder

			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				code.Write(line.Value(source))
			}

			block := strings.TrimSpace(code.String())

			if block != "" {
				codeBlocks = append(codeBlocks, block)
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return "", nil, err
	}

	return textBuilder.String(), urls, nil
}

func extractLinks(rawHtml string, baseUrl *url.URL) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHtml))
	if err != nil {
		return nil, err
	}

	var urls []string

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")

		if !exists {
			return
		}

		href = strings.TrimSpace(href)

		if href == "" {
			return
		}

		if strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") ||
			strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "tel:") {
			return
		}

		parsed, err := url.Parse(href)
		if err != nil {
			return
		}

		resolved := baseUrl.ResolveReference(parsed) // converts relative urls to absolute urls

		urls = append(urls, resolved.String())
	})

	return urls, nil
}

func extractCanonicalURL(rawHTML string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	canonical, exists := doc.Find(`link[rel="canonical"]`).Attr("href")
	if !exists {
		return "", nil
	}

	return strings.TrimSpace(canonical), nil
}

func normalizePageURL(rawURL string, rawHtml string) (string, error) {
	canonical, err := extractCanonicalURL(rawHtml)
	if err != nil {
		return "", err
	}
	base, _ := url.Parse(rawURL)
	canonParsed, _ := url.Parse(canonical)

	resoled := base.ResolveReference(canonParsed)

	canonical = resoled.String()

	// prefer canonical url if available
	if canonical != "" {
		return normalizer.RunNormalizationPipeline(canonical)
	}

	return normalizer.RunNormalizationPipeline(rawURL)
}
