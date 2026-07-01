package haml

import "testing"

// TestSplitKeyValueInternal drives splitKeyValue over every recognised and
// unrecognised shape directly.
func TestSplitKeyValueInternal(t *testing.T) {
	cases := []struct {
		in       string
		key, val string
		ok       bool
	}{
		{"a: 'x'", "a", "'x'", true},
		{"'k' => 'v'", "k", "'v'", true},
		{":s => 1", "s", "1", true},
		{`"q" => v`, "q", "v", true},
		{"'' => v", "", "", false},         // empty hashrocket key
		{": v", "", "", false},             // empty symbol key
		{"noseparator", "", "", false},     // neither form
		{"a: 'x, y'", "a", "'x, y'", true}, // colon-space inside value string ignored (first wins)
	}
	for _, c := range cases {
		k, v, ok := splitKeyValue(c.in)
		if ok != c.ok || (ok && (k != c.key || v != c.val)) {
			t.Errorf("splitKeyValue(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, k, v, ok, c.key, c.val, c.ok)
		}
	}
}

// TestParseHTMLAttrsInternal drives parseHTMLAttrs over bare flags, quoted and
// bare values, and trailing/edge whitespace.
func TestParseHTMLAttrsInternal(t *testing.T) {
	// Trailing "=" with no value stops cleanly.
	n := &node{}
	parseHTMLAttrs(n, "a=")
	// A double-quoted value with an escaped quote inside.
	n2 := &node{}
	parseHTMLAttrs(n2, `a="x\"y" b`)
	if len(n2.staticAttr) != 2 {
		t.Fatalf("parseHTMLAttrs got %d attrs", len(n2.staticAttr))
	}
	if n2.staticAttr[0].value != `x"y` || !n2.staticAttr[1].isBool {
		t.Errorf("parseHTMLAttrs values = %+v", n2.staticAttr)
	}
	// Only whitespace.
	n3 := &node{}
	parseHTMLAttrs(n3, "   ")
	if len(n3.staticAttr) != 0 {
		t.Errorf("whitespace-only html attrs = %+v", n3.staticAttr)
	}
	// A dynamic bare value marks the node dynamic.
	n4 := &node{}
	parseHTMLAttrs(n4, "href=someVar")
	if n4.dynAttrRB == "" {
		t.Error("dynamic html attr not marked")
	}
}

// TestParseDataHashInternal covers empty entries and non-literal values.
func TestParseDataHashInternal(t *testing.T) {
	n := &node{}
	if !parseDataHash(n, "a: '1', , b: '2'") { // empty entry skipped
		t.Fatal("parseDataHash literal failed")
	}
	if !parseDataHash(&node{}, "") {
		t.Error("empty data hash should be ok")
	}
	if parseDataHash(&node{}, "x: v") { // non-literal
		t.Error("non-literal data value should fail")
	}
	if parseDataHash(&node{}, "bogus") { // unparseable entry
		t.Error("unparseable data entry should fail")
	}
}

// TestMarkDynamicAppend covers the second (append) branch of markDynamic.
func TestMarkDynamicAppend(t *testing.T) {
	n := &node{}
	markDynamic(n, "a: 1")
	markDynamic(n, "b: 2")
	if n.dynAttrRB != "a: 1, b: 2" {
		t.Errorf("markDynamic append = %q", n.dynAttrRB)
	}
}

// TestFindTopLevel covers the quote and depth branches, including a match found
// only after a bracketed region.
func TestFindTopLevel(t *testing.T) {
	if findTopLevel("a{=>}b=>c", "=>") != 6 {
		t.Errorf("findTopLevel skip-in-braces = %d", findTopLevel("a{=>}b=>c", "=>"))
	}
	if findTopLevel(`"=>"=>x`, "=>") != 4 {
		t.Errorf("findTopLevel skip-in-quotes = %d", findTopLevel(`"=>"=>x`, "=>"))
	}
	if findTopLevel(`"a\"b"=>x`, "=>") != 6 {
		t.Errorf("findTopLevel escaped-quote = %d", findTopLevel(`"a\"b"=>x`, "=>"))
	}
	if findTopLevel("abc", "=>") != -1 {
		t.Error("findTopLevel no-match should be -1")
	}
}

// TestEndsWithDoInternal covers the "do" and "do |...|" token detection.
func TestEndsWithDoInternal(t *testing.T) {
	yes := []string{"loop do", "x do", "do", "each do |i|", "map do|x|"}
	for _, s := range yes {
		if !endsWithDo(s) {
			t.Errorf("endsWithDo(%q) = false", s)
		}
	}
	no := []string{"foo", "widow", "a | b", "ado"}
	for _, s := range no {
		if endsWithDo(s) {
			t.Errorf("endsWithDo(%q) = true", s)
		}
	}
}

// TestEndsWithPipeInternal covers the multiline-marker recognition edges.
func TestEndsWithPipeInternal(t *testing.T) {
	if !endsWithPipe("a |") {
		t.Error("'a |' should end with pipe marker")
	}
	if !endsWithPipe("|") {
		t.Error("'|' alone should be a marker")
	}
	if endsWithPipe("a|") {
		t.Error("'a|' (no space) is not a marker")
	}
	if endsWithPipe("abc") {
		t.Error("'abc' is not a marker")
	}
}

// TestRenderStaticAttrsNilValueAttr covers the nil-marker path for a non-boolean
// value attribute that is later overwritten (last-wins) — reaching the "seen"
// update branch inside the nil handling.
func TestRenderStaticAttrsNilValueAttr(t *testing.T) {
	// Two href entries, the first a nil marker, the second a real value: the
	// second overwrites, exercising the seen-index update on a value attr.
	compileGolden(t, "%a{href: nil, href: 'x'}",
		head+`_hamlout << "<a href=\"x\"></a>\n"`+"\n"+tail)
	// nil marker on a value attr that appears twice as nil (seen update path).
	compileGolden(t, "%a{href: nil, href: nil}",
		head+`_hamlout << "<a href=\"\"></a>\n"`+"\n"+tail)
}

// TestEmptyEntriesAndEscapes covers the empty-entry skip in a Ruby hash, the
// backslash-only blank line, a symbol key via unquoteKey, and the quote/depth
// branches of the symbol-key scan in splitKeyValue.
func TestEmptyEntriesAndEscapes(t *testing.T) {
	// Empty entry between commas is skipped.
	compileGolden(t, "%p{a: '1', , b: '2'}",
		head+`_hamlout << "<p a=\"1\" b=\"2\"></p>\n"`+"\n"+tail)
	// Backslash-only line emits a blank line (matches the gem's "\n").
	compileGolden(t, "\\", head+`_hamlout << "\n"`+"\n"+tail)
	// A :symbol key resolves via unquoteKey's ":" branch.
	k, _, ok := splitKeyValue(":sym => 'v'")
	if !ok || k != "sym" {
		t.Errorf("splitKeyValue :sym => v = (%q,%v)", k, ok)
	}
	// A bare (unquoted, non-symbol) hashrocket key hits unquoteKey's fall-through.
	if got := unquoteKey("bare"); got != "bare" {
		t.Errorf("unquoteKey(bare) = %q", got)
	}
	// Symbol-key scan skips a colon inside a quoted value and inside brackets,
	// and handles an escaped quote — reaching those branches.
	if _, _, ok := splitKeyValue(`a: "x: y"`); !ok {
		t.Error("colon inside quoted value should not break scan")
	}
	if _, _, ok := splitKeyValue(`a: [1, 2]`); !ok {
		t.Error("bracketed value should parse")
	}
	if _, _, ok := splitKeyValue(`a: "x\"y"`); !ok {
		t.Error("escaped quote in value should parse")
	}
}

// TestParseHTMLAttrsSpaceEqual covers the whitespace-around-"=" branch and a
// bare flag followed by more attributes.
func TestParseHTMLAttrsSpaceEqual(t *testing.T) {
	n := &node{}
	parseHTMLAttrs(n, "a = 'x'")
	if len(n.staticAttr) != 1 || n.staticAttr[0].value != "x" {
		t.Errorf("space-around-= html attr = %+v", n.staticAttr)
	}
	// Bare flag then another attribute.
	n2 := &node{}
	parseHTMLAttrs(n2, "checked type='text'")
	if len(n2.staticAttr) != 2 || !n2.staticAttr[0].isBool {
		t.Errorf("bare-flag-then-attr = %+v", n2.staticAttr)
	}
}

// TestParseFilterReindent covers parseFilter's "ci < strip" branch: a body line
// less-indented than the first child but still inside the filter.
func TestParseFilterReindent(t *testing.T) {
	// First body line indented 4, second indented 2 (still > filter indent 0).
	compileGolden(t, ":plain\n    deep\n  less",
		head+`_hamlout << "deep\nless\n"`+"\n"+tail)
}
