package metadatas

import "time"

type URLMeta struct {
	URL    string
	Domain string
	Path   string
	Depth  int64

	// discovery
	DiscoveredFrom string
	InboundLinks   int64

	// content
	HasCodeBlocks bool
	ContentType   string
	WordCount     int64

	// classification
	IsDocs         bool
	IsAPI          bool
	IsSpec         bool
	IsBlog         bool
	IsRecrawl      bool
	HasQueryParams bool

	FirstSeenAt   time.Time
	LastFetchedAt time.Time
}
