package parsing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/KingrogKDR/Dev-Search/internal/indexer"
	"github.com/KingrogKDR/Dev-Search/internal/scraper/normalizer"
	"github.com/KingrogKDR/Dev-Search/internal/scraper/queues"
	"github.com/KingrogKDR/Dev-Search/internal/storage"
	"github.com/KingrogKDR/Dev-Search/internal/streams"
	"github.com/redis/go-redis/v9"

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
type ParsedPage struct {
	Text          string
	Title         string
	Links         []string
	HasCodeBlocks bool
}

func NewParsePayload(objectKey string, hash uint64, typ string) *ParsePayload {
	return &ParsePayload{
		ObjectKey: objectKey,
		Hash:      hash,
		Type:      typ,
	}
}

const (
	UrlMetaKey = "urlmeta:%s"
	Streamer   = "parser"
)

func ExtractTextAndStore(ctx context.Context, job *queues.Job, store *storage.MinioStore, frontier *queues.Queue, parseStream *streams.MsgStream) error {
	if job.Type != string(queues.JOB_PARSE) {
		return nil
	}

	log.Printf("[Parser] Starting job for URL: %s", job.URL)

	if len(job.Payload) == 0 {
		return fmt.Errorf("parser received empty payload for %s", job.URL)
	}

	parsed, err := url.Parse(job.URL)
	if err != nil {
		return fmt.Errorf("failed parsing url: %w", err)
	}
	var payload ParsePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed unmarshaling parse payload: %w", err)
	}
	if payload.ObjectKey == "" {
		return fmt.Errorf("invalid payload: missing object key")
	}

	rawData, err := store.GetObject(ctx, payload.ObjectKey)

	if err != nil {
		return fmt.Errorf("failed getting object from s3: %w", err)
	}

	parsedPage, err := extractAccordingToType(string(rawData), payload.Type, parsed)

	if err != nil {
		return fmt.Errorf("failed extracting text from raw data: %w", err)
	}

	if err = store.StoreTextData(ctx, parsedPage.Text, payload.Hash); err != nil {
		return fmt.Errorf("failed storing text data: %w", err)
	}

	currentMeta, err := getUrlMeta(ctx, job.URL)
	if err != nil {
		return fmt.Errorf("failed fetching metadata for current url: %w", err)
	}

	if currentMeta == nil {
		log.Printf("[Parser] Warning: no existing metadata found for %s, creating fresh...", job.URL)
		currentMeta = queues.NewUrlMeta(0)
		queues.ClassifyURL(parsed, currentMeta)
	}
	currentMeta.HasCodeBlocks = parsedPage.HasCodeBlocks
	snippet := generateSnippet(parsedPage.Text)

	record := indexer.NewRecord(payload.Hash, job.URL, parsedPage.Title, snippet, payload.ObjectKey, currentMeta.InboundLinks)

	recordBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("Can't marshal record: %w", err)
	}

	msg := streams.NewMsg(recordBytes, Streamer)
	log.Printf("[Parser] Publishing message for %s", job.URL)
	err = parseStream.AddMsg(msg)
	if err != nil {
		log.Printf("[Parser] Failed to publish: %v", err)
	}

	nextDepth := currentMeta.Depth + 1

	canonical, err := extractCanonicalURL(string(rawData))
	if err != nil {
		log.Printf("[Parser] canonical extraction failed: %v", err)
	}

	for _, u := range parsedPage.Links {

		normalizedUrl, err := normalizePageURL(u, canonical)
		if err != nil {
			log.Printf("[Parser] Skipping URL %s, normalization failed: %v", u, err)
			continue
		}

		metaKey := fmt.Sprintf(UrlMetaKey, normalizedUrl)

		urlParsed, err := url.Parse(normalizedUrl)
		if err != nil {
			continue
		}

		if urlParsed.Hostname() == "" {
			continue
		}

		// create metadata for the discovered URL
		newUrlMeta := queues.NewUrlMeta(nextDepth)
		queues.ClassifyURL(urlParsed, newUrlMeta)

		metaBytes, err := json.Marshal(newUrlMeta)
		if err != nil {
			log.Printf("[Parser] metadata marshal failed: %v", err)
			continue
		}

		// atomic deduplication
		added, err := frontier.Redis.SetNX(ctx, metaKey, metaBytes, 0).Result()
		if err != nil {
			log.Printf("[Parser] metadata store failed: %v", err)
			continue
		}

		// URL already discovered
		if !added {

			existingMeta, err := getUrlMeta(ctx, normalizedUrl)
			if err != nil || existingMeta == nil {
				continue
			}

			existingMeta.InboundLinks++

			updatedBytes, err := json.Marshal(existingMeta)
			if err != nil {
				continue
			}

			frontier.Redis.Set(ctx, metaKey, updatedBytes, 0)

			continue
		}

		// new URL → enqueue
		crawlJob := queues.NewJob(normalizedUrl)
		crawlJob.Type = string(queues.JOB_CRAWL)
		crawlJob.BaseScore = queues.ScoreDevURL(newUrlMeta)

		if err := frontier.Enqueue(crawlJob); err != nil {
			log.Printf("[Parser] Failed to enqueue job: %v", err)
		}
	}
	return nil
}

func extractAccordingToType(data string, typ string, baseUrl *url.URL) (*ParsedPage, error) {
	switch typ {
	case "md":
		return extractMd(data, baseUrl)
	default:
		return extractHtml(data, baseUrl)
	}
}

func extractHtml(rawHtml string, baseUrl *url.URL) (*ParsedPage, error) {
	parsedPage := &ParsedPage{
		HasCodeBlocks: false,
	}

	if strings.Contains(rawHtml, "<code") {
		parsedPage.HasCodeBlocks = true
	}

	article, err := readability.FromReader(strings.NewReader(rawHtml), baseUrl)

	if err != nil {
		return nil, err
	}

	mainText := article.TextContent

	links, err := extractLinks(rawHtml, baseUrl)

	if err != nil {
		return nil, err
	}

	parsedPage.Title = article.Title
	if parsedPage.Title == "" {
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(rawHtml))
		parsedPage.Title = strings.TrimSpace(doc.Find("title").Text())
	}
	parsedPage.Text = strings.TrimSpace(mainText)
	parsedPage.Links = links

	return parsedPage, nil
}

func extractMd(rawMd string, baseUrl *url.URL) (*ParsedPage, error) {
	parsedPage := &ParsedPage{
		HasCodeBlocks: false,
	}

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
			if node.Level == 1 && parsedPage.Title == "" {
				parsedPage.Title = title
			}
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
			parsedPage.HasCodeBlocks = true
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
		return nil, err
	}

	parsedPage.Text = textBuilder.String()
	parsedPage.Links = urls

	return parsedPage, nil
}

func generateSnippet(text string) string {
	const maxLen = 200

	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")

	if len(text) <= maxLen {
		return text
	}

	// cut at word boundary
	snippet := text[:maxLen]
	lastSpace := strings.LastIndex(snippet, " ")
	if lastSpace > 0 {
		snippet = snippet[:lastSpace]
	}

	return snippet + "..."
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

func normalizePageURL(rawURL string, canonical string) (string, error) {
	if canonical != "" {
		base, _ := url.Parse(rawURL)
		canonParsed, err := url.Parse(canonical)
		if err == nil {
			resolved := base.ResolveReference(canonParsed)
			return normalizer.RunNormalizationPipeline(resolved.String())
		}
	}
	return normalizer.RunNormalizationPipeline(rawURL)
}

func getUrlMeta(ctx context.Context, currentUrl string) (*queues.UrlMeta, error) {
	rdb := storage.GetRedisClient()
	key := fmt.Sprintf(UrlMetaKey, currentUrl)

	data, err := rdb.Get(ctx, key).Bytes()

	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var meta queues.UrlMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}
