package languages

import "bytes"

const (
	wordBits       = 64
	minSignalBytes = 2
	quoteRemainder = 2
)

// Analyze inspects at most MaxBytes. complete must be false for a prefix whose
// suffix is unavailable. dst must be non-nil and owned by the calling goroutine.
func Analyze(data []byte, complete bool, dst *Analysis) {
	*dst = Analysis{Bytes: min(len(data), MaxBytes), Prefix: !complete || len(data) > MaxBytes}
	data = data[:dst.Bytes]
	if binary(data) {
		dst.Binary = true
		return
	}
	offset := 0
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		offset = 3
	}
	var seen [2]uint64
	var state lexicalState
	for offset < len(data) {
		end := bytes.IndexByte(data[offset:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += offset
		}
		line := data[offset:end]
		if offset == 0 || offset == 3 && bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
			if bytes.HasPrefix(line, []byte("#!")) {
				detectShebang(line, end < len(data) || !dst.Prefix, offset, dst, &seen)
				offset = end + 1
				continue
			}
		}
		start := state.codeStart(line)
		if start >= 0 {
			detectLine(bytes.TrimRight(line[start:], " \t\r"), end < len(data) || !dst.Prefix, offset+start, dst, &seen)
		}
		offset = end + 1
	}
}

func detectLine(code []byte, terminated bool, offset int, dst *Analysis, seen *[2]uint64) {
	if len(code) < minSignalBytes {
		return
	}
	for i := range rules {
		first := rules[i].prefix[0]
		if code[0] < 0x80 && first != '~' && first|0x20 != code[0]|0x20 {
			continue
		}
		if seen[i/wordBits]&(1<<uint(i%wordBits)) != 0 {
			continue
		}
		if matches(code, &rules[i], terminated) {
			record(dst, seen, i, offset)
		}
	}
}

func record(dst *Analysis, seen *[2]uint64, index, offset int) {
	if seen[index/wordBits]&(1<<uint(index%wordBits)) != 0 {
		return
	}
	seen[index/wordBits] |= 1 << uint(index%wordBits)
	dst.Signals[dst.Count] = Match{Rule: uint16(index), Offset: uint32(offset)}
	dst.Count++
}

func binary(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f' && b != '\x1b' {
			return true
		}
	}
	return false
}

func matches(line []byte, r *rule, terminated bool) bool {
	if len(line) == 0 || r.weight == declaredWeight {
		return false
	}
	if r.prefix == "~prolog" {
		return prologClause(line)
	}
	if r.prefix == "~typed" {
		return typedBinding(line)
	}
	fold := r.languages == 1<<HTML || r.languages == 1<<SQL
	if fold {
		if len(line) < len(r.prefix) || !bytes.EqualFold(line[:len(r.prefix)], []byte(r.prefix)) {
			return false
		}
	} else if !bytes.HasPrefix(line, []byte(r.prefix)) {
		return false
	}
	if r.contains != "" && !contains(line, r.contains, fold) {
		return false
	}
	if r.suffix != "" && !bytes.HasSuffix(line, []byte(r.suffix)) {
		return false
	}
	return constraints(line, r, terminated)
}

func contains(line []byte, pattern string, fold bool) bool {
	if !fold {
		return bytes.Contains(line, []byte(pattern))
	}
	for i := 0; i+len(pattern) <= len(line); i++ {
		if bytes.EqualFold(line[i:i+len(pattern)], []byte(pattern)) {
			return true
		}
	}
	return false
}

func typedBinding(line []byte) bool {
	var body []byte
	for _, prefix := range [...]string{"const ", "let ", "var "} {
		if bytes.HasPrefix(line, []byte(prefix)) {
			body = bytes.TrimSpace(line[len(prefix):])
			break
		}
	}
	if len(body) == 0 {
		return false
	}
	i := 0
	for i < len(body) && (body[i] >= 'a' && body[i] <= 'z' || body[i] >= 'A' && body[i] <= 'Z' || body[i] == '_' || body[i] == '$' || i > 0 && body[i] >= '0' && body[i] <= '9') {
		i++
	}
	if i == 0 {
		return false
	}
	body = bytes.TrimSpace(body[i:])
	return len(body) > 1 && body[0] == ':' && bytes.ContainsRune(body, '=')
}

func prologClause(line []byte) bool {
	if line[0] < 'a' || line[0] > 'z' {
		return false
	}
	open := bytes.IndexByte(line, '(')
	clause := bytes.Index(line, []byte(":-"))
	return open > 0 && clause > open && !bytes.ContainsAny(line[:clause], "\"'=")
}

func constraints(line []byte, r *rule, terminated bool) bool {
	switch r.id {
	case "ruby.def":
		return !bytes.ContainsAny(line, ":{")
	case "ruby.module":
		return !bytes.ContainsAny(line, ";{=")
	case "python.import":
		return !bytes.ContainsAny(line, ";\"'(){}") && !bytes.Contains(line, []byte(" from "))
	case "go.package":
		return !bytes.ContainsAny(line, ";{")
	case "html.root", "html.doctype", "php.open":
		return len(line) == len(r.prefix) && terminated || len(line) > len(r.prefix) && (space(line[len(r.prefix)]) || line[len(r.prefix)] == '>')
	case "c.include":
		return len(line) > len(r.prefix) && (space(line[len(r.prefix)]) || line[len(r.prefix)] == '<' || line[len(r.prefix)] == '"')
	}
	return true
}

func space(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

func nextWord(line []byte) (word, rest []byte) {
	line = bytes.TrimSpace(line)
	i := 0
	for i < len(line) && !space(line[i]) {
		i++
	}
	return line[:i], line[i:]
}

func detectShebang(line []byte, terminated bool, offset int, dst *Analysis, seen *[2]uint64) {
	word, rest := nextWord(line[2:])
	if i := bytes.LastIndexByte(word, '/'); i >= 0 {
		word = word[i+1:]
	}
	if bytes.Equal(word, []byte("env")) {
		word, rest = envCommand(rest)
	}
	if len(rest) == 0 && !terminated {
		return
	}
	if i := bytes.LastIndexByte(word, '/'); i >= 0 {
		word = word[i+1:]
	}
	for i := range rules {
		r := &rules[i]
		if r.weight != declaredWeight {
			continue
		}
		name := r.prefix[1:]
		if !bytes.HasPrefix(word, []byte(name)) {
			continue
		}
		tail := word[len(name):]
		if len(tail) > 0 && name != "python" && name != "ruby" && name != "perl" && name != "lua" {
			continue
		}
		valid := true
		for _, b := range tail {
			if b != '.' && (b < '0' || b > '9') {
				valid = false
				break
			}
		}
		if valid {
			record(dst, seen, i, offset)
			return
		}
	}
}

func envCommand(rest []byte) (word, tail []byte) {
	for {
		word, rest = nextWord(rest)
		if len(word) == 0 {
			return nil, nil
		}
		if bytes.Equal(word, []byte("-S")) || bytes.Equal(word, []byte("-i")) || bytes.Equal(word, []byte("--")) || bytes.ContainsRune(word, '=') {
			continue
		}
		// Unknown options may take arguments.
		if word[0] == '-' {
			return nil, nil
		}
		return word, rest
	}
}

// This shield handles common multiline comments and strings, without parsing.
type lexicalState struct {
	block  bool
	quote  byte
	triple bool
}

func (s *lexicalState) codeStart(line []byte) int {
	start := -1
	for i := 0; i < len(line); i++ {
		b := line[i]
		if s.block {
			if b == '*' && i+1 < len(line) && line[i+1] == '/' {
				s.block = false
				i++
			}
			continue
		}
		if s.quote != 0 {
			i = s.skipQuote(line, i)
			continue
		}
		if space(b) {
			continue
		}
		if bytes.HasPrefix(line[i:], []byte("//")) {
			break
		}
		if bytes.HasPrefix(line[i:], []byte("/*")) {
			s.block = true
			i++
			continue
		}
		if start < 0 {
			if b == ';' || b == '%' || b == '-' && i+1 < len(line) && line[i+1] == '-' {
				break
			}
			start = i
			if b == '#' {
				return start
			}
		}
		if b == '\'' || b == '"' || b == '`' {
			i = s.openQuote(line, i)
		}
	}
	if s.quote != '`' && !s.triple {
		s.quote = 0
	}
	return start
}

func (s *lexicalState) skipQuote(line []byte, i int) int {
	b := line[i]
	if b == '\\' {
		return i + 1
	}
	if b != s.quote {
		return i
	}
	if !s.triple {
		s.quote = 0
		return i
	}
	if i+2 < len(line) && line[i+1] == b && line[i+2] == b {
		s.quote = 0
		s.triple = false
		return i + quoteRemainder
	}
	return i
}

func (s *lexicalState) openQuote(line []byte, i int) int {
	s.quote = line[i]
	if i+2 < len(line) && line[i+1] == s.quote && line[i+2] == s.quote {
		s.triple = true
		return i + quoteRemainder
	}
	return i
}
