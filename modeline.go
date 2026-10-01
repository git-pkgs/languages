package languages

import "bytes"

const (
	modelineLines = 5
	emacsMarker   = "-*-"
)

var modelineRules = func() (result [LanguageCount - 1]rule) {
	for l := Language(1); l < LanguageCount; l++ {
		result[l-1] = rule{id: "modeline", description: "editor modeline", languages: NewSet(l), weight: declaredWeight}
	}
	return result
}()

func detectModeline(data []byte, complete bool, dst *Analysis) {
	headerEnd := 0
	for range modelineLines {
		i := bytes.IndexByte(data[headerEnd:], '\n')
		if i < 0 {
			headerEnd = len(data)
			break
		}
		headerEnd += i + 1
	}
	if bytes.Contains(data[:headerEnd], []byte("UseVimball")) {
		return
	}
	if modelineWindow(data[:headerEnd], complete, 0, dst) || !complete {
		return
	}
	end := len(bytes.TrimSuffix(data, []byte("\n")))
	start := end
	for range modelineLines {
		i := bytes.LastIndexByte(data[:start], '\n')
		if i < 0 {
			start = 0
			break
		}
		start = i
	}
	if start > 0 {
		start++
	}
	if start < headerEnd {
		start = headerEnd
	}
	modelineWindow(data[start:], true, start, dst)
}

func modelineWindow(data []byte, complete bool, offset int, dst *Analysis) bool {
	for len(data) > 0 {
		line, rest, terminated := bytes.Cut(data, []byte("\n"))
		if !terminated && !complete {
			return false
		}
		language := emacsMode(line)
		if language == Unknown {
			language = vimMode(line)
		}
		if language != Unknown {
			dst.Signals[dst.Count] = Match{Rule: uint16(len(rules) + len(interpreterRules) + int(language) - 1), Offset: uint64(offset)}
			dst.Count++
			return true
		}
		offset += len(line) + 1
		data = rest
	}
	return false
}

func emacsMode(line []byte) Language {
	if len(line) <= textChunkBytes {
		return emacsLineMode(line)
	}
	var stream emacsStream
	for len(line) > 0 {
		n := min(len(line), textChunkBytes)
		stream.write(line[:n])
		line = line[n:]
	}
	return stream.result()
}

func emacsLineMode(line []byte) Language {
	_, body, found := bytes.Cut(line, []byte(emacsMarker))
	if !found {
		return Unknown
	}
	body, _, found = bytes.Cut(body, []byte(emacsMarker))
	if !found {
		return Unknown
	}
	if !bytes.ContainsAny(body, ":;") {
		return parseMode(bytes.TrimSpace(body))
	}
	for len(body) > 0 {
		var option []byte
		option, body, _ = bytes.Cut(body, []byte(";"))
		key, value, ok := bytes.Cut(option, []byte(":"))
		if ok && bytes.EqualFold(bytes.TrimSpace(key), []byte("mode")) {
			return parseMode(bytes.TrimSpace(value))
		}
	}
	return Unknown
}

func vimMode(line []byte) Language {
	if len(line) <= textChunkBytes {
		return vimLineMode(line)
	}
	var stream vimStream
	for len(line) > 0 {
		n := min(len(line), textChunkBytes)
		stream.write(line[:n])
		line = line[n:]
	}
	return stream.finish()
}

func vimLineMode(line []byte) Language {
	for i := 0; i < len(line); i++ {
		if i != 0 && line[i-1] != ' ' && line[i-1] != '\t' {
			continue
		}
		tail := line[i:]
		if !bytes.HasPrefix(tail, []byte("vi")) && !bytes.HasPrefix(tail, []byte(vimCapitalName)) && !bytes.HasPrefix(tail, []byte("ex")) {
			continue
		}
		end := bytes.IndexAny(tail, " \t:")
		if end < 0 || tail[end] != ':' || !vimMarker(tail[:end], i > 0) {
			continue
		}
		options := bytes.TrimSpace(tail[end+1:])
		if separator := bytes.IndexAny(options, " \t"); separator >= 0 && (bytes.Equal(options[:separator], []byte("set")) || bytes.Equal(options[:separator], []byte("se"))) {
			options = bytes.TrimLeft(options[separator:], " \t")
			var closed bool
			options, _, closed = bytes.Cut(options, []byte(":"))
			if !closed {
				return Unknown
			}
		}
		return vimOptions(options)
	}
	return Unknown
}

func vimMarker(name []byte, whitespace bool) bool {
	if bytes.Equal(name, []byte("vi")) || bytes.Equal(name, []byte("ex")) && whitespace {
		return true
	}
	if !bytes.HasPrefix(name, []byte(vimName)) && !bytes.HasPrefix(name, []byte(vimCapitalName)) {
		return false
	}
	version := name[len(vimName):]
	if len(version) == 0 {
		return true
	}
	if bytes.ContainsAny(version[:1], "<=>") {
		version = version[1:]
	}
	if len(version) == 0 {
		return false
	}
	for _, c := range version {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func vimOptions(options []byte) Language {
	for len(options) > 0 {
		options = bytes.TrimLeft(options, " \t:")
		i := 0
		for i < len(options) && options[i] != ' ' && options[i] != '\t' && options[i] != ':' {
			if options[i] == '\\' && i+1 < len(options) {
				i++
			}
			i++
		}
		option := options[:i]
		options = options[i:]
		key, value, found := bytes.Cut(option, []byte("="))
		if !found && modeOption(key) {
			tail := bytes.TrimLeft(options, " \t")
			if bytes.HasPrefix(tail, []byte("=")) {
				value = tail[1:]
				if end := bytes.IndexAny(value, " \t:"); end >= 0 {
					value = value[:end]
				}
				found = true
			}
		}
		if found && modeOption(key) {
			return parseMode(value)
		}
	}
	return Unknown
}

func modeOption(key []byte) bool {
	return bytes.Equal(key, []byte("ft")) || bytes.Equal(key, []byte("filetype")) || bytes.Equal(key, []byte("syntax"))
}

func parseMode(name []byte) Language {
	for _, entry := range languageAliases {
		if bytes.EqualFold(name, []byte(entry.name)) {
			return entry.language
		}
	}
	return Unknown
}
