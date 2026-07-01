package haml

import (
	"strconv"
	"strings"
)

// parseRubyHashAttrs parses the body of a "{ ... }" Haml attribute hash. Each
// entry is either "key: value" (symbol-key shorthand) or "'key' => value" /
// ":key => value" (hashrocket). Literal values (quoted strings, numbers,
// true/false/nil, and nested data hashes) are resolved to static attributes; a
// non-literal value marks the whole hash dynamic and the raw Ruby is carried on
// dynAttrRB for the eval seam.
func parseRubyHashAttrs(n *node, body string) {
	for _, e := range splitTopLevel(body, ',') {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		key, val, ok := splitKeyValue(e)
		if !ok {
			// Could not statically parse this entry: fall back to dynamic.
			markDynamic(n, body)
			return
		}
		if key == "data" && strings.HasPrefix(strings.TrimSpace(val), "{") {
			// data: { ... } expands to data-<k> attributes.
			inner := strings.TrimSpace(val)
			inner = inner[1 : len(inner)-1]
			if !parseDataHash(n, inner) {
				markDynamic(n, body)
				return
			}
			continue
		}
		sa, literal := literalAttr(key, val)
		if !literal {
			markDynamic(n, body)
			return
		}
		n.staticAttr = append(n.staticAttr, sa)
	}
}

// parseDataHash expands a "data: { ... }" nested hash into data-<key> static
// attributes. It returns false if any value is non-literal.
func parseDataHash(n *node, inner string) bool {
	for _, e := range splitTopLevel(inner, ',') {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		key, val, ok := splitKeyValue(e)
		if !ok {
			return false
		}
		sa, literal := literalAttr("data-"+key, val)
		if !literal {
			return false
		}
		n.staticAttr = append(n.staticAttr, sa)
	}
	return true
}

// parseHTMLAttrs parses the body of an HTML-style "( ... )" attribute list:
// space-separated name=value pairs where value is a quoted string, a bare
// literal, or a Ruby expression. Non-literal values mark the element dynamic.
func parseHTMLAttrs(n *node, body string) {
	i := 0
	for i < len(body) {
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) {
			break
		}
		start := i
		for i < len(body) && body[i] != '=' && body[i] != ' ' && body[i] != '\t' {
			i++
		}
		name := body[start:i]
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) || body[i] != '=' {
			// Bare attribute name: treat as boolean-true flag.
			n.staticAttr = append(n.staticAttr, staticAttr{name: name, isBool: true, boolVal: true})
			continue
		}
		i++ // skip '='
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) {
			break
		}
		var val string
		if body[i] == '\'' || body[i] == '"' {
			q := body[i]
			i++
			vs := i
			for i < len(body) && body[i] != q {
				if body[i] == '\\' && i+1 < len(body) {
					i++
				}
				i++
			}
			val = body[vs:i]
			if i < len(body) {
				i++
			}
			n.staticAttr = append(n.staticAttr, staticAttr{name: name, value: unescapeRubyStr(val)})
		} else {
			vs := i
			for i < len(body) && body[i] != ' ' && body[i] != '\t' {
				i++
			}
			raw := body[vs:i]
			sa, literal := literalAttr(name, raw)
			if !literal {
				markDynamic(n, body)
				return
			}
			n.staticAttr = append(n.staticAttr, sa)
		}
	}
}

// splitKeyValue splits a single hash entry into its key and value, handling
// both "key: value" and "'key' => value" / ":key => value" forms. ok is false
// for shapes we do not statically understand.
func splitKeyValue(e string) (key, val string, ok bool) {
	// Hashrocket form.
	if idx := findTopLevel(e, "=>"); idx >= 0 {
		k := strings.TrimSpace(e[:idx])
		v := strings.TrimSpace(e[idx+2:])
		k = unquoteKey(k)
		if k == "" {
			return "", "", false
		}
		return k, v, true
	}
	// Symbol-key "key: value": the key is always a bare identifier before the
	// first ": " separator, so the first ": " delimits key from value (a colon
	// inside the value belongs to the value's own Ruby and is left untouched).
	if idx := strings.Index(e, ": "); idx >= 0 {
		k := strings.TrimSpace(e[:idx])
		v := strings.TrimSpace(e[idx+1:])
		if k == "" {
			return "", "", false
		}
		return k, v, true
	}
	return "", "", false
}

// unquoteKey strips a leading ":" symbol marker or surrounding quotes from an
// attribute key.
func unquoteKey(k string) string {
	k = strings.TrimSpace(k)
	if len(k) >= 2 && (k[0] == '\'' || k[0] == '"') && k[len(k)-1] == k[0] {
		return unescapeRubyStr(k[1 : len(k)-1])
	}
	if strings.HasPrefix(k, ":") {
		return k[1:]
	}
	return k
}

// literalAttr resolves a raw Ruby value into a static attribute, reporting
// whether it was a compile-time literal.
func literalAttr(name, val string) (staticAttr, bool) {
	val = strings.TrimSpace(val)
	switch val {
	case "true":
		return staticAttr{name: name, isBool: true, boolVal: true}, true
	case "false":
		return staticAttr{name: name, isBool: true, boolVal: false}, true
	case "nil":
		// nil on a boolean attribute omits it; on a value attribute the gem
		// renders name="". We model it as a boolean-false so it is omitted for
		// known boolean attrs and, for value attrs, handled by the emitter as "".
		return staticAttr{name: name, isBool: true, boolVal: false, value: "\x00nil"}, true
	}
	// Quoted string literal.
	if len(val) >= 2 && (val[0] == '\'' || val[0] == '"') && val[len(val)-1] == val[0] {
		inner := val[1 : len(val)-1]
		// Interpolation or escapes in a double-quoted literal keep it static only
		// when there is no "#{" interpolation.
		if val[0] == '"' && strings.Contains(inner, "#{") {
			return staticAttr{}, false
		}
		return staticAttr{name: name, value: unescapeRubyStr(inner)}, true
	}
	// Numeric literal.
	if _, err := strconv.Atoi(val); err == nil {
		return staticAttr{name: name, value: val}, true
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return staticAttr{name: name, value: val}, true
	}
	return staticAttr{}, false
}

// markDynamic records that the element's attribute hash could not be fully
// resolved at compile time; the raw Ruby hash body is carried for the eval
// seam, and any statically-parsed shorthand (.class/#id) already on the node is
// merged in by the emitter.
func markDynamic(n *node, body string) {
	if n.dynAttrRB == "" {
		n.dynAttrRB = strings.TrimSpace(body)
	} else {
		n.dynAttrRB += ", " + strings.TrimSpace(body)
	}
}

// unescapeRubyStr resolves the backslash escapes that appear inside a
// single/double-quoted attribute string. It handles the common cases (\\, \',
// \", \n, \t) which is sufficient for the literal values Haml attribute hashes
// carry.
func unescapeRubyStr(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitTopLevel splits s on sep, ignoring separators that appear inside quotes
// or nested brackets/braces/parens.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// findTopLevel returns the index of the first occurrence of sub at bracket/quote
// depth zero, or -1.
func findTopLevel(s, sub string) int {
	depth := 0
	var quote byte
	for i := 0; i+len(sub) <= len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		default:
			if depth == 0 && s[i:i+len(sub)] == sub {
				return i
			}
		}
	}
	return -1
}
