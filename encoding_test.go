package languages_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/git-pkgs/languages"
)

func encodeSource(source string, width int, order binary.AppendByteOrder) []byte {
	var result []byte
	if width == 2 {
		result = order.AppendUint16(result, 0xfeff)
		for _, value := range utf16.Encode([]rune(source)) {
			result = order.AppendUint16(result, value)
		}
		return result
	}
	result = order.AppendUint32(result, 0xfeff)
	for _, value := range source {
		result = order.AppendUint32(result, uint32(value))
	}
	return result
}

func TestEncodedSource(t *testing.T) {
	const header = "# 日本語 \U0001f642\n"
	for _, width := range []int{2, 4} {
		for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
			data := encodeSource(header+"def greet(name):\n    print(name)\n", width, order)
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			if a.Binary || a.Prefix || a.Bytes != int64(len(data)) || a.Detect("example.py").Language != languages.Python {
				t.Fatalf("%d %s: %+v %+v", width, order, a, a.Result())
			}
			if got := languages.Detect("example.py", data); got.Language != languages.Python || got.Conflict {
				t.Fatal(got)
			}
			if a.Count == 0 || int(a.Signals[0].Offset) != len(encodeSource(header, width, order)) {
				t.Fatalf("%d %s: incorrect source offset: %+v", width, order, a.Signals[:a.Count])
			}
			before := a
			clear(data)
			if a.Detect("example.py").Language != languages.Python || a != before {
				t.Fatal("encoded analysis retained the input")
			}
		}
	}
}

func TestEncodedDeclarations(t *testing.T) {
	for _, source := range []string{"#!/usr/bin/python3\nprint(1)\n", "# -*- mode: python -*-\n"} {
		data := encodeSource(source, 2, binary.LittleEndian)
		if got := languages.Detect("script.py", data); got.Language != languages.Python || got.Confidence != languages.High {
			t.Fatal(got)
		}
		if got := languages.Detect("script.rb", data); got.Language != languages.Unknown || !got.Conflict {
			t.Fatal(got)
		}
	}
}

func TestEncodedPrefix(t *testing.T) {
	data := encodeSource("#!/usr/bin/python3\n# \U0001f642", 2, binary.LittleEndian)
	for _, remove := range []int{1, 2, 3} {
		prefix := data[:len(data)-remove]
		var a languages.Analysis
		languages.Analyze(prefix, false, &a)
		if a.Binary || !a.Prefix || a.Result().Language != languages.Python {
			t.Fatalf("incomplete rune in prefix: %d %+v", remove, a.Result())
		}
		languages.Analyze(prefix, true, &a)
		if !a.Binary {
			t.Fatalf("accepted malformed complete encoding: %d", remove)
		}
	}
}

func TestEncodedFullFile(t *testing.T) {
	header := strings.Repeat("# 日本語 \U0001f642\n", 10000)
	for _, width := range []int{2, 4} {
		for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
			data := encodeSource(header+"def greet(name):\n    print(name)\n", width, order)
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			if a.Binary || a.Prefix || a.Bytes != int64(len(data)) || a.Result().Language != languages.Python {
				t.Fatalf("%d %s: bytes=%d prefix=%t binary=%t result=%+v", width, order, a.Bytes, a.Prefix, a.Binary, a.Result())
			}
			if a.Count == 0 || a.Signals[0].Offset != uint64(len(encodeSource(header, width, order))) {
				t.Fatalf("%d %s: incorrect source offsets: %+v", width, order, a.Signals[:a.Count])
			}
			languages.Analyze(data[:1024], false, &a)
			if a.Binary || !a.Prefix || a.Bytes != 1024 || a.Count != 0 {
				t.Fatalf("%d %s: explicit prefix: bytes=%d prefix=%t binary=%t signals=%+v", width, order, a.Bytes, a.Prefix, a.Binary, a.Signals[:a.Count])
			}
		}
	}
}

func TestMalformedEncoding(t *testing.T) {
	for _, data := range [][]byte{
		{0xff, 0xfe, 0x00, 0xdc},
		{0xff, 0xfe, 0x00, 0xd8, 'a', 0},
		{0, 0, 0xfe, 0xff, 0, 0x11, 0, 0},
		{0xff, 0xfe, 0, 0, 0, 0xd8, 0, 0},
	} {
		var a languages.Analysis
		languages.Analyze(data, true, &a)
		if !a.Binary || !a.Result().Candidates.Empty() {
			t.Fatalf("accepted malformed encoding: %x", data)
		}
	}
}

func TestEncodedAllocations(t *testing.T) {
	data := encodeSource("# unicode \U0001f642\ndef greet(name):\n    print(name)\n", 2, binary.LittleEndian)
	before := bytes.Clone(data)
	var a languages.Analysis
	if n := testing.AllocsPerRun(100, func() { languages.Analyze(data, true, &a) }); n != 0 && !raceEnabled {
		t.Fatalf("encoded analysis allocated: %v", n)
	}
	if !bytes.Equal(data, before) {
		t.Fatal("encoded analysis modified input")
	}
}

func TestEncodedControlPolicy(t *testing.T) {
	for _, width := range []int{2, 4} {
		for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
			for _, control := range []byte{0, 1, 2, 3, '\b', '\v', 0x0f, 0x1a, 0x1f} {
				source := "#!/usr/bin/python3\n# source " + string(control) + "\n"
				var a languages.Analysis
				languages.Analyze(encodeSource(source, width, order), true, &a)
				allowed := control != 0 && control != 1
				if a.Binary == allowed || allowed && a.Result().Language != languages.Python {
					t.Fatalf("%d %s control=%02x: %+v", width, order, control, a.Result())
				}
			}
		}
	}
}

func TestMIRCFormattingControls(t *testing.T) {
	data := []byte("alias hello {\n  echo -a \x02bold\x0f normal \x0304red\x0f \x1funderline\x0f\n}\n")
	var a languages.Analysis
	languages.Analyze(data, true, &a)
	if a.Binary || a.Detect("hello.mrc").Language != languages.Parse("mIRC Script") {
		t.Fatal(a.Detect("hello.mrc"))
	}
}

func TestBinarySignaturesOverrideFilename(t *testing.T) {
	for _, signature := range []string{"%PDF-1.7", "\x89PNG\r\n\x1a\n", "PK\x03\x04", "\x7fELF"} {
		data := []byte(signature + "\npackage main\n")
		var a languages.Analysis
		languages.Analyze(data, true, &a)
		if !a.Binary || a.Count != 0 || a.Detect("main.go").Language != languages.Unknown {
			t.Fatalf("%q: %+v", signature, a.Detect("main.go"))
		}
		if result := languages.Detect("main.go", data); result.Language != languages.Unknown || !result.Conflict {
			t.Fatal(result)
		}
	}
}
