package languages

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"strconv"
)

const setWords = (int(LanguageCount) + wordBits - 1) / wordBits

// Set stores language candidates without allocation. Its zero value is empty.
type Set struct{ words [setWords]uint64 }

func NewSet(languages ...Language) (s Set) {
	for _, l := range languages {
		s.add(l)
	}
	return s
}

func (s *Set) add(l Language) {
	if l > Unknown && l < LanguageCount {
		s.words[l/wordBits] |= 1 << (l % wordBits)
	}
}

func (s Set) Has(l Language) bool {
	return l > Unknown && l < LanguageCount && s.words[l/wordBits]&(1<<(l%wordBits)) != 0
}

func (s Set) Empty() bool { return s == Set{} }

func (s Set) Len() int {
	n := 0
	for _, word := range s.words {
		n += bits.OnesCount64(word)
	}
	return n
}

func (s Set) Only() Language {
	if s.Len() != 1 {
		return Unknown
	}
	return s.first()
}

func (s Set) first() Language {
	for i, word := range s.words {
		if word != 0 {
			return Language(i*64 + bits.TrailingZeros64(word))
		}
	}
	return Unknown
}

func (s Set) Union(other Set) Set {
	for i := range s.words {
		s.words[i] |= other.words[i]
	}
	return s
}

func (s Set) Intersect(other Set) Set {
	for i := range s.words {
		s.words[i] &= other.words[i]
	}
	return s
}

func (s Set) overlaps(other Set) bool {
	for i, word := range s.words {
		if word&other.words[i] != 0 {
			return true
		}
	}
	return false
}

// MarshalJSON writes candidate names, independent of numeric language indices.
func (s Set) MarshalJSON() ([]byte, error) {
	data := []byte{'['}
	for i, word := range s.words {
		for word != 0 {
			l := Language(i*64 + bits.TrailingZeros64(word))
			word &= word - 1
			if len(data) > 1 {
				data = append(data, ',')
			}
			data = strconv.AppendQuote(data, l.String())
		}
	}
	return append(data, ']'), nil
}

func (s *Set) UnmarshalJSON(data []byte) error {
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var result Set
	for _, name := range names {
		language := Parse(name)
		if language == Unknown {
			return fmt.Errorf("unknown language %q", name)
		}
		result.add(language)
	}
	*s = result
	return nil
}
