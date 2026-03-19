package indexer

type Record struct {
	ID           uint64 // text object Key
	URL          string
	Text         string
	InboundLinks int
}

func NewRecord(hash uint64, rawUrl string, text string, inboundLinks int) *Record {
	return &Record{
		ID:           hash,
		URL:          rawUrl,
		Text:         text,
		InboundLinks: inboundLinks,
	}
}
