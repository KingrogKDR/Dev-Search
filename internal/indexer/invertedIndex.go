package indexer

import (
	"strings"
	"sync"
	"unicode"

	"github.com/reiver/go-porterstemmer"
)

var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {},

	"is": {}, "am": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},

	"have": {}, "has": {}, "had": {}, "having": {},

	"do": {}, "does": {}, "did": {}, "doing": {},

	"and": {}, "or": {}, "but": {}, "if": {}, "because": {}, "as": {},

	"until": {}, "while": {}, "i": {}, "we": {}, "me": {}, "myself": {}, "our": {}, "ours": {},

	"of": {}, "at": {}, "by": {}, "for": {}, "with": {}, "about": {}, "against": {},

	"between": {}, "into": {}, "through": {}, "during": {}, "before": {}, "after": {},

	"above": {}, "below": {}, "to": {}, "from": {}, "up": {}, "down": {},

	"in": {}, "out": {}, "on": {}, "off": {}, "over": {}, "under": {},

	"again": {}, "further": {}, "then": {}, "once": {}, "yourself": {}, "ourselves": {},

	"here": {}, "there": {}, "when": {}, "where": {}, "why": {}, "how": {},

	"all": {}, "any": {}, "both": {}, "each": {}, "few": {}, "more": {}, "most": {},

	"other": {}, "some": {}, "such": {}, "you": {}, "yours": {}, "he": {}, "she": {}, "it": {},

	"only": {}, "own": {}, "same": {}, "so": {}, "than": {}, "too": {}, "very": {},

	"can": {}, "will": {}, "just": {}, "should": {}, "now": {},
}

type Document struct {
	Record        *Record
	InvertedIndex map[string][]uint64
	mu            sync.Mutex
}

func NewDocument(record *Record) *Document {
	return &Document{
		Record:        record,
		InvertedIndex: make(map[string][]uint64),
	}
}

func (d *Document) AddDocument(text string, recordId uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	words := strings.FieldsSeq(text)
	for word := range words {
		word = strings.ToLower(word)
		if _, exists := stopwords[word]; exists {
			continue
		}
		word = stemmer(word)
		d.InvertedIndex[word] = append(d.InvertedIndex[word], recordId)
	}

	// store in database

	return nil
}

func stemmer(word string) string {
	cleanedWord := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, word)

	stemmedWord := porterstemmer.StemString(cleanedWord)
	return stemmedWord
}
