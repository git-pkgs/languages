package languages

import "strings"

const (
	filenameReason      = "conventional filename"
	extensionReason     = "extension"
	cFamily         Set = 1<<C | 1<<CPP | 1<<ObjectiveC
	jsFamily        Set = 1<<JavaScript | 1<<TypeScript | 1<<JSX | 1<<TSX
	tsFamily        Set = 1<<TypeScript | 1<<TSX
	jsxFamily       Set = 1<<JSX | 1<<TSX
	shellFamily     Set = 1<<Shell | 1<<Bash | 1<<Zsh
	schemeFamily    Set = 1<<Scheme | 1<<Racket
	templateFamily  Set = 1<<Jinja | 1<<Twig
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
	case "Gemfile", "Rakefile", "Guardfile", "Vagrantfile", "Brewfile", "Podfile", "Capfile", "Appraisals", "Dangerfile", "Steepfile", "Snapfile", ".pryrc", ".simplecov":
		return Context{1 << Ruby, filenameReason}
	case ".bashrc", ".bash_profile", ".bash_aliases", ".bash_logout", ".bash_functions", "bashrc", "bash_profile", "bash_aliases", "bash_logout":
		return Context{1 << Bash, filenameReason}
	case ".zshrc", ".zprofile", ".zshenv", ".zlogin", ".zlogout", "zshrc", "zprofile", "zshenv", "zlogin", "zlogout":
		return Context{1 << Zsh, filenameReason}
	case ".profile", "profile", ".xinitrc", "xinitrc", ".xsession", "xsession", ".envrc", ".flaskenv", ".kshrc", "kshrc", ".cshrc", "cshrc", ".login", "login", "PKGBUILD", ".tmux.conf", "tmux.conf":
		return Context{1 << Shell, filenameReason}
	case "cpanfile", "Rexfile", ".latexmkrc", "latexmkrc":
		return Context{1 << Perl, filenameReason}
	case "SConstruct", "SConscript", ".gclient", "DEPS":
		return Context{1 << Python, filenameReason}
	case ".luacheckrc", ".busted":
		return Context{1 << Lua, filenameReason}
	}
	if strings.HasSuffix(path, ".h.in") {
		return Context{cFamily, extensionReason}
	}
	if strings.HasSuffix(path, ".js.frag") {
		return Context{1 << JavaScript, extensionReason}
	}
	if strings.HasSuffix(path, ".dll.config") || strings.HasSuffix(path, ".exe.config") {
		return Context{1 << XML, extensionReason}
	}
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return Context{}
	}
	var s Set
	switch path[i:] {
	case ".py", ".pyw", ".pyi", ".gyp", ".gypi":
		s = 1 << Python
	case ".rb", ".rake", ".rbi", ".jbuilder", ".rabl", ".gemspec", ".ru", ".builder", ".podspec":
		s = 1 << Ruby
	case ".go":
		s = 1 << Go
	case ".rs":
		s = 1 << Rust
	case ".java", ".jsh":
		s = 1 << Java
	case ".c":
		s = 1 << C
	case ".cc", ".cpp", ".cxx", ".hpp", ".C", ".hh", ".hxx", ".h++", ".cp", ".c++", ".inl", ".ipp", ".tpp", ".txx", ".ixx", ".cppm", ".ino":
		s = 1 << CPP
	case ".h", ".H", ".re":
		s = cFamily
	case ".m":
		s = 1<<ObjectiveC | 1<<MATLAB
	case ".js", ".mjs", ".cjs", ".es", ".es6", ".xsjs", ".xsjslib", ".jscad", ".jsb":
		s = 1 << JavaScript
	case ".ts", ".mts", ".cts":
		s = 1 << TypeScript
	case ".jsx":
		s = 1 << JSX
	case ".tsx":
		s = 1 << TSX
	case ".sh", ".command":
		s = shellFamily
	case ".bash":
		s = 1 << Bash
	case ".zsh", ".zsh-theme":
		s = 1 << Zsh
	case ".fish":
		s = 1 << Fish
	case ".pl":
		s = 1<<Perl | 1<<Raku | 1<<Prolog
	case ".pm":
		s = 1<<Perl | 1<<Raku
	case ".al", ".ph", ".psgi":
		s = 1 << Perl
	case ".pro", ".prolog", ".yap":
		s = 1 << Prolog
	case ".raku", ".rakumod", ".p6", ".pm6", ".pl6":
		s = 1 << Raku
	case ".lisp", ".cl", ".lsp":
		s = 1 << CommonLisp
	case ".scm", ".ss":
		s = schemeFamily
	case ".sps", ".sld", ".sls", ".sch":
		s = 1 << Scheme
	case ".rkt", ".scrbl":
		s = 1 << Racket
	case ".clj", ".cljs", ".cljc", ".hic", ".cl2", ".boot":
		s = 1 << Clojure
	case ".php":
		s = 1 << PHP
	case ".lua", ".rockspec", ".pd_lua":
		s = 1 << Lua
	case ".cs", ".csx", ".cake", ".linq":
		s = 1 << CSharp
	case ".html", ".htm":
		s = 1 << HTML
	case ".xml", ".xsl", ".svg", ".xsd", ".xslt", ".xaml", ".axaml", ".gpx", ".csproj", ".vbproj", ".fsproj", ".vcxproj", ".wixproj", ".props", ".targets", ".proj", ".pubxml", ".resx", ".slnx", ".xmp", ".mjml", ".gmx", ".icls", ".ux":
		s = 1 << XML
	case ".jinja", ".j2", ".jinja2":
		s = 1 << Jinja
	case ".twig":
		s = 1 << Twig
	case ".erb":
		s = 1 << ERB
	case ".sql", ".mysql", ".pgsql", ".psql", ".ddl", ".dml":
		s = 1 << SQL
	}
	if s == 0 {
		return Context{}
	}
	return Context{s, extensionReason}
}
