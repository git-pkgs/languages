package languages

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

const (
	commandBytes = 64
	execPrefix   = "exec "
	selfArgument = `"$0"`
)

type commandWord struct {
	head, base                         [commandBytes]byte
	size, baseSize                     int
	equal, flagS, badFlags, badVersion bool
}

func (w *commandWord) append(data []byte) {
	for _, b := range data {
		if w.size > 0 {
			w.badFlags = w.badFlags || w.flagS || b != 'i' && b != 'v' && b != 'S'
			w.flagS = w.flagS || b == 'S'
		}
		w.equal = w.equal || b == '='
		if w.size < len(w.head) {
			w.head[w.size] = b
		}
		w.size = min(w.size+1, len(w.head)+1)
		if b == '/' {
			w.baseSize, w.badVersion = 0, false
			continue
		}
		if w.baseSize < len(w.base) {
			w.base[w.baseSize] = b
		} else {
			w.badVersion = w.badVersion || b != '.' && (b < '0' || b > '9')
		}
		w.baseSize = min(w.baseSize+1, len(w.base)+1)
	}
}

func (w *commandWord) is(value string) bool {
	return w.size == len(value) && bytes.Equal(w.head[:min(w.size, len(w.head))], []byte(value))
}

func (w *commandWord) baseIs(value string) bool {
	return w.baseSize == len(value) && bytes.Equal(w.base[:min(w.baseSize, len(w.base))], []byte(value))
}

func (w *commandWord) prefix(value string) bool {
	return bytes.HasPrefix(w.head[:min(w.size, len(w.head))], []byte(value))
}

func (w *commandWord) rule() int {
	word := w.base[:min(w.baseSize, len(w.base))]
	if w.baseIs("perl6") {
		word = []byte("raku")
	}
	for i := range rules {
		r := &rules[i]
		if r.weight != declaredWeight || !bytes.HasPrefix(word, []byte(r.prefix[1:])) {
			continue
		}
		name := r.prefix[1:]
		if len(word) > len(name) && !versionedInterpreter(name) || w.badVersion {
			continue
		}
		valid := true
		for _, b := range word[len(name):] {
			valid = valid && (b == '.' || b >= '0' && b <= '9')
		}
		if valid {
			return i
		}
	}
	if w.baseSize <= len(w.base) {
		for i := range interpreterRules {
			if bytes.Equal(word, []byte(interpreterRules[i].prefix[1:])) {
				return len(rules) + i
			}
		}
	}
	return -1
}

type commandWords struct {
	word, trimmed                commandWord
	started, separated, trailing bool
}

func (s *commandWords) rune(r rune, data []byte, visit func(commandWord, bool)) {
	if !s.started || s.separated {
		if unicode.IsSpace(r) {
			return
		}
		if s.started {
			visit(s.word, true)
		}
		*s = commandWords{started: true}
	}
	if r < utf8.RuneSelf && space(byte(r)) {
		s.separated = true
		return
	}
	if unicode.IsSpace(r) {
		if !s.trailing {
			s.trimmed = s.word
		}
		s.trailing = true
	} else {
		s.trailing = false
	}
	s.word.append(data)
}

func (s *commandWords) finish(visit func(commandWord, bool)) {
	if !s.started {
		return
	}
	word := s.word
	if s.trailing {
		word = s.trimmed
	}
	visit(word, false)
}

type commandState uint8

const (
	commandFirst commandState = iota
	commandEnv
	commandUnset
	commandNext
	commandArguments
	commandInvalid
)

type commandSelector struct {
	state                commandState
	wrapper              bool
	command              commandWord
	hasRest, osaOverride bool
}

func (s *commandSelector) word(word commandWord, more bool) {
	switch s.state {
	case commandFirst:
		if !s.wrapper && word.baseIs("env") || s.wrapper && word.is("env") {
			s.state = commandEnv
		} else {
			s.selectWord(word, more)
		}
	case commandNext:
		s.selectWord(word, more)
	case commandEnv:
		s.env(word, more)
	case commandUnset:
		s.state = commandEnv
	case commandArguments:
		s.osaOverride = s.osaOverride || word.prefix("-l")
	case commandInvalid:
	}
}

func (s *commandSelector) selectWord(word commandWord, more bool) {
	s.state, s.command, s.hasRest = commandArguments, word, more
}

func (s *commandSelector) env(word commandWord, more bool) {
	switch {
	case word.is("--"):
		s.state = commandNext
	case word.is("-u") || word.is("--unset"):
		s.state = commandUnset
	case word.size > 1 && word.head[0] == '-' && !word.badFlags:
	case word.is("--ignore-environment") || word.is("--split-string") || word.prefix("--unset="):
	case word.head[0] != '-' && word.equal:
	case word.head[0] == '-':
		s.state = commandInvalid
	default:
		s.selectWord(word, more)
	}
}

type commandLine struct {
	decoder        runeStream
	words          commandWords
	selector       commandSelector
	prefix         int
	rejected, self bool
	window         lineTail
}

func (s *commandLine) write(data []byte) {
	if s.selector.wrapper && !s.self {
		var boundary [2 * ruleWindow]byte
		n := copy(boundary[:], s.window.data[:s.window.size])
		n += copy(boundary[n:], data[:min(len(data), ruleWindow)])
		s.self = bytes.Contains(boundary[:n], []byte(selfArgument)) || bytes.Contains(data, []byte(selfArgument))
		s.window.append(data)
	}
	s.decoder.write(data, false, s.rune)
}

func (s *commandLine) rune(r rune, data []byte) {
	if s.rejected {
		return
	}
	prefix := "#!"
	if s.selector.wrapper {
		prefix = execPrefix
		if s.prefix == 0 && unicode.IsSpace(r) {
			return
		}
	}
	if s.prefix < len(prefix) {
		if r != rune(prefix[s.prefix]) {
			s.rejected = true
		} else {
			s.prefix++
		}
		return
	}
	s.words.rune(r, data, s.selector.word)
}

func (s *commandLine) finish() bool {
	s.decoder.write(nil, true, s.rune)
	s.words.finish(s.selector.word)
	prefix := "#!"
	if s.selector.wrapper {
		prefix = execPrefix
	}
	return !s.rejected && s.prefix == len(prefix)
}

type headerStream struct {
	line                 commandLine
	lines                int
	first                commandSelector
	valid, wrapperChosen bool
	wrapper              commandWord
}

func (s *headerStream) write(data []byte) {
	for len(data) > 0 && s.lines < modelineLines {
		line, rest, ended := bytes.Cut(data, []byte("\n"))
		s.line.write(line)
		if !ended {
			return
		}
		s.finishLine(true)
		data = rest
	}
}

func (s *headerStream) finishLine(terminated bool) {
	valid := s.line.finish()
	if s.lines == 0 {
		s.first = s.line.selector
		s.valid = valid && (terminated || s.first.hasRest)
	} else if !s.wrapperChosen && valid && s.line.self && terminated {
		s.wrapperChosen = true
		s.wrapper = s.line.selector.command
	}
	s.lines++
	s.line = commandLine{selector: commandSelector{wrapper: true}}
}

func (s *headerStream) finish(complete bool) int {
	if s.lines < modelineLines {
		s.finishLine(complete)
	}
	if !s.valid || s.first.state != commandArguments {
		return -1
	}
	word := s.first.command
	if shellExecutable(word.base[:min(word.baseSize, len(word.base))]) && s.wrapperChosen && s.wrapper.baseSize > 0 {
		word = s.wrapper
	}
	if word.baseIs("osascript") && s.first.osaOverride {
		return -1
	}
	return word.rule()
}
