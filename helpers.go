package haml

import "strings"

// voidTags is the HTML5 set of void elements: they never have a closing tag and
// carry no content, so Haml renders them as "<tag>".
var voidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// isVoidTag reports whether tag is an HTML5 void element.
func isVoidTag(tag string) bool { return voidTags[tag] }

// booleanAttrs are attributes Haml renders as bare names when truthy and omits
// when nil/false, matching the gem's boolean-attribute handling.
var booleanAttrs = map[string]bool{
	"disabled": true, "readonly": true, "multiple": true, "checked": true,
	"autobuffer": true, "autoplay": true, "controls": true, "loop": true,
	"selected": true, "hidden": true, "scoped": true, "async": true,
	"defer": true, "reversed": true, "ismap": true, "seamless": true,
	"muted": true, "required": true, "autofocus": true, "novalidate": true,
	"formnovalidate": true, "open": true, "pubdate": true, "itemscope": true,
	"allowfullscreen": true, "default": true, "inert": true, "sortable": true,
	"truespeed": true, "typemustmatch": true, "download": true,
}

// isBooleanAttr reports whether name is an HTML boolean attribute.
func isBooleanAttr(name string) bool { return booleanAttrs[name] }

// blockOpeners are the Ruby keywords whose "-" line opens a block that Haml
// closes with a matching "end" after the nested children.
var blockOpeners = []string{
	"if", "unless", "while", "until", "for", "case", "begin",
	"def", "class", "module", "loop",
}

// opensBlock reports whether a "-" control statement opens a Ruby block that
// needs a trailing "end". It recognises leading block keywords and trailing
// "do"/"do |args|" forms, but not modifier "if"/"unless"/"while"/"until"
// (a statement with the keyword mid-line), nor "else"/"elsif"/"when"/"rescue"
// /"ensure" continuations (which belong to an already-open block).
func opensBlock(stmt string) bool {
	s := strings.TrimSpace(stmt)
	if s == "" {
		return false
	}
	// Continuation keywords do not open a new block.
	for _, k := range []string{"else", "elsif", "when", "in", "rescue", "ensure", "end"} {
		if s == k || strings.HasPrefix(s, k+" ") {
			return false
		}
	}
	// Trailing "do" / "do |..|" opens a block.
	if endsWithDo(s) {
		return true
	}
	// Leading block keyword, but only when used as a statement opener (not a
	// modifier). We approximate: a leading keyword opens a block unless the
	// statement also reads as a one-liner (contains " then " won't nest here).
	for _, k := range blockOpeners {
		if s == k || strings.HasPrefix(s, k+" ") || strings.HasPrefix(s, k+"(") {
			return true
		}
	}
	return false
}

// isContinuation reports whether a "-" control statement continues an already
// open block rather than opening a fresh one: the "elsif"/"else"/"when"/"in"/
// "rescue"/"ensure" branch keywords. These share the enclosing block's "end".
func isContinuation(stmt string) bool {
	s := strings.TrimSpace(stmt)
	for _, k := range []string{"elsif", "else", "when", "in", "rescue", "ensure"} {
		if s == k || strings.HasPrefix(s, k+" ") {
			return true
		}
	}
	return false
}

// endsWithDo reports whether a statement ends with a "do" or "do |block args|"
// block-opener.
func endsWithDo(s string) bool {
	s = strings.TrimRight(s, " \t")
	if strings.HasSuffix(s, "do") {
		// Ensure "do" is a standalone token.
		if len(s) == 2 || s[len(s)-3] == ' ' || s[len(s)-3] == '\t' {
			return true
		}
	}
	if strings.HasSuffix(s, "|") {
		// "... do |a, b|" — find a "do |" before the trailing pipe.
		if idx := strings.LastIndex(s, "do |"); idx >= 0 {
			return true
		}
		if idx := strings.LastIndex(s, "do|"); idx >= 0 {
			return true
		}
	}
	return false
}

// rubyStrLit renders s as a Ruby double-quoted string literal (used when
// splicing static shorthand into a dynamic attribute hash).
func rubyStrLit(s string) string { return rubyDump(s) }

// rubyInterp renders literal text containing "#{}" interpolation as a Ruby
// double-quoted string. Interpolation sequences are preserved verbatim so the
// eval seam evaluates them; the surrounding literal bytes are escaped the way a
// double-quoted Ruby literal requires (", \ and interpolation-triggering '#').
func rubyInterp(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	i := 0
	for i < len(s) {
		if s[i] == '#' && i+1 < len(s) && s[i+1] == '{' {
			// Copy the whole "#{ ... }" interpolation verbatim (balanced braces).
			depth := 0
			j := i
			for j < len(s) {
				if s[j] == '{' {
					depth++
				} else if s[j] == '}' {
					depth--
					if depth == 0 {
						j++
						break
					}
				}
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		switch s[i] {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		default:
			b.WriteByte(s[i])
		}
		i++
	}
	b.WriteByte('"')
	return b.String()
}
