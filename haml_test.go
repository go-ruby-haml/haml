package haml

import (
	"strings"
	"testing"
)

// compileGolden asserts the compiled Ruby source for a template equals want.
// These are deterministic and interpreter-free, so they alone keep coverage at
// 100% on lanes without ruby.
func compileGolden(t *testing.T, tpl, want string) {
	t.Helper()
	got, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatalf("Compile(%q): %v", tpl, err)
	}
	if got != want {
		t.Errorf("Compile(%q)\n got=%q\nwant=%q", tpl, got, want)
	}
}

const head = "_hamlout = ::String.new\n"
const tail = "_hamlout\n"

func TestCompileElements(t *testing.T) {
	compileGolden(t, "%p hello", head+`_hamlout << "<p>hello</p>\n"`+"\n"+tail)
	compileGolden(t, "%p", head+`_hamlout << "<p></p>\n"`+"\n"+tail)
	compileGolden(t, ".foo", head+`_hamlout << "<div class=\"foo\"></div>\n"`+"\n"+tail)
	compileGolden(t, "#bar", head+`_hamlout << "<div id=\"bar\"></div>\n"`+"\n"+tail)
	compileGolden(t, ".foo.bar#baz", head+`_hamlout << "<div class=\"foo bar\" id=\"baz\"></div>\n"`+"\n"+tail)
	compileGolden(t, "%br", head+`_hamlout << "<br>\n"`+"\n"+tail)
	compileGolden(t, "%meta{charset: 'utf-8'}/", head+`_hamlout << "<meta charset=\"utf-8\">\n"`+"\n"+tail)
	compileGolden(t, "%div/", head+`_hamlout << "<div>\n"`+"\n"+tail)
}

func TestCompileNesting(t *testing.T) {
	compileGolden(t, "%p\n  %b x", head+`_hamlout << "<p>\n<b>x</b>\n</p>\n"`+"\n"+tail)
	compileGolden(t, "%ul\n  %li a\n  %li b",
		head+`_hamlout << "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"`+"\n"+tail)
	compileGolden(t, "%tag\n  %inner\n    deep",
		head+`_hamlout << "<tag>\n<inner>\ndeep\n</inner>\n</tag>\n"`+"\n"+tail)
}

func TestCompileText(t *testing.T) {
	compileGolden(t, "text at root", head+`_hamlout << "text at root\n"`+"\n"+tail)
	compileGolden(t, "line1\nline2", head+`_hamlout << "line1\nline2\n"`+"\n"+tail)
	compileGolden(t, "\\%p literal", head+`_hamlout << "%p literal\n"`+"\n"+tail)
	compileGolden(t, "", head+tail) // empty template
	compileGolden(t, "\n\n", head+tail)
}

func TestCompileExpr(t *testing.T) {
	compileGolden(t, "= 1 + 2",
		head+`_hamlout << ::Haml::Util.escape_html((1 + 2).to_s); _hamlout << "\n"`+"\n"+tail)
	compileGolden(t, "!= raw",
		head+`_hamlout << (raw).to_s; _hamlout << "\n"`+"\n"+tail)
	compileGolden(t, "%p= x",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((x).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "%p!= x",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << (x).to_s`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "%p&= x",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((x).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "%p~ x",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((x).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "~ x",
		head+`_hamlout << ::Haml::Util.escape_html((x).to_s); _hamlout << "\n"`+"\n"+tail)
	compileGolden(t, "!= raw",
		head+`_hamlout << (raw).to_s; _hamlout << "\n"`+"\n"+tail)
	compileGolden(t, "&= x",
		head+`_hamlout << ::Haml::Util.escape_html((x).to_s); _hamlout << "\n"`+"\n"+tail)
}

// TestCompilePlainTextMarkers covers the "&"/"!" plain-text escape markers, the
// default-escaped interpolation, and the literal fall-through for "&x"/bare "&".
func TestCompilePlainTextMarkers(t *testing.T) {
	// "! text" — plain text, interpolation NOT escaped.
	compileGolden(t, "! raw", head+`_hamlout << "raw\n"`+"\n"+tail)
	compileGolden(t, "! #{name}",
		head+`_hamlout << (name).to_s`+"\n"+`_hamlout << "\n"`+"\n"+tail)
	// "& text" — plain text, interpolation escaped (the default).
	compileGolden(t, "& #{name}",
		head+`_hamlout << ::Haml::Util.escape_html((name).to_s)`+"\n"+`_hamlout << "\n"`+"\n"+tail)
	// "&x"/"!x" and a bare "&" are literal text, not markers.
	compileGolden(t, "&x", head+`_hamlout << "&x\n"`+"\n"+tail)
	compileGolden(t, "!x", head+`_hamlout << "!x\n"`+"\n"+tail)
	compileGolden(t, "&", head+`_hamlout << "&\n"`+"\n"+tail)
	compileGolden(t, "!", head+`_hamlout << "!\n"`+"\n"+tail)
	// Inline "&"/"!" markers on an element.
	compileGolden(t, "%p& #{name}",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((name).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "%p! #{name}",
		head+`_hamlout << "<p>"`+"\n"+`_hamlout << (name).to_s`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
}

func TestCompileControl(t *testing.T) {
	compileGolden(t, "- x = 5\n%p= x",
		head+"x = 5\n"+`_hamlout << "<p>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((x).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "- if flag\n  %p yes\n- else\n  %p no",
		head+"if flag\n"+`_hamlout << "<p>yes</p>\n"`+"\n"+"else\n"+`_hamlout << "<p>no</p>\n"`+"\n"+"end\n"+tail)
	compileGolden(t, "- 3.times do |i|\n  %span= i",
		head+"3.times do |i|\n"+`_hamlout << "<span>"`+"\n"+`_hamlout << ::Haml::Util.escape_html((i).to_s)`+"\n"+`_hamlout << "</span>\n"`+"\n"+"end\n"+tail)
	// case/when chain.
	compileGolden(t, "- case n\n- when 1\n  %p one\n- else\n  %p other",
		head+"case n\n"+"when 1\n"+`_hamlout << "<p>one</p>\n"`+"\n"+"else\n"+`_hamlout << "<p>other</p>\n"`+"\n"+"end\n"+tail)
	// begin/rescue.
	compileGolden(t, "- begin\n  %p a\n- rescue\n  %p b",
		head+"begin\n"+`_hamlout << "<p>a</p>\n"`+"\n"+"rescue\n"+`_hamlout << "<p>b</p>\n"`+"\n"+"end\n"+tail)
	// silent comment discarded.
	compileGolden(t, "-# nothing\n%p x", head+`_hamlout << "<p>x</p>\n"`+"\n"+tail)
}

func TestCompileComments(t *testing.T) {
	compileGolden(t, "/ comment", head+`_hamlout << "<!-- comment -->\n"`+"\n"+tail)
	compileGolden(t, "/", head+`_hamlout << "<!--\n-->\n"`+"\n"+tail)
	compileGolden(t, "/\n  %p x",
		head+`_hamlout << "<!--\n<p>x</p>\n-->\n"`+"\n"+tail)
	compileGolden(t, "/[if IE]\n  %p ie",
		head+`_hamlout << "<!--[if IE]>\n<p>ie</p>\n<![endif]-->\n"`+"\n"+tail)
	compileGolden(t, "/[if IE] text",
		head+`_hamlout << "<!--[if IE]> text <![endif]-->\n"`+"\n"+tail)
}

func TestCompileDoctype(t *testing.T) {
	// html5 (default): every version collapses to <!DOCTYPE html>; named types
	// resolve against the HTML 4.01 table; XML emits nothing at all.
	compileGolden(t, "!!!", head+`_hamlout << "<!DOCTYPE html>\n"`+"\n"+tail)
	compileGolden(t, "!!! 5", head+`_hamlout << "<!DOCTYPE html>\n"`+"\n"+tail)
	compileGolden(t, "!!! 1.1", head+`_hamlout << "<!DOCTYPE html>\n"`+"\n"+tail)
	compileGolden(t, "!!! Strict",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://www.w3.org/TR/html4/strict.dtd\">\n"`+"\n"+tail)
	compileGolden(t, "!!! Frameset",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01 Frameset//EN\" \"http://www.w3.org/TR/html4/frameset.dtd\">\n"`+"\n"+tail)
	compileGolden(t, "!!! RDFa",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML+RDFa 1.0//EN\" \"http://www.w3.org/MarkUp/DTD/xhtml-rdfa-1.dtd\">\n"`+"\n"+tail)
	// "!!! XML" in an html format emits nothing (not even a newline); an unknown
	// doctype name (invalid in the html table) is likewise dropped.
	compileGolden(t, "!!! XML", head+tail)
	compileGolden(t, "!!! Basic", head+tail)

	// xhtml: version-sensitive table, self-closing XML prolog, transitional
	// default.
	compileGolden2(t, "!!!", "xhtml",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Transitional//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd\">\n"`+"\n"+tail)
	compileGolden2(t, "!!! 1.1", "xhtml",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.1//EN\" \"http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd\">\n"`+"\n"+tail)
	compileGolden2(t, "!!! XML", "xhtml",
		head+`_hamlout << "<?xml version='1.0' encoding='utf-8' ?>\n"`+"\n"+tail)
	compileGolden2(t, "!!! Mobile", "xhtml",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//WAPFORUM//DTD XHTML Mobile 1.2//EN\" \"http://www.openmobilealliance.org/tech/DTD/xhtml-mobile12.dtd\">\n"`+"\n"+tail)

	// html4: always the transitional doctype.
	compileGolden2(t, "!!!", "html4",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\" \"http://www.w3.org/TR/html4/loose.dtd\">\n"`+"\n"+tail)
	compileGolden2(t, "!!! Strict", "html4",
		head+`_hamlout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://www.w3.org/TR/html4/strict.dtd\">\n"`+"\n"+tail)
}

func TestCompileFilters(t *testing.T) {
	compileGolden(t, ":plain\n  raw <b>", head+`_hamlout << "raw <b>\n"`+"\n"+tail)
	compileGolden(t, ":escaped\n  <b>", head+`_hamlout << "&lt;b&gt;\n"`+"\n"+tail)
	compileGolden(t, ":javascript\n  var x = 1;",
		head+`_hamlout << "<script>\n  var x = 1;\n</script>\n"`+"\n"+tail)
	compileGolden(t, ":css\n  .a { c: red; }",
		head+`_hamlout << "<style>\n  .a { c: red; }\n</style>\n"`+"\n"+tail)
	compileGolden(t, ":preserve\n  a\n  b",
		head+`_hamlout << "a&#x000A;b&#x000A;\n"`+"\n"+tail)
	compileGolden(t, ":ruby\n  x = 1\n  y = 2", head+"x = 1\ny = 2\n"+tail)
	compileGolden(t, ":unknownfilter\n  body", head+`_hamlout << "body\n"`+"\n"+tail)
	// Filter body followed by a dedented sibling.
	compileGolden(t, ":plain\n  a\n%p b",
		head+`_hamlout << "a\n<p>b</p>\n"`+"\n"+tail)
	// Blank line inside then continuing filter body.
	compileGolden(t, ":plain\n  a\n\n  b",
		head+`_hamlout << "a\n\nb\n"`+"\n"+tail)
}

func TestCompileStaticAttrs(t *testing.T) {
	compileGolden(t, "%a{href: 'x'}", head+`_hamlout << "<a href=\"x\"></a>\n"`+"\n"+tail)
	compileGolden(t, "%a(href='x')", head+`_hamlout << "<a href=\"x\"></a>\n"`+"\n"+tail)
	compileGolden(t, "%a.c1{class: 'c2'}", head+`_hamlout << "<a class=\"c1 c2\"></a>\n"`+"\n"+tail)
	compileGolden(t, "%a#i1{id: 'i2'}", head+`_hamlout << "<a id=\"i1_i2\"></a>\n"`+"\n"+tail)
	compileGolden(t, "%input{checked: true}", head+`_hamlout << "<input checked>\n"`+"\n"+tail)
	compileGolden(t, "%input{checked: false}", head+`_hamlout << "<input>\n"`+"\n"+tail)
	compileGolden(t, "%input{disabled: nil}", head+`_hamlout << "<input>\n"`+"\n"+tail)
	compileGolden(t, "%a{href: nil}", head+`_hamlout << "<a href=\"\"></a>\n"`+"\n"+tail)
	compileGolden(t, "%p{a: 1}", head+`_hamlout << "<p a=\"1\"></p>\n"`+"\n"+tail)
	compileGolden(t, "%p{a: 1.5}", head+`_hamlout << "<p a=\"1.5\"></p>\n"`+"\n"+tail)
	compileGolden(t, "%div{data: {foo: 'bar', baz: 'q'}}",
		head+`_hamlout << "<div data-baz=\"q\" data-foo=\"bar\"></div>\n"`+"\n"+tail)
	compileGolden(t, "%p{'data-x' => 'y'}", head+`_hamlout << "<p data-x=\"y\"></p>\n"`+"\n"+tail)
	compileGolden(t, "%p{:sym => 'v'}", head+`_hamlout << "<p sym=\"v\"></p>\n"`+"\n"+tail)
	compileGolden(t, `%input{name: 'q', value: 'a"b'}`,
		head+`_hamlout << "<input name=\"q\" value=\"a&quot;b\">\n"`+"\n"+tail)
	// HTML-style bare boolean attr.
	compileGolden(t, "%input(disabled)", head+`_hamlout << "<input disabled>\n"`+"\n"+tail)
	// HTML-style numeric literal.
	compileGolden(t, "%p(a=1)", head+`_hamlout << "<p a=\"1\"></p>\n"`+"\n"+tail)
	// Duplicate value attr: last wins.
	compileGolden(t, "%p{title: 'a', title: 'b'}", head+`_hamlout << "<p title=\"b\"></p>\n"`+"\n"+tail)
	compileGolden(t, "%p{checked: true, checked: true}", head+`_hamlout << "<p checked></p>\n"`+"\n"+tail)
}

func TestCompileDynamicAttrs(t *testing.T) {
	compileGolden(t, "%p{id: who}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {id: who})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// Dynamic hash with static shorthand class as a separate leading hash so the
	// runtime helper accumulates class across hashes.
	compileGolden(t, "%p.c{id: who}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {class: "c"}, {id: who})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	compileGolden(t, "%p#i{class: cls}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {id: "i"}, {class: cls})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// Interpolated double-quoted attr value forces dynamic.
	compileGolden(t, `%p{title: "x#{y}"}`,
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {title: "x#{y}"})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// data hash with a dynamic value forces dynamic.
	compileGolden(t, "%p{data: {x: v}}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {data: {x: v}})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// HTML-style dynamic value forces dynamic.
	compileGolden(t, "%a(href=url)",
		head+`_hamlout << "<a"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {href=url})`+"\n"+`_hamlout << "></a>\n"`+"\n"+tail)
}

// TestCompileObjectRef covers the "[obj]" object-reference syntax, which always
// compiles to a dynamic Haml::ObjectRef.parse hash, optionally merged with a
// leading static/dynamic attribute hash.
func TestCompileObjectRef(t *testing.T) {
	compileGolden(t, "%li[x]",
		head+`_hamlout << "<li"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", ::Haml::ObjectRef.parse([x]))`+"\n"+`_hamlout << "></li>\n"`+"\n"+tail)
	compileGolden(t, "%div[x, :pre]",
		head+`_hamlout << "<div"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", ::Haml::ObjectRef.parse([x, :pre]))`+"\n"+`_hamlout << "></div>\n"`+"\n"+tail)
	// A fully-literal attribute hash is folded in as a leading hash before the
	// object reference so its class merges ahead of the object-ref class.
	compileGolden(t, "%li[x]{class: 'extra'}",
		head+`_hamlout << "<li"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {class: "extra"}, ::Haml::ObjectRef.parse([x]))`+"\n"+`_hamlout << "></li>\n"`+"\n"+tail)
	// A literal non-class/id attribute folds in via the string-hashrocket form.
	compileGolden(t, "%input[x]{type: 'text', checked: true}",
		head+`_hamlout << "<input"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("html5", {"type" => "text", "checked" => true}, ::Haml::ObjectRef.parse([x]))`+"\n"+`_hamlout << ">\n"`+"\n"+tail)
}

// TestCompileFormat covers the :xhtml and :html4 formats: self-closing void
// tags, boolean-attribute expansion, and the format passed to the dynamic
// attribute helper. An invalid format is rejected.
func TestCompileFormat(t *testing.T) {
	compileGolden2(t, "%img{src: 'x'}", "xhtml", head+`_hamlout << "<img src=\"x\" />\n"`+"\n"+tail)
	compileGolden2(t, "%br", "xhtml", head+`_hamlout << "<br />\n"`+"\n"+tail)
	compileGolden2(t, "%hr/", "xhtml", head+`_hamlout << "<hr />\n"`+"\n"+tail)
	compileGolden2(t, "%input{checked: true}", "xhtml", head+`_hamlout << "<input checked=\"checked\" />\n"`+"\n"+tail)
	compileGolden2(t, "%p{id: w}", "xhtml",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render("xhtml", {id: w})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// html4 keeps html-style bare void tags but a transitional doctype.
	compileGolden2(t, "%br", "html4", head+`_hamlout << "<br>\n"`+"\n"+tail)
	if _, err := Compile("%p", Options{Format: "bogus"}); err == nil {
		t.Fatal("invalid format should error")
	}
}

// compileGolden2 is compileGolden with an explicit format.
func compileGolden2(t *testing.T, tpl, format, want string) {
	t.Helper()
	got, err := Compile(tpl, Options{Format: format})
	if err != nil {
		t.Fatalf("Compile(%q, %q): %v", tpl, format, err)
	}
	if got != want {
		t.Errorf("Compile(%q, %q)\n got=%q\nwant=%q", tpl, format, got, want)
	}
}

func TestCompileInterpolation(t *testing.T) {
	// Interpolation is HTML-escaped by default (the gem's behaviour): each "#{}"
	// becomes an escaped append, literal runs stay coalesced static.
	compileGolden(t, "%p Hello #{name}",
		head+`_hamlout << "<p>Hello "`+"\n"+`_hamlout << ::Haml::Util.escape_html((name).to_s)`+"\n"+`_hamlout << "</p>\n"`+"\n"+tail)
	compileGolden(t, "plain #{x} txt",
		head+`_hamlout << "plain "`+"\n"+`_hamlout << ::Haml::Util.escape_html((x).to_s)`+"\n"+`_hamlout << " txt\n"`+"\n"+tail)
	// Literal quotes/backslashes around interpolation stay literal (static run).
	compileGolden(t, `say "#{q}" \x`,
		head+`_hamlout << "say \""`+"\n"+`_hamlout << ::Haml::Util.escape_html((q).to_s)`+"\n"+`_hamlout << "\" \\x\n"`+"\n"+tail)
	// An unterminated interpolation copies the remaining text as the expression.
	compileGolden(t, "a #{b",
		head+`_hamlout << "a "`+"\n"+`_hamlout << ::Haml::Util.escape_html((b).to_s)`+"\n"+`_hamlout << "\n"`+"\n"+tail)
}

func TestCompileMultiline(t *testing.T) {
	compileGolden(t, "%p A |\n  B |\n  C |", head+`_hamlout << "<p>A B C</p>\n"`+"\n"+tail)
	compileGolden(t, "hello |\nworld |", head+`_hamlout << "hello world \n"`+"\n"+tail)
	// A "|" without a preceding space is not a continuation.
	compileGolden(t, "%p A|", head+`_hamlout << "<p>A|</p>\n"`+"\n"+tail)
}

func TestCompileWhitespaceMarkers(t *testing.T) {
	// ">" (remove outer whitespace): strips the whitespace preceding the tag and
	// suppresses the newline after its close. Between two static siblings the
	// preceding newline is trimmed from the coalesced static run.
	compileGolden(t, "%li a\n%li> b",
		head+`_hamlout << "<li>a</li><li>b</li>"`+"\n"+tail)
	// The nuke-outer rstrip on a fresh buffer (nothing precedes) still emits the
	// runtime rstrip! and suppresses the trailing newline.
	compileGolden(t, "%p>\n  a\n  b",
		head+"_hamlout.rstrip!\n"+`_hamlout << "<p>\na\nb\n</p>"`+"\n"+tail)
	// "<" (remove inner whitespace): no newline after the open tag; the trailing
	// whitespace before the close tag is trimmed.
	compileGolden(t, "%p<\n  a\n  b",
		head+`_hamlout << "<p>a\nb</p>\n"`+"\n"+tail)
	// A dynamic sibling before a nuke-outer element takes the runtime-rstrip path.
	compileGolden(t, `= "x"`+"\n%p> y",
		head+`_hamlout << ::Haml::Util.escape_html(("x").to_s); _hamlout << "\n"`+"\n"+
			"_hamlout.rstrip!\n"+`_hamlout << "<p>y</p>"`+"\n"+tail)
	// Nuke-outer on a void tag suppresses its trailing newline (the leading
	// rstrip! is a runtime no-op on the still-empty buffer).
	compileGolden(t, "%img>\n%img", head+"_hamlout.rstrip!\n"+`_hamlout << "<img><img>\n"`+"\n"+tail)
	// Nuke-inner on an element with inline content is a no-op (already inline).
	compileGolden(t, "%p< x", head+`_hamlout << "<p>x</p>\n"`+"\n"+tail)
}

func TestOptions(t *testing.T) {
	got, err := Compile("%p= x", Options{BufVar: "buf", EscapeFn: "esc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "buf = ::String.new\n") {
		t.Errorf("BufVar not applied: %q", got)
	}
	if !strings.Contains(got, "esc((x).to_s)") {
		t.Errorf("EscapeFn not applied: %q", got)
	}
}

func TestRender(t *testing.T) {
	// nil evaluator returns the compiled source.
	src, err := Render("%p x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "<p>x</p>") {
		t.Errorf("Render(nil eval) = %q", src)
	}
	// Custom evaluator is invoked with source and locals.
	var gotSrc string
	var gotLocals map[string]string
	out, err := Render("%p= n", map[string]string{"n": "1"},
		func(s string, l map[string]string) (string, error) {
			gotSrc, gotLocals = s, l
			return "RENDERED", nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if out != "RENDERED" {
		t.Errorf("Render eval result = %q", out)
	}
	if !strings.Contains(gotSrc, "escape_html") || gotLocals["n"] != "1" {
		t.Errorf("evaluator got src=%q locals=%v", gotSrc, gotLocals)
	}
}

func TestHTMLEscape(t *testing.T) {
	cases := map[string]string{
		"plain":     "plain",
		`a&<>"'b`:   "a&amp;&lt;&gt;&quot;&#39;b",
		"no-change": "no-change",
	}
	for in, want := range cases {
		if got := HTMLEscape(in); got != want {
			t.Errorf("HTMLEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrimTrailingNewline(t *testing.T) {
	if got := TrimTrailingNewline("x\n"); got != "x" {
		t.Errorf("got %q", got)
	}
	if got := TrimTrailingNewline("x"); got != "x" {
		t.Errorf("got %q", got)
	}
	// preludeMarker exists purely to document the runtime contract.
	if preludeMarker == "" {
		t.Error("preludeMarker should document the runtime symbols")
	}
}
