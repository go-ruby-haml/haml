package haml

import (
	"fmt"
	"strings"
)

// node is one parsed Haml line together with its nested children. The parser
// turns the indentation-structured template into a tree of these; the compiler
// walks the tree to emit Ruby source.
type node struct {
	kind     nodeKind
	raw      string // the line content with leading indentation removed
	children []*node

	// Element fields (kindElement).
	tag        string
	staticAttr []staticAttr // .class/#id shorthand + literal {}/() attributes
	dynAttrRB  string       // Ruby hash source for non-literal {}/() attributes, or ""
	selfClose  bool         // explicit "/" self-close marker
	nuke       nukeMode     // ">"/"<" whitespace removal

	// Content carried on the same line as an element, or a standalone text/code
	// line. text is the literal/interpolated content; codeExpr is a Ruby
	// expression; ctrl is a Ruby control statement.
	text     string
	textKind textKind
	codeExpr string
	control  string

	// Filter fields (kindFilter).
	filterName string
	filterBody []string

	// Comment fields (kindComment).
	commentCond string // conditional-comment condition, e.g. "if IE", or ""
}

type nodeKind int

const (
	kindElement nodeKind = iota
	kindText             // plain text / interpolated text at any level
	kindCode             // "-" control line (no output)
	kindExpr             // "=" expression line (output)
	kindComment          // "/" HTML comment
	kindSilent           // "-#" silent comment (discarded)
	kindFilter           // ":name" filter block
	kindDoctype          // "!!!" doctype
)

type textKind int

const (
	textPlain     textKind = iota // literal or interpolated, unescaped
	textEscaped                   // "=" expression, HTML-escaped
	textUnescaped                 // "!=" / "!" expression, not escaped
)

// nukeMode records the ">" (nuke outer) / "<" (nuke inner) whitespace-removal
// markers on an element.
type nukeMode struct {
	outer bool // ">"
	inner bool // "<"
}

// staticAttr is a fully-resolved attribute known at compile time: a literal
// name/value, or a boolean flag.
type staticAttr struct {
	name           string
	value          string // resolved value (already the raw string, escaped at emit)
	isBool         bool   // true => rendered as a bare boolean attribute when boolVal
	boolVal        bool
	classShorthand bool // came from ".x" (merged with space)
	idShorthand    bool // came from "#x" (merged with "_")
}

// parse splits the template into physical lines, resolves the "|" multiline
// continuation, and builds the indentation tree.
func parse(template string) ([]*node, error) {
	lines := splitLines(template)
	lines = mergeContinuations(lines)

	// Build a flat list of (indent, node) then nest by indentation.
	type entry struct {
		indent int
		n      *node
	}
	var entries []entry
	i := 0
	for i < len(lines) {
		ln := lines[i]
		if strings.TrimSpace(ln) == "" {
			i++
			continue
		}
		indent := countIndent(ln)
		content := ln[indent:]
		n, consumed, err := parseLine(content, indent, lines, i)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{indent, n})
		i += consumed
	}

	// Nest by indentation using a stack.
	var roots []*node
	type stackItem struct {
		indent int
		n      *node
	}
	var stack []stackItem
	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, e.n)
		} else {
			parent := stack[len(stack)-1].n
			parent.children = append(parent.children, e.n)
		}
		stack = append(stack, stackItem{e.indent, e.n})
	}
	return roots, nil
}

// splitLines splits on "\n", dropping a single trailing empty element so a
// template ending in "\n" does not yield a spurious blank final line.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// mergeContinuations joins lines ending in " |" (Haml's multiline marker) with
// the following continuation lines, matching the gem: each "|"-terminated line
// contributes its content (with the trailing " |" removed) and a single space
// separates the joined pieces, the whole run ending with a trailing space.
func mergeContinuations(lines []string) []string {
	var out []string
	i := 0
	for i < len(lines) {
		ln := lines[i]
		if strings.HasSuffix(strings.TrimRight(ln, " \t"), "|") && endsWithPipe(ln) {
			indent := ln[:countIndent(ln)]
			var parts []string
			for i < len(lines) && endsWithPipe(lines[i]) {
				parts = append(parts, stripPipe(lines[i]))
				i++
			}
			out = append(out, indent+strings.Join(parts, " ")+" ")
			continue
		}
		out = append(out, ln)
		i++
	}
	return out
}

// endsWithPipe reports whether a line ends with the Haml multiline marker: a
// "|" that is preceded by whitespace (or is the only content) after trimming
// trailing spaces.
func endsWithPipe(ln string) bool {
	t := strings.TrimRight(ln, " \t")
	if !strings.HasSuffix(t, "|") {
		return false
	}
	// The "|" must be a standalone marker: preceded by a space, or the line is
	// just "|". "%p A|" (no space) is not a continuation in Haml.
	body := t[:len(t)-1]
	return body == "" || strings.HasSuffix(body, " ")
}

// stripPipe removes the leading indentation and trailing " |" marker from a
// continuation line, returning the bare content.
func stripPipe(ln string) string {
	indent := countIndent(ln)
	body := ln[indent:]
	body = strings.TrimRight(body, " \t")
	body = strings.TrimSuffix(body, "|")
	return strings.TrimRight(body, " \t")
}

// countIndent returns the number of leading space/tab bytes.
func countIndent(s string) int {
	n := 0
	for n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		n++
	}
	return n
}

// parseLine parses one logical line's content (indentation removed) into a
// node. lines/idx are supplied so filter/plain blocks can consume the nested
// lines that belong to them; consumed is the number of physical lines used.
func parseLine(content string, indent int, lines []string, idx int) (n *node, consumed int, err error) {
	consumed = 1
	// content is never empty here: parse skips blank lines before calling us.
	switch content[0] {
	case '%', '.', '#':
		return parseElement(content)
	case '=':
		return parseExprLine(content, false)
	case '!':
		if strings.HasPrefix(content, "!!!") {
			return &node{kind: kindDoctype, raw: content}, 1, nil
		}
		if strings.HasPrefix(content, "!=") {
			return parseExprLine(content, true)
		}
		if strings.HasPrefix(content, "!") {
			// "! expr" unescaped output.
			return parseExprLine(content, true)
		}
	case '-':
		if strings.HasPrefix(content, "-#") {
			return &node{kind: kindSilent}, 1, nil
		}
		ctrl := strings.TrimSpace(content[1:])
		return &node{kind: kindCode, control: ctrl}, 1, nil
	case '~':
		// Preserve: like "=" but preserving newlines. We treat it as escaped
		// expression output (preserve semantics on the raw string are a runtime
		// concern handled by the eval seam's preserve helper).
		nn, _, _ := parseExprLine("="+content[1:], false)
		nn.textKind = textEscaped
		return nn, 1, nil
	case '/':
		return parseComment(content)
	case ':':
		return parseFilter(content, indent, lines, idx)
	case '\\':
		// Escaped first character: the rest is literal text.
		return &node{kind: kindText, text: content[1:], textKind: textPlain}, 1, nil
	}
	// Plain text.
	return &node{kind: kindText, text: content, textKind: textPlain}, 1, nil
}

// parseExprLine parses a "= expr" (or "!= expr") output line.
func parseExprLine(content string, unescaped bool) (*node, int, error) {
	// content starts with "=" or "!=".
	rest := content
	if strings.HasPrefix(rest, "!=") {
		rest = rest[2:]
		unescaped = true
	} else if strings.HasPrefix(rest, "=") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "!") {
		rest = rest[1:]
		unescaped = true
	}
	expr := strings.TrimSpace(rest)
	tk := textEscaped
	if unescaped {
		tk = textUnescaped
	}
	return &node{kind: kindExpr, codeExpr: expr, textKind: tk}, 1, nil
}

// parseComment parses a "/" HTML comment, including conditional comments
// "/[if IE]".
func parseComment(content string) (*node, int, error) {
	rest := strings.TrimSpace(content[1:])
	n := &node{kind: kindComment}
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end >= 0 {
			n.commentCond = rest[1:end]
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	n.text = rest
	return n, 1, nil
}

// parseFilter parses a ":name" filter and consumes its indented body block.
func parseFilter(content string, indent int, lines []string, idx int) (*node, int, error) {
	name := strings.TrimSpace(content[1:])
	n := &node{kind: kindFilter, filterName: name}
	// Consume following lines that are more-indented than the filter line.
	consumed := 1
	j := idx + 1
	// Determine the child indentation from the first non-blank following line.
	childIndent := -1
	for j < len(lines) {
		ln := lines[j]
		if strings.TrimSpace(ln) == "" {
			// Blank line inside a filter block is kept as an empty body line if
			// the block continues; decide by peeking ahead.
			n.filterBody = append(n.filterBody, "")
			consumed++
			j++
			continue
		}
		ci := countIndent(ln)
		if ci <= indent {
			break
		}
		if childIndent == -1 {
			childIndent = ci
		}
		// Strip the filter's child indentation.
		strip := childIndent
		if ci < strip {
			strip = ci
		}
		n.filterBody = append(n.filterBody, ln[strip:])
		consumed++
		j++
	}
	// Trim trailing blank body lines that were speculatively consumed.
	for len(n.filterBody) > 0 && n.filterBody[len(n.filterBody)-1] == "" {
		n.filterBody = n.filterBody[:len(n.filterBody)-1]
		consumed--
	}
	return n, consumed, nil
}

// SyntaxError reports a malformed Haml template, mirroring the gem's
// Haml::SyntaxError. Line carries the offending source line.
type SyntaxError struct {
	Line string
	Msg  string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("haml: %s: %q", e.Msg, e.Line)
}
