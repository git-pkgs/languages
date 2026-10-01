package languages

import (
	"bytes"
	"strings"
	"testing"
)

func TestVimStream(t *testing.T) {
	for _, source := range []string{
		"vim: ft=python", "# vim: set ft=ruby :", "# vim: set ft=ruby", "vi: syntax=cpp", "ex: ft=ruby", " ex: ft=ruby",
		"Vim: ft=ruby", "vim>700: ft=ruby", "vim700: ft=ruby", "vim>: ft=ruby", "vim7<: ft=ruby",
		"# \u2000vim: ft=ruby", "vim: \u2000ft=ruby", "vim: set\u2000 ft=ruby", "vim: set \u2000ft=ruby:",
		"vim: ft =ruby", "vim: ft= ruby", "vim: ft = ruby", "vim: ft\u2000=ruby", "vim: ft \u2000=ruby",
		"vim: ft=ruby\u2000", "vim: ft=ruby\u2000 \t", "vim: ft=ruby\u2000 other", "vim: set ft=ruby\u2000:",
		"vim: ft=\u2000ruby", "vim: ft=\u2000", "vim: ft=ruby\\ python", "vim: ft=ruby\\:python",
		"vim: ft =ruby\\ python", "vim: set ft=ruby\\: ft=python:", "vim: ft=unknown ft=ruby", "vim: set ft=ruby : ft=python",
		"vim: :set ft=ruby", "vi:x vim: ft=ruby", "vim: ft\t=ruby\t\r\n", "vim: ft=ruby\xff", "vim: ft=ruby\xe2",
		strings.Repeat("x", 65536) + " vim: ft=ruby",
		"vim" + strings.Repeat("7", 65536) + ": set ft=ruby:",
		"vim: option=" + strings.Repeat("x", 65536) + " ft=ruby",
		"vim: ft=ruby" + strings.Repeat("\u2000", 65536),
		"vim: set ft=ruby" + strings.Repeat(" ", 65536) + ":",
	} {
		for _, chunk := range []int{1, 2, 3, 7, 31, 1024, 32768} {
			assertVimStream(t, []byte(source), chunk)
		}
	}
}

func assertVimStream(t *testing.T, line []byte, chunk int) {
	t.Helper()
	want := vimLineMode(line)
	var stream vimStream
	for at := 0; at < len(line); at += chunk {
		part := bytes.Clone(line[at:min(at+chunk, len(line))])
		stream.write(part)
		clear(part)
	}
	if got := stream.finish(); got != want {
		t.Fatalf("chunk %d: got %s want %s line=%q", chunk, got, want, line)
	}
}

func FuzzVimStream(f *testing.F) {
	for _, source := range []string{"vim: ", "# vi: ", " ex: ", "vim>700: set ", "Vim: se "} {
		f.Add([]byte(source), []byte("ft=ruby :"), uint8(1))
	}
	f.Fuzz(func(t *testing.T, prefix, body []byte, chunk uint8) {
		assertVimStream(t, append(bytes.Clone(prefix), body...), int(chunk)+1)
	})
}
