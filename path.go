package languages

import "strings"

const (
	filenameReason   = "conventional filename"
	extensionReason  = "extension"
	manpageExtension = "man"
)

var (
	cFamily        Set = NewSet(C, CPP, ObjectiveC)
	jsFamily       Set = NewSet(JavaScript, TypeScript, JSX, TSX)
	tsFamily       Set = NewSet(TypeScript, TSX)
	jsxFamily      Set = NewSet(JSX, TSX)
	shellFamily    Set = NewSet(Shell, Bash, Zsh)
	schemeFamily   Set = NewSet(Scheme, Racket)
	templateFamily Set = NewSet(Jinja, Twig)
)

// AnalyzePath examines only the final path component, with either path separator.
// Filenames are case-sensitive; extensions are case-insensitive.
func AnalyzePath(path string) Context {
	const maxPathBytes = 64 * 1024
	if len(path) > maxPathBytes {
		return Context{}
	}
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		path = path[i+1:]
	}
	switch path {
	case "Gemfile", "Rakefile", "Guardfile", "Vagrantfile", "Brewfile", "Podfile", "Capfile", "Appraisals", "Dangerfile", "Steepfile", "Snapfile", ".pryrc", ".simplecov":
		return Context{Candidates: NewSet(Ruby), Reason: filenameReason}
	case ".bashrc", ".bash_profile", ".bash_aliases", ".bash_logout", ".bash_functions", "bashrc", "bash_profile", "bash_aliases", "bash_logout":
		return Context{Candidates: NewSet(Bash), Reason: filenameReason}
	case ".zshrc", ".zprofile", ".zshenv", ".zlogin", ".zlogout", "zshrc", "zprofile", "zshenv", "zlogin", "zlogout":
		return Context{Candidates: NewSet(Zsh), Reason: filenameReason}
	case ".profile", "profile", ".xinitrc", "xinitrc", ".xsession", "xsession", ".envrc", ".flaskenv", ".kshrc", "kshrc", ".cshrc", "cshrc", ".login", "login", "PKGBUILD", ".tmux.conf", "tmux.conf":
		return Context{Candidates: NewSet(Shell), Reason: filenameReason}
	case "cpanfile", "Rexfile", ".latexmkrc", "latexmkrc":
		return Context{Candidates: NewSet(Perl), Reason: filenameReason}
	case "SConstruct", "SConscript", ".gclient", "DEPS":
		return Context{Candidates: NewSet(Python), Reason: filenameReason}
	case ".luacheckrc", ".busted":
		return Context{Candidates: NewSet(Lua), Reason: filenameReason}
	}
	context := Context{Candidates: lookupPath(filenameRegistry[:], path), Reason: filenameReason}
	if context.Candidates.Empty() {
		context = extensionContext(path)
	}
	if context.Candidates.Len() > 1 {
		context.heuristic = pathHeuristic(path)
	}
	return context
}

func extensionContext(path string) Context {
	if strings.HasSuffix(path, ".C") {
		return Context{Candidates: NewSet(CPP), Reason: extensionReason}
	}
	if strings.HasSuffix(path, ".H") {
		return Context{Candidates: cFamily, Reason: extensionReason}
	}
	path = strings.ToLower(path)
	if strings.HasSuffix(path, ".h.in") {
		return Context{Candidates: cFamily, Reason: extensionReason}
	}
	if strings.HasSuffix(path, ".js.frag") {
		return Context{Candidates: NewSet(JavaScript), Reason: extensionReason}
	}
	if strings.HasSuffix(path, ".dll.config") || strings.HasSuffix(path, ".exe.config") {
		return Context{Candidates: NewSet(XML), Reason: extensionReason}
	}
	i := strings.LastIndexByte(path, '.')
	for dot := strings.IndexByte(path, '.'); dot >= 0 && dot < i; {
		if candidates := lookupPath(extensionRegistry[:], path[dot:]); !candidates.Empty() {
			return Context{Candidates: candidates, Reason: extensionReason}
		}
		next := strings.IndexByte(path[dot+1:], '.')
		if next < 0 {
			break
		}
		dot += next + 1
	}
	if i < 0 {
		return Context{}
	}
	s := extensionCandidates(path[i:])
	if s.Empty() && manpageName(path) {
		s = NewSet(Roff, RoffManpage)
	}
	if s.Empty() {
		return Context{}
	}
	return Context{Candidates: s, Reason: extensionReason}
}

func manpageName(path string) bool {
	path = strings.TrimSuffix(path, ".in")
	i := strings.LastIndexByte(path, '.')
	if i < 0 || i == len(path)-1 {
		return false
	}
	ext := path[i+1:]
	if ext == "0p" || ext == "n" || ext == manpageExtension || ext == "mdoc" {
		return true
	}
	if ext[0] < '1' || ext[0] > '9' || len(ext) > 1 && ext[1] >= '0' && ext[1] <= '9' {
		return false
	}
	for _, c := range ext[1:] {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func extensionCandidates(extension string) Set {
	switch extension {
	case ".jsx":
		return NewSet(JSX)
	case ".tsx":
		return NewSet(TSX, XML)
	case ".sh", ".command":
		return shellFamily
	case ".zsh-theme":
		return NewSet(Zsh)
	case ".psql", ".dml":
		return NewSet(SQL)
	case ".h", ".re":
		return cFamily.Union(lookupPath(extensionRegistry[:], extension))
	}
	return lookupPath(extensionRegistry[:], extension)
}
