// Package classifier scores content with a generated lexical model.
package classifier

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/git-pkgs/languages/internal/tokenize"
)

const (
	indexWidth     = 16
	postingWidth   = 4
	fieldWidth     = 4
	tokenCacheSize = 64
	tokenCacheWays = 4
)

//go:embed index.bin
var indexData string

//go:embed tokens.bin
var tokenData string

//go:embed weights.bin
var weightData string

type Analysis struct {
	Scores [len(Names)]int64
	Tokens int
}

func Analyze(data []byte, dst *Analysis) {
	reset(dst)
	var counter tokenCounter
	scanner := tokenize.NewScanner(data)
	for token := scanner.Next(); token != nil; token = scanner.Next() {
		counter.add(token, dst)
	}
	counter.flush(dst)
}

func reset(dst *Analysis) {
	*dst = Analysis{}
	for i, prior := range priors {
		dst.Scores[i] = int64(prior)
	}
}

type tokenCounter struct {
	cache [tokenCacheSize][tokenCacheWays]tokenCount
	next  [tokenCacheSize]uint8
}

func (c *tokenCounter) add(token []byte, dst *Analysis) {
	bucket := tokenHash(token) % tokenCacheSize
	var entry *tokenCount
	for i := range c.cache[bucket] {
		if c.cache[bucket][i].token == string(token) {
			entry = &c.cache[bucket][i]
			break
		}
	}
	if entry == nil {
		name, offset, count := lookup(token)
		if count == 0 {
			return
		}
		entry = &c.cache[bucket][c.next[bucket]]
		c.next[bucket] = (c.next[bucket] + 1) % tokenCacheWays
		entry.addTo(dst)
		*entry = tokenCount{token: name, offset: offset, count: count}
	}
	dst.Tokens++
	entry.repeats++
}

func (c *tokenCounter) flush(dst *Analysis) {
	for i := range c.cache {
		for j := range c.cache[i] {
			c.cache[i][j].addTo(dst)
		}
	}
}

type tokenCount struct {
	token         string
	offset, count int
	repeats       int64
}

func (t *tokenCount) addTo(dst *Analysis) {
	for offset, end := t.offset, t.offset+t.count*postingWidth; offset < end; offset += postingWidth {
		language := binary.LittleEndian.Uint16([]byte(weightData[offset:]))
		weight := binary.LittleEndian.Uint16([]byte(weightData[offset+postingWidth/2:]))
		dst.Scores[language] += int64(weight) * t.repeats
	}
}

func tokenHash(token []byte) uint32 {
	const offset, prime = 2166136261, 16777619
	hash := uint32(offset)
	for _, b := range token {
		hash = (hash ^ uint32(b)) * prime
	}
	return hash
}

// Best returns model indices, or -1 when no candidate has token evidence.
func (a *Analysis) Best(accept func(int) bool) (first, second int) {
	first, second = -1, -1
	if a.Tokens == 0 {
		return first, second
	}
	best, next := int64(math.MinInt64), int64(math.MinInt64)
	for i, score := range a.Scores {
		if accept != nil && !accept(i) {
			continue
		}
		if score > best {
			second, next, first, best = first, best, i, score
		} else if score > next {
			second, next = i, score
		}
	}
	return first, second
}

func lookup(token []byte) (name string, offset, count int) {
	lo, hi := 0, len(indexData)/indexWidth
	for lo < hi {
		mid := lo + (hi-lo)>>1
		entry := mid * indexWidth
		start := indexUint(entry)
		length := indexUint(entry + fieldWidth)
		stored := tokenData[start : start+length]
		if string(token) == stored {
			return stored, indexUint(entry + 2*fieldWidth), indexUint(entry + 3*fieldWidth)
		}
		if string(token) < stored {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return "", 0, 0
}

func indexUint(offset int) int {
	return int(binary.LittleEndian.Uint32([]byte(indexData[offset:])))
}
