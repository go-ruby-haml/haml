package haml

import (
	"errors"
	"strings"
	"testing"
)

// TestSyntaxError covers the unterminated-attribute-list error path threaded
// through parseElement -> parseLine -> parse -> Compile, plus SyntaxError.Error.
func TestSyntaxError(t *testing.T) {
	for _, tpl := range []string{"%p{a: 'b'", "%p(href='x'", "%p{a: {b: 'c'}"} {
		_, err := Compile(tpl, Options{})
		if err == nil {
			t.Fatalf("Compile(%q) expected error", tpl)
		}
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Fatalf("Compile(%q) error type = %T", tpl, err)
		}
		if !strings.Contains(se.Error(), "unterminated") {
			t.Errorf("SyntaxError.Error() = %q", se.Error())
		}
	}
	// Render surfaces the same compile error.
	if _, err := Render("%p{a: 'b'", nil, nil); err == nil {
		t.Fatal("Render expected compile error")
	}
}

// TestRenderEvalError covers the evaluator error path.
func TestRenderEvalError(t *testing.T) {
	_, err := Render("%p x", nil, func(string, map[string]string) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Render eval error = %v", err)
	}
}

// TestRubyDumpEscapes covers every branch of rubyDump: the C-style escapes, the
// interpolation-guard '#', high bytes as \xHH, and the printable path.
func TestRubyDumpEscapes(t *testing.T) {
	cases := map[string]string{
		"plain":              `"plain"`,
		"\a\b\t\n\v\f\r\x1b": `"\a\b\t\n\v\f\r\e"`,
		`a"b\c`:              `"a\"b\\c"`,
		"a#{b}":              `"a\#{b}"`,
		"a#b":                `"a#b"`,
		"a#$g":               `"a\#$g"`,
		"a#@v":               `"a\#@v"`,
		"\x00\xff":           `"\x00\xFF"`,
	}
	for in, want := range cases {
		if got := rubyDump(in); got != want {
			t.Errorf("rubyDump(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRubyInterpEscapes covers rubyInterp: preserved interpolation with escaped
// surrounding literal quotes/backslashes.
func TestRubyInterpEscapes(t *testing.T) {
	if got := rubyInterp(`a"b\c#{x}d`); got != `"a\"b\\c#{x}d"` {
		t.Errorf("rubyInterp = %q", got)
	}
	// Nested braces inside interpolation are copied verbatim.
	if got := rubyInterp(`#{h[:k]}x`); got != `"#{h[:k]}x"` {
		t.Errorf("rubyInterp nested = %q", got)
	}
	// Unterminated interpolation copies to end without panicking.
	if got := rubyInterp(`a#{b`); got != `"a#{b"` {
		t.Errorf("rubyInterp unterminated = %q", got)
	}
}

// TestUnescapeRubyStr covers the escape resolution branches.
func TestUnescapeRubyStr(t *testing.T) {
	cases := map[string]string{
		`plain`:     "plain",
		`a\nb`:      "a\nb",
		`a\tb`:      "a\tb",
		`a\rb`:      "a\rb",
		`a\'b`:      "a'b",
		`a\\b`:      `a\b`,
		`trailing\`: `trailing\`, // dangling backslash kept
	}
	for in, want := range cases {
		if got := unescapeRubyStr(in); got != want {
			t.Errorf("unescapeRubyStr(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSplitKeyValueForms covers hashrocket, symbol keys, quoted keys, and the
// not-understood shapes that fall back to dynamic.
func TestSplitKeyValueForms(t *testing.T) {
	// String key via hashrocket, colon inside a value string, ternary-like value.
	compileGolden(t, "%p{'k' => 'v'}", head+`_hamlout << "<p k=\"v\"></p>\n"`+"\n"+tail)
	compileGolden(t, "%p{:s => 'v'}", head+`_hamlout << "<p s=\"v\"></p>\n"`+"\n"+tail)
	// A value containing a nested hash / array does not confuse the splitter.
	compileGolden(t, "%p{a: 'x,y', b: 'z'}", head+`_hamlout << "<p a=\"x,y\" b=\"z\"></p>\n"`+"\n"+tail)
	// Empty hashrocket key falls back to dynamic.
	compileGolden(t, "%p{'' => v}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render({'' => v})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
	// Un-parseable entry (no ": " and no "=>") falls back to dynamic.
	compileGolden(t, "%p{foo}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render({foo})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
}

// TestParseFilterTrailingBlank exercises parseFilter's speculative blank-line
// consumption and trimming, and a filter that ends the template.
func TestParseFilterTrailingBlank(t *testing.T) {
	compileGolden(t, ":plain\n  a\n\n", head+`_hamlout << "a\n"`+"\n"+tail)
	// data hash whose value is itself dynamic keeps the whole thing dynamic.
	compileGolden(t, "%p{data: {x: 'a', y: v}}",
		head+`_hamlout << "<p"`+"\n"+`_hamlout << ::Haml::HamlAttributes.render({data: {x: 'a', y: v}})`+"\n"+`_hamlout << "></p>\n"`+"\n"+tail)
}

// TestOpensBlockAndContinuation covers block-opener and continuation detection
// edge cases that the golden tests do not otherwise reach.
func TestOpensBlockAndContinuation(t *testing.T) {
	blockYes := []string{"if x", "unless x", "while x", "until x", "for i in a",
		"case x", "begin", "def f", "class C", "module M", "loop do",
		"items.each do |i|", "3.times do", "loop", "if(x)"}
	for _, s := range blockYes {
		if !opensBlock(s) {
			t.Errorf("opensBlock(%q) = false, want true", s)
		}
	}
	blockNo := []string{"", "x = 1", "puts y", "else", "elsif z", "when 1",
		"rescue", "ensure", "end", "in x", "return do_thing"}
	for _, s := range blockNo {
		if opensBlock(s) {
			t.Errorf("opensBlock(%q) = true, want false", s)
		}
	}
	contYes := []string{"else", "elsif x", "when 1", "in y", "rescue", "ensure"}
	for _, s := range contYes {
		if !isContinuation(s) {
			t.Errorf("isContinuation(%q) = false", s)
		}
	}
	if isContinuation("if x") {
		t.Error("isContinuation(if) should be false")
	}
}

// TestEmitTextInterpolationStandalone covers a standalone interpolated text
// node (not inline element content).
func TestEmitTextInterpolationStandalone(t *testing.T) {
	compileGolden(t, "root #{v} text",
		head+`_hamlout << "root #{v} text"; _hamlout << "\n"`+"\n"+tail)
	// Blank standalone text node between elements (empty line handling).
	compileGolden(t, "%p a\n\n%p b",
		head+`_hamlout << "<p>a</p>\n<p>b</p>\n"`+"\n"+tail)
}

// TestValNilMarker covers staticAttr.val on the nil marker for a value attr.
func TestValNilMarker(t *testing.T) {
	sa := staticAttr{name: "href", value: "\x00nil"}
	if sa.val() != "" {
		t.Errorf("val() nil marker = %q", sa.val())
	}
	sa2 := staticAttr{name: "href", value: "x"}
	if sa2.val() != "x" {
		t.Errorf("val() = %q", sa2.val())
	}
}

// TestScanBalancedQuotes covers scanBalanced's quote handling (braces inside
// strings do not close the attribute list) and escapes.
func TestScanBalancedQuotes(t *testing.T) {
	compileGolden(t, `%p{a: "x}y"}`, head+`_hamlout << "<p a=\"x}y\"></p>\n"`+"\n"+tail)
	compileGolden(t, `%p{a: 'it\'s'}`, head+`_hamlout << "<p a=\"it&#39;s\"></p>\n"`+"\n"+tail)
}
