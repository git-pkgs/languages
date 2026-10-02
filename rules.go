package languages

const (
	functionPrefix   = "function "
	prologPrefix     = "~prolog"
	typedPrefix      = "~typed"
	constPrefix      = "const "
	funcPrefix       = "func "
	importPrefix     = "import "
	rubyRequireID    = "ruby.require"
	rubyDefID        = "ruby.def"
	rubyModuleID     = "ruby.module"
	pythonImportID   = "python.import"
	goFuncID         = "go.func"
	swiftFuncID      = "swift.func"
	goSwiftFuncID    = "go-swift.func"
	javaPackageID    = "java.package"
	javaImportID     = "java.import"
	matlabFunctionID = "matlab.function"
	racketLangID     = "racket.lang"
	goPackageID      = "go.package"
	htmlRootID       = "html.root"
	htmlDoctypeID    = "html.doctype"
	phpOpenID        = "php.open"
	hackOpenID       = "hack.open"
	cIncludeID       = "c.include"
	goTemplateID     = "go-template.action"
)

type rule struct {
	id, description          string
	languages                Set
	weight                   uint8
	prefix, contains, suffix string
}

var ruleStarts = func() (starts [256][2]uint64) {
	for i := range rules {
		first := rules[i].prefix[0]
		for b := range starts {
			if b >= 0x80 || first == '~' || byte(b)|0x20 == first|0x20 {
				starts[b][i/wordBits] |= 1 << uint(i%wordBits)
			}
		}
	}
	return starts
}()

// Each rule contributes once. declaredWeight is reserved for interpreter rules.
var rules = [...]rule{
	{"roff.title", "manpage title", NewSet(RoffManpage), strongWeight, ".TH ", "", ""},
	{"roff.date", "mdoc date", NewSet(RoffManpage), strongWeight, ".Dd ", "", ""},
	{"ruby.shebang", "shebang indicates Ruby", NewSet(Ruby), declaredWeight, "@ruby", "", ""},
	{"python.shebang", "shebang indicates Python", NewSet(Python), declaredWeight, "@python", "", ""},
	{"shell.shebang", "shebang indicates sh", NewSet(Shell), declaredWeight, "@sh", "", ""},
	{"bash.shebang", "shebang indicates Bash", NewSet(Bash), declaredWeight, "@bash", "", ""},
	{"zsh.shebang", "shebang indicates Zsh", NewSet(Zsh), declaredWeight, "@zsh", "", ""},
	{"fish.shebang", "shebang indicates Fish", NewSet(Fish), declaredWeight, "@fish", "", ""},
	{"perl.shebang", "shebang indicates Perl", NewSet(Perl), declaredWeight, "@perl", "", ""},
	{"raku.shebang", "shebang indicates Raku", NewSet(Raku), declaredWeight, "@raku", "", ""},
	{"node.shebang", "shebang indicates the JavaScript family", jsFamily, declaredWeight, "@node", "", ""},
	{"php.shebang", "shebang indicates PHP", NewSet(PHP), declaredWeight, "@php", "", ""},
	{"lua.shebang", "shebang indicates Lua", NewSet(Lua), declaredWeight, "@lua", "", ""},
	{"racket.shebang", "shebang indicates Racket", NewSet(Racket), declaredWeight, "@racket", "", ""},
	{"ruby.frozen", "Ruby frozen_string_literal pragma", NewSet(Ruby), 7, "#", "frozen_string_literal:", ""},
	{rubyRequireID, "Ruby/Sieve require statement", NewSet(Ruby, Sieve), 5, "require ", "", ""},
	{"ruby.require-relative", "Ruby require_relative statement", NewSet(Ruby), 6, "require_relative ", "", ""},
	{rubyModuleID, "Ruby-style module declaration", NewSet(Ruby), 3, "module ", "", ""},
	{rubyDefID, "Ruby-style method declaration", NewSet(Ruby), 4, "def ", "", ""},
	{"ruby.end", "end terminator", NewSet(Ruby, Lua, MATLAB), 1, "end", "", "end"},
	{"ruby.attr", "Ruby attribute declaration", NewSet(Ruby), 6, "attr_", ":", ""},
	{"python.def", "Python function declaration", NewSet(Python), 6, "def ", "(", ":"},
	{"python.async-def", "Python async function declaration", NewSet(Python), 6, "async def ", "(", ":"},
	{"python.from", "Python from/import statement", NewSet(Python), 5, "from ", " import ", ""},
	{pythonImportID, "Python-style import statement", NewSet(Python), 3, importPrefix, "", ""},
	{"python.class", "Python class declaration", NewSet(Python), 5, "class ", "", ":"},
	{"python.main", "Python __name__ guard", NewSet(Python), 6, "if __name__", "==", ":"},
	{goPackageID, "package declaration without semicolon", NewSet(Go, Java), 3, "package ", "", ""},
	{goFuncID, "Go function declaration", NewSet(Go), 6, funcPrefix, "(", ""},
	{"go.import", "Go import block", NewSet(Go), 6, "import (", "", ""},
	{"go.type", "Go struct type declaration", NewSet(Go), 6, "type ", " struct", ""},
	{"rust.use", "Rust/Perl/Raku use path", NewSet(Rust, Perl, Raku), 5, "use ", "::", ";"},
	{"rust.fn", "Rust function declaration", NewSet(Rust), 6, "fn ", "(", ""},
	{"rust.pub-fn", "Rust public function declaration", NewSet(Rust), 6, "pub fn ", "(", ""},
	{"rust.impl", "Rust impl block", NewSet(Rust), 5, "impl ", "{", ""},
	{"rust.derive", "Rust derive attribute", NewSet(Rust), 6, "#[derive(", "", ""},
	{javaPackageID, "Package declaration with semicolon", NewSet(Java, Perl, X10, Groovy, Scala, Kotlin), 5, "package ", "", ";"},
	{javaImportID, "Java-family import path", NewSet(Java, X10, Groovy, Scala, Kotlin), 5, importPrefix, ".", ";"},
	{"java.class", "public class declaration", NewSet(Java, CSharp), 4, "public class ", "", ""},
	{"java.main", "Java main method signature", NewSet(Java), 7, "public static void main(", "String", ""},
	{cIncludeID, "C-family include directive", cFamily, 5, "#include", "", ""},
	{"c.guard", "C-family conditional preprocessor directive", cFamily, 3, "#ifndef ", "", ""},
	{"c.typedef", "C-family typedef", cFamily, 4, "typedef ", "", ""},
	{"c.main", "C-family main function", cFamily, 4, "int main(", "", ""},
	{"cpp.namespace", "Namespace block", NewSet(CPP, CSharp, PHP, Hack), 6, "namespace ", "{", ""},
	{"cpp.using", "C++ using namespace directive", NewSet(CPP), 6, "using namespace ", "", ";"},
	{"cpp.template", "C++ template declaration", NewSet(CPP), 7, "template", "<", ""},
	{"objc.import", "Objective-C import directive", NewSet(ObjectiveC), 6, "#import ", "", ""},
	{"objc.interface", "Objective-C interface declaration", NewSet(ObjectiveC), 7, "@interface ", "", ""},
	{"objc.implementation", "Objective-C implementation declaration", NewSet(ObjectiveC), 7, "@implementation ", "", ""},
	{matlabFunctionID, "MATLAB function with output assignment", NewSet(MATLAB), 6, functionPrefix, "=", ""},
	{"matlab.class", "MATLAB classdef declaration", NewSet(MATLAB), 6, "classdef ", "", ""},
	{"js.function", "JavaScript-family function declaration", jsFamily, 5, functionPrefix, "(", ""},
	{"js.const", "JavaScript-family const declaration", jsFamily, 3, constPrefix, "=", ""},
	{"js.import", "JavaScript-family import/from statement", jsFamily, 5, importPrefix, " from ", ""},
	{"js.export", "JavaScript-family default export", jsFamily, 5, "export default ", "", ""},
	{"js.module", "CommonJS module export", jsFamily, 6, "module.exports", "=", ""},
	{"ts.interface", "TypeScript interface declaration", tsFamily, 6, "interface ", "{", ""},
	{"ts.export-interface", "TypeScript exported interface", tsFamily, 6, "export interface ", "{", ""},
	{"ts.type", "TypeScript exported type alias", tsFamily, 6, "export type ", "=", ""},
	{"jsx.return", "JSX-style markup in return expression", jsxFamily, 6, "return ", "<", ">;"},
	{"shell.if", "shell if/then construct", shellFamily, 5, "if ", "; then", ""},
	{"shell.case", "shell case/in construct", shellFamily, 5, "case ", "", " in"},
	{"shell.export", "shell export assignment", shellFamily, 3, "export ", "=", ""},
	{"shell.fi", "shell fi terminator", shellFamily, 3, "fi", "", "fi"},
	{"shell.extended-test", "Bash/Zsh extended test", NewSet(Bash, Zsh), 5, "[[ ", " ]]", ""},
	{"fish.set", "Shell/Fish set command", shellFamily.Union(NewSet(Fish)), 5, "set -", "", ""},
	{"perl.strict", "Perl strict pragma", NewSet(Perl), 6, "use strict", "", ";"},
	{"perl.warnings", "Perl warnings pragma", NewSet(Perl), 6, "use warnings", "", ";"},
	{"perl.sub", "Perl/Raku subroutine declaration", NewSet(Perl, Raku), 4, "sub ", "{", ""},
	{"raku.version", "Raku language directive", NewSet(Raku), 9, "use v6", "", ";"},
	{"raku.unit", "Raku unit module declaration", NewSet(Raku), 7, "unit module ", "", ";"},
	{"lisp.defun", "Lisp defun form", NewSet(CommonLisp, EmacsLisp, LFE), 6, "(defun ", "", ""},
	{"lisp.package", "Common Lisp package form", NewSet(CommonLisp), 6, "(in-package ", "", ""},
	{"scheme.define", "Scheme-family define form", schemeFamily, 5, "(define ", "", ""},
	{racketLangID, "Racket #lang directive", NewSet(Racket), 9, "#lang ", "", ""},
	{"clojure.ns", "Clojure namespace form", NewSet(Clojure), 6, "(ns ", "", ""},
	{"clojure.defn", "Clojure/Hy defn form", NewSet(Clojure, Hy), 6, "(defn ", "", ""},
	{phpOpenID, "PHP opening tag", NewSet(PHP), 9, "<?php", "", ""},
	{"lua.local-function", "Lua local function declaration", NewSet(Lua), 6, "local function ", "(", ""},
	{"lua.require", "Lua local require assignment", NewSet(Lua), 5, "local ", "require", ""},
	{"csharp.using", "C#-family using directive", NewSet(CSharp, Beef, Uno), 5, "using ", ".", ";"},
	{"csharp.namespace", "C# file-scoped namespace", NewSet(CSharp), 6, "namespace ", "", ";"},
	{"csharp.system", "C#-family System using directive", NewSet(CSharp, Beef, Uno), 5, "using System;", "", ""},
	{htmlDoctypeID, "HTML doctype declaration", NewSet(HTML), 9, "<!DOCTYPE html", "", ""},
	{htmlRootID, "HTML root tag", NewSet(HTML), 6, "<html", "", ""},
	{"xml.declaration", "XML declaration", NewSet(XML), 9, "<?xml ", "", ""},
	{"template.block", "Jinja/Twig block directive", templateFamily, 5, "{%", "block ", "%}"},
	{"template.extends", "Jinja/Twig extends directive", templateFamily, 6, "{%", "extends ", "%}"},
	{"erb.tag", "ERB-style expression tag", NewSet(ERB), 3, "<%=", "", ""},
	{"erb.ruby", "Ruby syntax inside template tag", NewSet(ERB), 6, "<%", " do ", ""},
	{"sql.select", "SQL SELECT/FROM statement", NewSet(SQL), 5, "SELECT ", " FROM ", ""},
	{"sql.create", "SQL CREATE TABLE statement", NewSet(SQL), 6, "CREATE TABLE ", "", ""},
	{"prolog.module", "Prolog module directive", NewSet(Prolog), 7, ":- module(", "", ""},
	{"prolog.import", "Prolog use_module directive", NewSet(Prolog), 7, ":- use_module(", "", ""},
	{"prolog.rule", "Prolog-style rule clause", NewSet(Prolog), 5, prologPrefix, "", ""},
	{"prolog.shebang", "shebang indicates SWI-Prolog", NewSet(Prolog), declaredWeight, "@swipl", "", ""},
	{"prolog.dynamic", "Prolog dynamic directive", NewSet(Prolog), 6, ":- dynamic ", "", "."},
	{"ts.binding", "Typed variable binding", NewSet(TypeScript, TSX, Swift, Kotlin, Rust, Zig, Cairo, Move, Pony), 3, typedPrefix, "", ""},
	{"ts.export-enum", "TypeScript exported enumeration", tsFamily, 9, "export enum ", "{", ""},
	{"cython.cdef", "Cython C declaration", NewSet(Cython), 6, "cdef ", "", ""},
	{"cython.cimport", "Cython C import", NewSet(Cython), 6, "cimport ", "", ""},
	{"cython.from", "Cython C import path", NewSet(Cython), 6, "from ", " cimport ", ""},
	{"cython.pragma", "Cython compiler directive", NewSet(Cython), 9, "#cython:", "", ""},
	{"cython.spaced-pragma", "Cython compiler directive", NewSet(Cython), 9, "# cython:", "", ""},
	{hackOpenID, "Hack opening tag", NewSet(Hack), 9, "<?hh", "", ""},
	{swiftFuncID, "Swift function signature", NewSet(Swift), 6, funcPrefix, "(", ""},
	{"golo.function", "Golo function assignment", NewSet(Golo), 6, functionPrefix, "= |", ""},
	{"sieve.header", "Sieve header condition", NewSet(Sieve), 6, "if header ", ":", ""},
	{"ruby.require-path", "Ruby File require expression", NewSet(Ruby), 6, "require File.", "(", ""},
	{"csharp.nullable", "C# nullable directive", NewSet(CSharp), 6, "#nullable ", "", ""},
	{"js.console", "JavaScript-family console call", jsFamily, 3, "console.", "(", ""},
	{"perl.version", "Perl version directive", NewSet(Perl), 9, "use 5.", "", ";"},
	{"perl.v-version", "Perl version directive", NewSet(Perl), 9, "use v5", "", ";"},
	{"raku.my-class", "Raku lexical class", NewSet(Raku), 6, "my class ", "", ""},
	{"raku.my-role", "Raku lexical role", NewSet(Raku), 6, "my role ", "", ""},
	{"raku.attribute", "Raku private attribute", NewSet(Raku), 6, "has $!", "", ";"},
	{"raku.multi", "Raku multi routine", NewSet(Raku), 6, "multi ", "(", ""},
	{"raku.module", "Raku module declaration", NewSet(Raku), 5, "module ", "", ";"},
	{"cpp.public", "C++ public access label", NewSet(CPP), 6, "public:", "", ""},
	{"cpp.protected", "C++ protected access label", NewSet(CPP), 6, "protected:", "", ""},
	{"cpp.constexpr", "C++ constexpr declaration", NewSet(CPP), 6, "constexpr ", "", ""},
	{"percent.comment", "MATLAB/Prolog comment", NewSet(MATLAB, Prolog), 3, "%", "", ""},
	{goSwiftFuncID, "Go/Swift function declaration without parameters or return type", NewSet(Go, Swift), 6, funcPrefix, "(", ""},
	{goTemplateID, "Go template action", NewSet(GoTemplate), strongWeight, "@go-template", "", ""},
}
