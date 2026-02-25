package metadatas

import "time"

type URLMeta struct {
	URL    string
	Domain string
	Path   string
	Depth  int

	// discovery
	DiscoveredFrom string
	InboundLinks   int

	// content
	HasCodeBlocks bool
	ContentType   string
	WordCount     int

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
