package haml

import (
	"os/exec"
	"strings"
	"testing"
)

// oracleCase is a template plus the locals it needs, expressed as name -> Ruby
// literal source, and an optional output format. Each is rendered both by the
// `haml` gem and by eval'ing our compiled source under the reference prelude;
// the two HTML strings must match byte-for-byte.
type oracleCase struct {
	tpl    string
	locals map[string]string
	format string // "" (html5), "xhtml" or "html4"
}

var oracleCorpus = []oracleCase{
	// Elements and shorthand.
	{"%p hello", nil, ""},
	{"%p", nil, ""},
	{".foo", nil, ""},
	{"#bar", nil, ""},
	{".foo.bar#baz", nil, ""},
	{"%section#main.wide", nil, ""},
	{"%tag\n  %inner\n    deep", nil, ""},
	{"%ul\n  %li a\n  %li b", nil, ""},
	{"%p\n  %b bold\n  plain", nil, ""},
	// Attributes (static).
	{"%a{href: 'x'} link", nil, ""},
	{"%a(href='x') link", nil, ""},
	{"%a.c1{class: 'c2'} t", nil, ""},
	{"%a#i1{id: 'i2'} t", nil, ""},
	{"%div{data: {foo: 'bar', baz: 'q'}}", nil, ""},
	{"%p{style: 'a', title: 'b'}", nil, ""},
	{"%p{'data-x' => 'y'}", nil, ""},
	{"%input{type: 'checkbox', checked: true}", nil, ""},
	{"%input{type: 'checkbox', checked: false}", nil, ""},
	{"%input{required: true, type: 'text'}", nil, ""},
	{"%p{a: 1}", nil, ""},
	{`%input{name: 'q', value: 'a"b'}`, nil, ""},
	{"%meta{charset: 'utf-8'}/", nil, ""},
	// Void tags.
	{"%br", nil, ""}, {"%img", nil, ""}, {"%hr", nil, ""}, {"%input", nil, ""}, {"%meta", nil, ""},
	// Doctype / comments.
	{"!!!", nil, ""},
	{"/ comment", nil, ""},
	{"/[if IE]\n  %p ie", nil, ""},
	{"-# silent\n%p hi", nil, ""},
	// Filters.
	{":plain\n  raw <b> text", nil, ""},
	{":javascript\n  var x = 1;", nil, ""},
	{":css\n  .a { color: red; }", nil, ""},
	{":escaped\n  <b>", nil, ""},
	{"%div\n  :plain\n    static", nil, ""},
	// Text and escaping.
	{"text at root", nil, ""},
	{"%p= '<b>&'", nil, ""},
	{"%p!= '<b>'", nil, ""},
	{"%p&= '<x>'", nil, ""},
	{"= 1 + 2", nil, ""},
	{"%p= 1 + 2", nil, ""},
	{"\\%p not a tag", nil, ""},
	{"%p A |\n  B |\n  C |", nil, ""},
	// Dynamic expressions / attributes / control flow.
	{"%p= name", map[string]string{"name": `"World"`}, ""},
	{"%p{id: who}", map[string]string{"who": `"bar"`}, ""},
	{"%a{href: url} link", map[string]string{"url": `"http://x"`}, ""},
	{"%p Hello #{name}", map[string]string{"name": `"Bob"`}, ""},
	{"plain #{1 + 1} txt", nil, ""},
	{"%p!= raw", map[string]string{"raw": `"<b>x</b>"`}, ""},
	{"- if flag\n  %p yes\n- else\n  %p no", map[string]string{"flag": "true"}, ""},
	{"%ul\n  - items.each do |i|\n    %li= i", map[string]string{"items": `["a","b"]`}, ""},
	{"- 3.times do |i|\n  %span= i", nil, ""},
	{"%section\n  %h1= title\n  %p= body", map[string]string{"title": `"T"`, "body": `"B"`}, ""},
	{"%p{class: cls}", map[string]string{"cls": `"a b"`}, ""},
	// Whitespace-removal markers: ">" (outer) and "<" (inner).
	{"%li hi\n%li> there\n%li last", nil, ""},
	{"%div\n  %p> a\n  %p b", nil, ""},
	{"%div\n  %p>< a", nil, ""},
	{"%p<\n  a\n  b", nil, ""},
	{"%p>\n  a\n  b", nil, ""},
	{"%blockquote<\n  %div\n    Foo!", nil, ""},
	{"%li=1\n%li>=2\n%li=3", nil, ""},
	{"= \"x\"\n%p> y", nil, ""},
	{"%img>\n%img", nil, ""},
	// Full doctype table (html5 default, then xhtml and html4).
	{"!!! Strict", nil, ""}, {"!!! Frameset", nil, ""}, {"!!! RDFa", nil, ""},
	{"!!! XML", nil, ""}, {"!!! 1.1", nil, ""},
	{"!!!", nil, "xhtml"}, {"!!! XML", nil, "xhtml"}, {"!!! 1.1", nil, "xhtml"},
	{"!!! Strict", nil, "xhtml"}, {"!!! Mobile", nil, "xhtml"}, {"!!! Basic", nil, "xhtml"},
	{"!!!", nil, "html4"}, {"!!! Strict", nil, "html4"},
	// Interpolation is HTML-escaped by default; "!" disables it, "&" forces it.
	{`%p a #{"<b>"} c`, nil, ""},
	{`plain #{"<b>"}`, nil, ""},
	{`& #{"<b>"}`, nil, ""},
	{`! #{"<b>"}`, nil, ""},
	{"&x", nil, ""}, {"!x", nil, ""}, {"&", nil, ""},
	{"&= '<x>'", nil, ""},
	{`%a{title: "a<b"} x`, map[string]string{}, ""},
	// Object references (needs the FooBar class the oracle defines).
	{"%div[x]", map[string]string{"x": "FooBar.new"}, ""},
	{"%div[x, :pre]", map[string]string{"x": "FooBar.new"}, ""},
	{"%li[x]{class: 'extra'}", map[string]string{"x": "FooBar.new"}, ""},
	{"%tr.c[x]", map[string]string{"x": "FooBar.new"}, ""},
	{"%div[nil]", nil, ""},
	// xhtml format: self-closing void tags and expanded boolean attributes.
	{"%img{src: 'x'}", nil, "xhtml"},
	{"%input{checked: true, type: 'text'}", nil, "xhtml"},
	{"%br", nil, "xhtml"}, {"%hr/", nil, "xhtml"},
	{"%input{checked: c, type: 't'}", map[string]string{"c": "true"}, "xhtml"},
	{"%p{id: who}", map[string]string{"who": `"z"`}, "xhtml"},
	// :preserve keeps every newline as an entity.
	{":preserve\n  a\n  b", nil, ""},
	// Dynamic class/id merge across shorthand + hash + data expansion.
	{"%p.base{class: cls}", map[string]string{"cls": `["m","n"]`}, ""},
	{"%p{class: cls, data: {a: 'b'}}", map[string]string{"cls": `"x y"`}, ""},
}

func rubyAvailable() bool {
	if _, err := exec.LookPath("ruby"); err != nil {
		return false
	}
	cmd := exec.Command("ruby", "-e", "require 'haml'")
	return cmd.Run() == nil
}

func setupLocals(locals map[string]string) string {
	var b strings.Builder
	for k, v := range locals {
		b.WriteString(k + " = " + v + "; ")
	}
	return b.String()
}

// oracleClasses is Ruby the oracle prepends so object-reference cases can build
// a stable object (id 42, class name FooBar => class "foo_bar").
const oracleClasses = `class FooBar; def id; 42; end; end; `

// gemFormat maps an oracleCase.format to the gem's :format symbol.
func gemFormat(format string) string {
	if format == "" {
		return "html5"
	}
	return format
}

// gemRender renders tpl with the `haml` gem, binding locals under the given
// format.
func gemRender(t *testing.T, tpl string, locals map[string]string, format string) string {
	t.Helper()
	script := `$stdout.binmode; require 'haml'; ` + oracleClasses + setupLocals(locals) +
		`b = binding; ` +
		`print Haml::Template.new(format: :` + gemFormat(format) + `) { $stdin.read }.render(Object.new, ` +
		`b.local_variables.reject { |n| n == :b }.map { |n| [n, b.local_variable_get(n)] }.to_h)`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(tpl)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gem render %q: %v\n%s", tpl, err, out)
	}
	return string(out)
}

// ourRender eval's our compiled source under the reference prelude, binding
// locals, proving the emitted Ruby renders identically to the gem.
func ourRender(t *testing.T, tpl string, locals map[string]string, format string) string {
	t.Helper()
	src, err := Compile(tpl, Options{Format: format})
	if err != nil {
		t.Fatalf("Compile %q: %v", tpl, err)
	}
	script := `$stdout.binmode; require_relative 'testdata/prelude'; ` + oracleClasses +
		setupLocals(locals) + `print eval($stdin.read)`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval our src %q: %v\nsrc=%s\n%s", tpl, err, src, out)
	}
	return string(out)
}

func TestDifferentialRenderAgainstGem(t *testing.T) {
	if !rubyAvailable() {
		t.Skip("ruby with the haml gem not available; skipping differential oracle")
	}
	for _, tc := range oracleCorpus {
		want := gemRender(t, tc.tpl, tc.locals, tc.format)
		got := ourRender(t, tc.tpl, tc.locals, tc.format)
		if got != want {
			t.Errorf("render mismatch tpl=%q fmt=%q\n gem=%q\n our=%q", tc.tpl, tc.format, want, got)
		}
	}
}
