package parsing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

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
type ParsedPage struct {
	Text          string
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
)

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
		queues.ClassifyURL(parsed, currentMeta)
		currentMeta = queues.NewUrlMeta(0)
	}

	nextDepth := currentMeta.Depth + 1
	currentMeta.HasCodeBlocks = parsedPage.HasCodeBlocks

	for _, u := range parsedPage.Links {
		normalizedUrl, err := normalizePageURL(u, string(rawData))
		if err != nil {
			log.Printf("[Parser] Skipping URL %s, normalization failed: %v", u, err)
			continue
		}
		metaKey := fmt.Sprintf(UrlMetaKey, normalizedUrl)
		// check for duplication
		exists, err := frontier.Redis.Exists(ctx, metaKey).Result()
		if err != nil {
			log.Printf("[Parser] Redis exists check failed for %s: %v", normalizedUrl, err)
			continue
		}
		discoveredUrlMeta, err := getUrlMeta(ctx, normalizedUrl)
		if err != nil {
			log.Printf("[Parser] can't get url meta for %s: %v", normalizedUrl, err)
			continue
		}

		if exists > 0 {
			discoveredUrlMeta.InboundLinks++
			continue
		}
		// create default url metadata for the unique urls
		urlParsed, err := url.Parse(normalizedUrl)

		newUrlMeta := queues.NewUrlMeta(nextDepth)

		queues.ClassifyURL(urlParsed, newUrlMeta)

		metaBytes, err := json.Marshal(newUrlMeta)
		if err != nil {
			log.Printf("[Parser] metadata marshal failed: %v", err)
			continue
		}

		err = frontier.Redis.Set(ctx, metaKey, metaBytes, 0).Err()
		if err != nil {
			log.Printf("[Parser] metadata store failed: %v", err)
			continue
		}

		// enqueue these new urls to the frontier
		job := queues.NewJob(normalizedUrl)
		job.BaseScore = queues.ScoreDevURL(newUrlMeta)
		if err := frontier.Enqueue(job); err != nil {
			log.Printf("Failed to enqueue job: %v", err)
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

	parsedPage.Text = mainText
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

func getUrlMeta(ctx context.Context, currentUrl string) (*queues.UrlMeta, error) {
	rdb := storage.GetRedisClient()
	key := fmt.Sprintf(UrlMetaKey, currentUrl)

	data, err := rdb.Get(ctx, key).Bytes()

	if err != nil {
		return nil, err
	}

	var meta queues.UrlMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
