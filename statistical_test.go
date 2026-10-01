package languages_test

import (
	"bytes"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestStatisticalRepeatedDependencies(t *testing.T) {
	data := []byte("module example.org/project\nrequire (\n")
	data = append(data, bytes.Repeat([]byte("example.org/dependency v1.0.0\n"), 1500)...)
	data = append(data, ")\n"...)
	var a languages.Analysis
	languages.Analyze(data, true, &a)
	for _, got := range []languages.Result{languages.Detect("", data), a.Result()} {
		if got.Language != languages.GoModule || !got.Statistical || got.Conflict {
			t.Fatal(got)
		}
	}
}

func TestStatisticalFallback(t *testing.T) {
	data := []byte("module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n")
	var a languages.Analysis
	languages.Analyze(data, true, &a)
	got := a.Result()
	if got.Language != languages.GoModule || !got.Statistical || got.Confidence != languages.Low || got.Conflict {
		t.Fatal(got)
	}
	if detected := languages.Detect("", data); detected != got {
		t.Fatal(detected, got)
	}
	before := a
	if combined := a.Detect("go.mod"); combined.Language != languages.GoModule || combined.Conflict || a != before {
		t.Fatal(combined, "classification changed cached analysis")
	}
	for i := range data {
		data[i] = 0
	}
	if a.Result() != got {
		t.Fatal("analysis retained input bytes")
	}
	languages.Analyze(nil, true, &a)
	if result := a.Result(); !result.Candidates.Empty() || result.Statistical {
		t.Fatal(result)
	}
}

func TestStatisticalAllocations(t *testing.T) {
	data := []byte("module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n")
	var a languages.Analysis
	if got := testing.AllocsPerRun(100, func() {
		languages.Analyze(data, true, &a)
		_ = a.Result()
		_ = a.Detect("go.mod")
	}); got != 0 && !raceEnabled {
		t.Fatalf("allocations: %v", got)
	}
}

func TestShortContentWithAmbiguousExtension(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"package.mo", "within Example;\npackage Widgets\nend Widgets;\n", languages.Modelica},
		{"hello.gs", "init\n\tprint( \"Hello, World!\" )\n", languages.Genie},
		{"top.sls", "base:\n  '*':\n    - packages\n    - webserver\n", languages.Salt},
	} {
		var a languages.Analysis
		languages.Analyze([]byte(tt.source), true, &a)
		before := a
		if got := a.Detect(tt.name); got.Language != tt.want || got.Conflict || !got.Statistical || got.Confidence != languages.Low {
			t.Errorf("%s: %+v", tt.name, got)
		}
		if a != before {
			t.Fatal("filename changed cached analysis")
		}
		if got := languages.Detect(tt.name, nil); got.Language != languages.Unknown {
			t.Errorf("empty content selected %s for %s", got.Language, tt.name)
		}
	}
}

func TestSpecificLanguageWithSharedSyntax(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"jobs.sql", `CREATE TABLE jobs (id integer, attempts integer);
CREATE FUNCTION retry_job(job_id integer) RETURNS void AS $$
BEGIN
  UPDATE jobs SET attempts = attempts + 1 WHERE id = job_id;
  RAISE NOTICE 'Retried job %', job_id;
END;
$$ LANGUAGE plpgsql;
`, languages.PLpgSQL},
		{"cursor.sql", `DECLARE @name varchar(80);
DECLARE names_cursor CURSOR FOR SELECT name FROM employees;
OPEN names_cursor;
FETCH NEXT FROM names_cursor INTO @name;
WHILE @@FETCH_STATUS = 0
BEGIN
  PRINT @name;
  FETCH NEXT FROM names_cursor INTO @name;
END;
CLOSE names_cursor;
DEALLOCATE names_cursor;
GO
`, languages.TSQL},
		{"counter.gs", `package example
uses java.util.ArrayList
class Counter {
  var _value : Integer = 0
  construct(value : Integer) {
    _value = value
  }
  property get Value() : Integer {
    return _value
  }
  function increment() {
    _value++
  }
}
`, languages.Gosu},
		{"board.sch", `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE eagle SYSTEM "eagle.dtd">
<eagle version="9.6.2">
<drawing>
<settings><setting alwaysvectorfont="no"/></settings>
<grid distance="0.1" unitdist="inch" unit="inch"/>
<layers><layer number="1" name="Top" color="4" fill="1" visible="yes" active="yes"/></layers>
<schematic><libraries/><parts/><sheets/></schematic>
</drawing>
</eagle>
`, languages.Eagle},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.source)
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			before := a
			for _, got := range []languages.Result{languages.Detect(tt.name, data), a.Detect(tt.name)} {
				if got.Language != tt.want || got.Conflict {
					t.Errorf("got %+v, want %s", got, tt.want)
				}
			}
			if a != before {
				t.Fatal("path combination changed cached analysis")
			}
		})
	}
}
