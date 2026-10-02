package languages

import (
	"bytes"
	"math/bits"

	"github.com/git-pkgs/languages/internal/classifier"
	"github.com/git-pkgs/magic"
)

const (
	wordBits           = 64
	minSignalBytes     = 2
	sourceTextControls = "\x02\x03\b\v\x0f\x1a\x1f"
)

// Analyze inspects all supplied bytes. complete must be false for a prefix whose
// suffix is unavailable. dst must be non-nil and owned by the calling goroutine.
func Analyze(data []byte, complete bool, dst *Analysis) {
	analyze(data, complete, dst, allHeuristics)
}

func analyze(data []byte, complete bool, dst *Analysis, heuristic uint16) {
	if len(data) > textChunkBytes {
		stream := readerStreams.Get().(*readerStream)
		defer readerStreams.Put(stream)
		stream.reset()
		for offset := 0; offset < len(data); offset += textChunkBytes {
			stream.write(data[offset:min(offset+textChunkBytes, len(data))])
		}
		*dst = stream.finish(complete, heuristic)
		dst.Bytes = int64(len(data))
		return
	}
	*dst = Analysis{Bytes: int64(len(data)), Prefix: !complete}
	format := magic.DetectWithOptions(data, magic.Options{Prefix: dst.Prefix, TextControls: sourceTextControls})
	if format.Kind == magic.KindBinary || format.Kind == magic.KindUnknown && format.Encoding != "" {
		dst.Binary = true
		return
	}
	switch format.Encoding {
	case magic.EncodingUTF16LE, magic.EncodingUTF16BE, magic.EncodingUTF32LE, magic.EncodingUTF32BE:
		analyzeEncoded(data, !dst.Prefix, dst, format.Encoding, heuristic)
		return
	}
	analyzeText(data, !dst.Prefix, dst, heuristic)
}

func analyzeText(data []byte, complete bool, dst *Analysis, heuristic uint16) {
	if len(data) <= textChunkBytes {
		analyzeTextBuffer(data, complete, dst, heuristic)
		return
	}
	stream := textStreams.Get().(*textStream)
	defer textStreams.Put(stream)
	stream.reset()
	for offset := 0; offset < len(data); offset += textChunkBytes {
		location := sourceMap{base: uint64(offset)}
		stream.write(data[offset:min(offset+textChunkBytes, len(data))], location.at)
	}
	*dst = stream.finish(complete, heuristic)
	dst.Bytes = int64(len(data))
}

func analyzeTextBuffer(data []byte, complete bool, dst *Analysis, heuristic uint16) {
	*dst = Analysis{Bytes: int64(len(data)), Prefix: !complete}
	detectModeline(data, !dst.Prefix, dst)
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
				detectShebang(data, line, end < len(data) || !dst.Prefix, offset, dst, &seen)
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
	if !dst.hasDeclaration() {
		var templates templateStream
		templates.write(data, func(i int) uint64 { return uint64(i) })
		templates.finish(dst)
		classifier.Analyze(data, &dst.classification)
		analyzeHeuristics(data, dst, heuristic)
	}
}

func detectLine(code []byte, terminated bool, offset int, dst *Analysis, seen *[2]uint64) {
	if len(code) < minSignalBytes {
		return
	}
	if len(code) > textChunkBytes {
		var summary lineSummary
		for len(code) > 0 {
			n := min(len(code), textChunkBytes)
			summary.write(code[:n])
			code = code[n:]
		}
		for word, candidates := range summary.finish(terminated) {
			candidates &^= seen[word]
			for candidates != 0 {
				i := word*wordBits + bits.TrailingZeros64(candidates)
				candidates &= candidates - 1
				record(dst, seen, i, offset)
			}
		}
		return
	}
	for word, candidates := range ruleStarts[code[0]] {
		candidates &^= seen[word]
		for candidates != 0 {
			i := word*wordBits + bits.TrailingZeros64(candidates)
			candidates &= candidates - 1
			if matches(code, &rules[i], terminated) {
				record(dst, seen, i, offset)
			}
		}
	}
}

func record(dst *Analysis, seen *[2]uint64, index, offset int) {
	if seen[index/wordBits]&(1<<uint(index%wordBits)) != 0 {
		return
	}
	seen[index/wordBits] |= 1 << uint(index%wordBits)
	dst.Signals[dst.Count] = Match{Rule: uint16(index), Offset: uint64(offset)}
	dst.Count++
}

func matches(line []byte, r *rule, terminated bool) bool {
	if len(line) == 0 || r.weight == declaredWeight || r.id == goTemplateID {
		return false
	}
	if r.prefix == prologPrefix {
		return prologClause(line)
	}
	if r.prefix == typedPrefix {
		return typedBinding(line)
	}
	fold := r.languages == NewSet(HTML) || r.languages == NewSet(SQL)
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
	for _, prefix := range [...]string{constPrefix, "let ", "var "} {
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
	case rubyRequireID:
		value := bytes.TrimSpace(line[len(r.prefix):])
		value = bytes.TrimSpace(bytes.TrimPrefix(value, []byte("(")))
		return len(value) > 0 && (value[0] == '\'' || value[0] == '"' || value[0] >= 'A' && value[0] <= 'Z')
	case rubyDefID:
		return !bytes.ContainsAny(line, ":{")
	case rubyModuleID:
		return len(line) > len(r.prefix) && line[len(r.prefix)] >= 'A' && line[len(r.prefix)] <= 'Z' && !bytes.ContainsAny(line, ";{=./")
	case pythonImportID:
		return terminated && !bytes.ContainsAny(line, ";\"'(){}") && !bytes.Contains(line, []byte(" from "))
	case goFuncID, swiftFuncID, goSwiftFuncID:
		return functionDeclaration(line, terminated) == r.languages
	case javaPackageID, javaImportID:
		return !bytes.ContainsAny(line, ":\"'{}()=")
	case matlabFunctionID:
		return matlabFunction(line)
	case racketLangID:
		word, _ := nextWord(line[len(r.prefix):])
		return racketLanguage(word)
	case goPackageID:
		return !bytes.ContainsAny(line, ";{")
	case htmlRootID, htmlDoctypeID, phpOpenID, hackOpenID:
		return len(line) == len(r.prefix) && terminated || len(line) > len(r.prefix) && (space(line[len(r.prefix)]) || line[len(r.prefix)] == '>')
	case cIncludeID:
		return len(line) > len(r.prefix) && (space(line[len(r.prefix)]) || line[len(r.prefix)] == '<' || line[len(r.prefix)] == '"')
	}
	return true
}

func functionDeclaration(line []byte, terminated bool) Set {
	header, _, body := bytes.Cut(line, []byte("{"))
	if !terminated && !body || !bytes.ContainsRune(header, ')') {
		return Set{}
	}
	if bytes.ContainsRune(header, ':') || bytes.Contains(header, []byte("->")) {
		return NewSet(Swift)
	}
	_, parameters, _ := bytes.Cut(header, []byte("("))
	if bytes.Equal(bytes.TrimSpace(parameters), []byte(")")) {
		return NewSet(Go, Swift)
	}
	return NewSet(Go)
}

func matlabFunction(line []byte) bool {
	_, value, _ := bytes.Cut(line, []byte("="))
	value = bytes.TrimSpace(value)
	return len(value) > 0 && (value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z') && !bytes.ContainsAny(value, "|{")
}

func racketLanguage(word []byte) bool {
	for _, name := range [...]string{"racket", "typed/racket", "scheme"} {
		if bytes.Equal(word, []byte(name)) || bytes.HasPrefix(word, []byte(name+"/")) {
			return true
		}
	}
	return bytes.HasPrefix(word, []byte("scribble/"))
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

func detectShebang(data, line []byte, terminated bool, offset int, dst *Analysis, seen *[2]uint64) {
	if len(data) <= textChunkBytes {
		detectShebangLine(data, line, terminated, offset, dst, seen)
		return
	}
	var stream headerStream
	data = data[offset:]
	for len(data) > 0 && stream.lines < modelineLines {
		n := min(len(data), textChunkBytes)
		stream.write(data[:n])
		data = data[n:]
	}
	index := stream.finish(!dst.Prefix)
	if index < 0 {
		return
	}
	if index < len(rules) {
		record(dst, seen, index, offset)
	} else {
		dst.Signals[dst.Count] = Match{Rule: uint16(index), Offset: uint64(offset)}
		dst.Count++
	}
}

func detectShebangLine(data, line []byte, terminated bool, offset int, dst *Analysis, seen *[2]uint64) {
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
	if shellExecutable(word) {
		if command := wrapperCommand(data, !dst.Prefix); len(command) > 0 {
			word = command
		}
	}
	if bytes.Equal(word, []byte("perl6")) {
		word = []byte("raku")
	}
	if osaLanguageOverride(word, rest) {
		return
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
		if len(tail) > 0 && !versionedInterpreter(name) {
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
	for i := range interpreterRules {
		if bytes.Equal(word, []byte(interpreterRules[i].prefix[1:])) {
			dst.Signals[dst.Count] = Match{Rule: uint16(len(rules) + i), Offset: uint64(offset)}
			dst.Count++
			return
		}
	}
}

func versionedInterpreter(name string) bool {
	return name == "python" || name == "ruby" || name == "perl" || name == "lua"
}

func osaLanguageOverride(command, options []byte) bool {
	if !bytes.Equal(command, []byte("osascript")) {
		return false
	}
	for len(options) > 0 {
		var option []byte
		option, options = nextWord(options)
		if bytes.HasPrefix(option, []byte("-l")) {
			return true
		}
	}
	return false
}

func envCommand(rest []byte) (word, tail []byte) {
	for {
		word, rest = nextWord(rest)
		if len(word) == 0 {
			return nil, nil
		}
		if bytes.Equal(word, []byte("--")) {
			return nextWord(rest)
		}
		if bytes.Equal(word, []byte("-u")) || bytes.Equal(word, []byte("--unset")) {
			_, rest = nextWord(rest)
			continue
		}
		if envFlags(word) || bytes.Equal(word, []byte("--ignore-environment")) || bytes.Equal(word, []byte("--split-string")) || bytes.HasPrefix(word, []byte("--unset=")) {
			continue
		}
		if word[0] != '-' && bytes.ContainsRune(word, '=') {
			continue
		}
		// Unknown options may take arguments.
		if word[0] == '-' {
			return nil, nil
		}
		return word, rest
	}
}

func envFlags(word []byte) bool {
	if len(word) < 2 || word[0] != '-' {
		return false
	}
	for i, b := range word[1:] {
		if b != 'i' && b != 'v' && (b != 'S' || i != len(word)-2) {
			return false
		}
	}
	return true
}

func shellExecutable(word []byte) bool {
	switch string(word) {
	case "sh", "bash", "dash", "ksh", "zsh":
		return true
	}
	return false
}

func wrapperCommand(data []byte, complete bool) []byte {
	for range 5 {
		line, rest, terminated := bytes.Cut(data, []byte("\n"))
		if !terminated && !complete {
			return nil
		}
		data = rest
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("exec ")) || !bytes.Contains(line, []byte("\"$0\"")) {
			continue
		}
		word, tail := nextWord(line[len("exec "):])
		if bytes.Equal(word, []byte("env")) {
			word, _ = envCommand(tail)
		}
		if i := bytes.LastIndexByte(word, '/'); i >= 0 {
			word = word[i+1:]
		}
		return word
	}
	return nil
}

func commentLine(line []byte) bool {
	return line[0] == ';' || bytes.HasPrefix(line, []byte("--"))
}
