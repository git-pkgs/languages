package languages_test

import (
	"testing"

	"github.com/git-pkgs/languages"
)

func TestDetectFilenameConventions(t *testing.T) {
	for language, names := range map[languages.Language][]string{
		languages.Ruby:       {"Brewfile", "Podfile", "Capfile", "Appraisals", "Dangerfile", "Steepfile", "Snapfile", ".pryrc", ".simplecov", "app.rbi", "index.json.jbuilder", "view.rabl", "app.gemspec", "config.ru", "feed.builder", "lib.podspec"},
		languages.Bash:       {".bash_aliases", ".bash_logout", ".bash_functions", "bashrc", "bash_profile", "bash_aliases", "bash_logout"},
		languages.Zsh:        {".zshenv", ".zlogin", ".zlogout", "zshrc", "zprofile", "zshenv", "zlogin", "zlogout", "theme.zsh-theme"},
		languages.Shell:      {".profile", "profile", ".xinitrc", "xinitrc", ".xsession", "xsession", ".envrc", ".flaskenv", ".kshrc", "kshrc", ".cshrc", "cshrc", ".login", "login", "PKGBUILD", ".tmux.conf", "tmux.conf"},
		languages.Perl:       {"cpanfile", "Rexfile", ".latexmkrc", "latexmkrc", "header.ph", "app.psgi"},
		languages.Python:     {"SConstruct", "SConscript", ".gclient", "DEPS", "module.pyi", "binding.gyp", "standalone.gypi"},
		languages.Lua:        {".luacheckrc", ".busted", "lib.rockspec", "widget.pd_lua"},
		languages.CPP:        {"header.hxx", "header.h++", "main.c++", "math.inl", "math.ipp", "math.tpp", "array.txx", "module.ixx", "module.cppm", "sketch.ino"},
		languages.JavaScript: {"intro.js.frag", "module.es6", "app.xsjs", "math.xsjslib", "logo.jscad", "build.jsb"},
		languages.Raku:       {"Role.pm6", "app.pl6"},
		languages.Prolog:     {"queues.yap"},
		languages.Scheme:     {"main.sps", "library.sld"},
		languages.Racket:     {"manual.scrbl"},
		languages.Clojure:    {"page.hic", "test.cl2"},
		languages.CSharp:     {"script.csx", "query.linq"},
		languages.Java:       {"script.jsh"},
		languages.XML:        {"lib.dll.config", "app.exe.config", "schema.xsd", "view.xaml", "view.axaml", "route.gpx", "app.csproj", "app.vbproj", "app.fsproj", "app.vcxproj", "app.wixproj", "build.props", "build.targets", "build.proj", "publish.pubxml", "strings.resx", "app.slnx", "data.xmp", "mail.mjml", "project.gmx", "theme.icls", "view.ux"},
		languages.XSLT:       {"style.xslt"},
		languages.PLpgSQL:    {"schema.pgsql"},
		languages.SQL:        {"schema.psql", "schema.dml"},
	} {
		for _, name := range names {
			t.Run(name, func(t *testing.T) {
				got := languages.Detect("src/"+name, []byte("\n"))
				if got.Language != language || got.Confidence != languages.Low {
					t.Fatal(got, language)
				}
			})
		}
	}
}

func TestDetectAmbiguousExtensions(t *testing.T) {
	for name, want := range map[string]languages.Set{
		"config.h.in":   languages.NewSet(languages.C, languages.CPP, languages.ObjectiveC),
		"header.H":      languages.NewSet(languages.C, languages.CPP, languages.ObjectiveC),
		"scanner.re":    languages.NewSet(languages.C, languages.CPP, languages.ObjectiveC, languages.Reason),
		"module.es":     languages.NewSet(languages.JavaScript, languages.Erlang),
		"main.cp":       languages.NewSet(languages.CPP, languages.ComponentPascal),
		"main.lsp":      languages.NewSet(languages.CommonLisp, languages.NewLisp),
		"schema.ddl":    languages.NewSet(languages.SQL, languages.PLSQL),
		"build.command": languages.NewSet(languages.Shell, languages.Bash, languages.Zsh),
	} {
		if got := languages.Detect(name, nil); got.Language != languages.Unknown || got.Candidates != want {
			t.Fatal(name, got)
		}
	}
}
