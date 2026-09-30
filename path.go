package languages

import "strings"

const (
	filenameReason     = "conventional filename"
	cFamily        Set = 1<<C | 1<<CPP | 1<<ObjectiveC
	jsFamily       Set = 1<<JavaScript | 1<<TypeScript | 1<<JSX | 1<<TSX
	tsFamily       Set = 1<<TypeScript | 1<<TSX
	jsxFamily      Set = 1<<JSX | 1<<TSX
	shellFamily    Set = 1<<Shell | 1<<Bash | 1<<Zsh
	schemeFamily   Set = 1<<Scheme | 1<<Racket
	templateFamily Set = 1<<Jinja | 1<<Twig
)

// AnalyzePath examines only the final path component, with either path separator.
// Matching is case-sensitive; directory names do not establish a language.
func AnalyzePath(path string) Context {
	if len(path) > MaxBytes {
		return Context{}
	}
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		path = path[i+1:]
	}
	switch path {
	case "Gemfile", "Rakefile", "Guardfile", "Vagrantfile":
		return Context{1 << Ruby, filenameReason}
	case ".bashrc", ".bash_profile":
		return Context{1 << Bash, filenameReason}
	case ".zshrc", ".zprofile":
		return Context{1 << Zsh, filenameReason}
	}
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return Context{}
	}
	var s Set
	switch path[i:] {
	case ".py", ".pyw":
		s = 1 << Python
	case ".rb", ".rake":
		s = 1 << Ruby
	case ".go":
		s = 1 << Go
	case ".rs":
		s = 1 << Rust
	case ".java":
		s = 1 << Java
	case ".c":
		s = 1 << C
	case ".cc", ".cpp", ".cxx", ".hpp", ".C":
		s = 1 << CPP
	case ".h":
		s = cFamily
	case ".m":
		s = 1<<ObjectiveC | 1<<MATLAB
	case ".js", ".mjs", ".cjs":
		s = 1 << JavaScript
	case ".ts", ".mts", ".cts":
		s = 1 << TypeScript
	case ".jsx":
		s = 1 << JSX
	case ".tsx":
		s = 1 << TSX
	case ".sh":
		s = shellFamily
	case ".bash":
		s = 1 << Bash
	case ".zsh":
		s = 1 << Zsh
	case ".fish":
		s = 1 << Fish
	case ".pl":
		s = 1<<Perl | 1<<Raku | 1<<Prolog
	case ".pm":
		s = 1<<Perl | 1<<Raku
	case ".pro", ".prolog":
		s = 1 << Prolog
	case ".raku", ".rakumod", ".p6":
		s = 1 << Raku
	case ".lisp", ".cl":
		s = 1 << CommonLisp
	case ".scm", ".ss":
		s = schemeFamily
	case ".rkt":
		s = 1 << Racket
	case ".clj", ".cljs", ".cljc":
		s = 1 << Clojure
	case ".php":
		s = 1 << PHP
	case ".lua":
		s = 1 << Lua
	case ".cs":
		s = 1 << CSharp
	case ".html", ".htm":
		s = 1 << HTML
	case ".xml", ".xsl", ".svg":
		s = 1 << XML
	case ".jinja", ".j2", ".jinja2":
		s = 1 << Jinja
	case ".twig":
		s = 1 << Twig
	case ".erb":
		s = 1 << ERB
	case ".sql":
		s = 1 << SQL
	}
	if s == 0 {
		return Context{}
	}
	return Context{s, "extension"}
}
