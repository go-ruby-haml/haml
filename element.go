package haml

import "strings"

// parseElement parses an element line: a "%tag", or a "."/"#" shorthand
// (implying a div), followed by optional .class/#id shorthand, {} or ()
// attributes, self-close "/", whitespace-removal ">"/"<", and inline content.
func parseElement(content string) (*node, int, error) {
	n := &node{kind: kindElement, tag: "div"}
	i := 0

	// Tag name.
	if content[0] == '%' {
		i++
		start := i
		for i < len(content) && isTagChar(content[i]) {
			i++
		}
		n.tag = content[start:i]
	}

	// .class / #id shorthand (may precede attribute hashes).
	for i < len(content) && (content[i] == '.' || content[i] == '#') {
		marker := content[i]
		i++
		start := i
		for i < len(content) && isNameChar(content[i]) {
			i++
		}
		name := content[start:i]
		if marker == '.' {
			n.staticAttr = append(n.staticAttr, staticAttr{name: "class", value: name, classShorthand: true})
		} else {
			n.staticAttr = append(n.staticAttr, staticAttr{name: "id", value: name, idShorthand: true})
		}
	}

	// Attribute hashes: {} (Ruby-style) and () (HTML-style), possibly repeated.
	for i < len(content) && (content[i] == '{' || content[i] == '(') {
		open := content[i]
		close := byte('}')
		if open == '(' {
			close = ')'
		}
		body, next, ok := scanBalanced(content, i, open, close)
		if !ok {
			// Unterminated attribute list — the gem raises Haml::SyntaxError.
			return nil, 0, &SyntaxError{Line: content, Msg: "unterminated attribute list"}
		}
		if open == '{' {
			parseRubyHashAttrs(n, body)
		} else {
			parseHTMLAttrs(n, body)
		}
		i = next
	}

	// Whitespace-removal markers and self-close.
	for i < len(content) && (content[i] == '>' || content[i] == '<' || content[i] == '/') {
		switch content[i] {
		case '>':
			n.nuke.outer = true
		case '<':
			n.nuke.inner = true
		case '/':
			n.selfClose = true
		}
		i++
	}

	// Inline content after the tag.
	rest := content[i:]
	rest = strings.TrimLeft(rest, " ")
	if rest != "" {
		switch {
		case strings.HasPrefix(rest, "!="):
			n.codeExpr = strings.TrimSpace(rest[2:])
			n.textKind = textUnescaped
			n.text = "\x00expr"
		case strings.HasPrefix(rest, "&="):
			n.codeExpr = strings.TrimSpace(rest[2:])
			n.textKind = textEscaped
			n.text = "\x00expr"
		case strings.HasPrefix(rest, "="):
			n.codeExpr = strings.TrimSpace(rest[1:])
			n.textKind = textEscaped
			n.text = "\x00expr"
		case strings.HasPrefix(rest, "~"):
			n.codeExpr = strings.TrimSpace(rest[1:])
			n.textKind = textEscaped
			n.text = "\x00expr"
		default:
			// Inline element text is right-stripped by Haml (leading whitespace
			// was already removed above); root-level plain text keeps its spaces.
			n.text = strings.TrimRight(rest, " ")
			n.textKind = textPlain
		}
	}
	return n, 1, nil
}

// scanBalanced returns the body between a balanced open/close pair starting at
// content[start] (which must equal open). It respects single- and double-quoted
// Ruby strings so braces inside strings do not confuse the matcher. next is the
// index just past the closing delimiter.
func scanBalanced(content string, start int, open, close byte) (body string, next int, ok bool) {
	depth := 0
	var quote byte
	for i := start; i < len(content); i++ {
		c := content[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(content) {
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
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return content[start+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

// isTagChar reports whether c is valid in a %tag name.
func isTagChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':' || c == '_'
}

// isNameChar reports whether c is valid in a .class / #id shorthand name.
func isNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}
