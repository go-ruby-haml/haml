package haml

import (
	"sort"
	"strings"
)

// compiler walks the node tree and accumulates Ruby source. Static output is
// coalesced into literal-string appends; dynamic output ("=", interpolation,
// "-", dynamic attributes) becomes Ruby the eval seam runs.
type compiler struct {
	bufVar   string
	escapeFn string
	format   string // "html5" (default), "xhtml" or "html4"
	src      strings.Builder
	pending  strings.Builder // coalesced static literal text awaiting flush
}

// xhtml reports whether the compiler targets the XHTML format, which self-closes
// void tags ("<br />") and expands boolean attributes ("checked=\"checked\"").
func (c *compiler) xhtml() bool { return c.format == "xhtml" }

// rstripOutput removes trailing whitespace from the output emitted so far,
// implementing Haml's whitespace-removal markers. When a coalesced static run is
// pending, its tail is trimmed in place; otherwise the last emitted output was
// dynamic, so a runtime String#rstrip! is emitted to trim the buffer tail.
func (c *compiler) rstripOutput() {
	if c.pending.Len() > 0 {
		s := strings.TrimRight(c.pending.String(), " \t\r\n\f\v")
		c.pending.Reset()
		c.pending.WriteString(s)
		return
	}
	c.src.WriteString(c.bufVar + ".rstrip!\n")
}

// compileTree emits the full Ruby program for the parsed roots.
func (c *compiler) compileTree(roots []*node) {
	c.emitNodes(roots)
	c.flush()
}

// emitNodes emits a sibling list with control-flow awareness: an "if"/"unless"/
// "case"/"begin"/... control node and its "elsif"/"else"/"when"/"in"/"rescue"/
// "ensure" continuation siblings share a single trailing "end", exactly as Haml
// nests them. Each control node emits its keyword line and its own children; the
// closing "end" is deferred until the continuation chain finishes.
func (c *compiler) emitNodes(nodes []*node) {
	i := 0
	for i < len(nodes) {
		n := nodes[i]
		if n.kind == kindCode && opensBlock(n.control) {
			// Emit the whole if/elsif/else (or case/when, begin/rescue) chain.
			c.emitRuby(n.control)
			c.emitNodes(n.children)
			i++
			for i < len(nodes) && nodes[i].kind == kindCode && isContinuation(nodes[i].control) {
				c.emitRuby(nodes[i].control)
				c.emitNodes(nodes[i].children)
				i++
			}
			c.emitRuby("end")
			continue
		}
		c.emit(n)
		i++
	}
}

// pushStatic appends literal HTML to the pending static run.
func (c *compiler) pushStatic(s string) { c.pending.WriteString(s) }

// flush writes any pending static run as a single buffer append.
func (c *compiler) flush() {
	if c.pending.Len() == 0 {
		return
	}
	c.src.WriteString(c.bufVar + " << " + rubyDump(c.pending.String()) + "\n")
	c.pending.Reset()
}

// emitRuby writes a raw Ruby statement line, flushing pending static output
// first so ordering is preserved.
func (c *compiler) emitRuby(stmt string) {
	c.flush()
	c.src.WriteString(stmt + "\n")
}

// emit dispatches on node kind.
func (c *compiler) emit(n *node) {
	switch n.kind {
	case kindElement:
		c.emitElement(n)
	case kindText:
		c.emitText(n)
	case kindExpr:
		c.emitExpr(n.codeExpr, n.textKind, true)
	case kindCode:
		c.emitControl(n)
	case kindComment:
		c.emitComment(n)
	case kindSilent:
		// discarded
	case kindFilter:
		c.emitFilter(n)
	case kindDoctype:
		c.emitDoctype(n)
	}
}

// emitDoctype resolves a "!!!" line to the exact doctype string for the
// configured format. A doctype that resolves to the empty string (e.g. "!!! XML"
// outside xhtml) emits nothing at all, not even a newline, matching the gem. An
// invalid doctype name is silently ignored (the gem raises at compile time, but
// well-formed templates never reach here — parse validates the "!!!" prefix).
func (c *compiler) emitDoctype(n *node) {
	s, err := resolveDoctype(n.raw, c.format)
	if err != nil || s == "" {
		return
	}
	c.pushStatic(s + "\n")
}

// emitText emits a plain-text node, honouring "#{}" interpolation. Literal runs
// coalesce into the static buffer; each interpolated expression is HTML-escaped
// by default (the gem's behaviour), or left raw for a "!"-marked node.
func (c *compiler) emitText(n *node) {
	c.emitInterpText(n.text, !n.noEscape)
	c.pushStatic("\n")
}

// emitInterpText appends literal text with "#{}" interpolation. Each literal run
// is emitted as coalesced static output; each interpolation is emitted as a Ruby
// append, HTML-escaped when escape is true. This matches how the gem escapes only
// the interpolated values, never the surrounding literal bytes.
func (c *compiler) emitInterpText(text string, escape bool) {
	i := 0
	for i < len(text) {
		if text[i] == '#' && i+1 < len(text) && text[i+1] == '{' {
			expr, next := scanInterp(text, i)
			c.emitInterpExpr(expr, escape)
			i = next
			continue
		}
		start := i
		for i < len(text) && !(text[i] == '#' && i+1 < len(text) && text[i+1] == '{') {
			i++
		}
		c.pushStatic(text[start:i])
	}
}

// emitInterpExpr emits a single interpolated expression append, escaped or raw.
func (c *compiler) emitInterpExpr(expr string, escape bool) {
	if escape {
		c.emitRuby(c.bufVar + " << " + c.escapeFn + "((" + expr + ").to_s)")
		return
	}
	c.emitRuby(c.bufVar + " << (" + expr + ").to_s")
}

// scanInterp returns the Ruby expression inside the "#{ ... }" beginning at
// text[i] (with balanced braces) and the index just past the closing brace. An
// unterminated interpolation yields the remaining text.
func scanInterp(text string, i int) (expr string, next int) {
	depth := 0
	j := i
	for j < len(text) {
		switch text[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[i+2 : j], j + 1
			}
		}
		j++
	}
	return text[i+2:], len(text)
}

// emitExpr emits an "=" / "!=" expression. standalone marks a block-level line
// (which gets its own trailing newline); inline expressions used as an
// element's content pass standalone=false so the caller controls newlines.
func (c *compiler) emitExpr(expr string, tk textKind, standalone bool) {
	var appended string
	switch tk {
	case textUnescaped:
		appended = "(" + expr + ").to_s"
	default:
		appended = c.escapeFn + "((" + expr + ").to_s)"
	}
	nl := ""
	if standalone {
		nl = `; ` + c.bufVar + ` << "\n"`
	}
	c.emitRuby(c.bufVar + " << " + appended + nl)
}

// emitControl emits a non-block "-" control line (e.g. "- x = 5"). Every
// block-opening control ("- if", "- ... do") is intercepted by emitNodes, which
// manages the shared trailing "end", so this path only handles plain statements
// and their (rare) nested output.
func (c *compiler) emitControl(n *node) {
	c.emitRuby(n.control)
	c.emitNodes(n.children)
}

// emitComment emits an HTML comment "/" or a conditional comment "/[cond]".
func (c *compiler) emitComment(n *node) {
	hasChildren := len(n.children) > 0
	if n.commentCond != "" {
		if hasChildren {
			c.pushStatic("<!--[" + n.commentCond + "]>\n")
			c.emitNodes(n.children)
			c.pushStatic("<![endif]-->\n")
		} else {
			c.pushStatic("<!--[" + n.commentCond + "]> " + n.text + " <![endif]-->\n")
		}
		return
	}
	if hasChildren {
		c.pushStatic("<!--\n")
		c.emitNodes(n.children)
		c.pushStatic("-->\n")
	} else if n.text != "" {
		c.pushStatic("<!-- " + n.text + " -->\n")
	} else {
		c.pushStatic("<!--\n-->\n")
	}
}

// emitElement emits an element node and its subtree, applying the ">" (remove
// outer whitespace) and "<" (remove inner whitespace) markers exactly as the gem
// does: ">" strips the whitespace preceding the tag and suppresses the newline
// after its close; "<" suppresses the newline after the open tag and strips the
// whitespace before the close.
func (c *compiler) emitElement(n *node) {
	if n.nuke.outer {
		c.rstripOutput()
	}
	open, closeTag, void := c.renderTag(n)
	c.pushStatic(open)

	outerNL := "\n"
	if n.nuke.outer {
		outerNL = ""
	}
	if void {
		c.pushStatic(outerNL)
		return
	}

	hasChildren := len(n.children) > 0
	hasInline := n.text != ""

	switch {
	case hasInline && n.text == "\x00expr":
		// Inline expression content: <tag>EXPR</tag> on one line.
		c.emitExpr(n.codeExpr, n.textKind, false)
		c.pushStatic(closeTag + outerNL)
	case hasInline:
		// Inline literal/interpolated text.
		c.emitInterpText(n.text, !n.noEscape)
		c.pushStatic(closeTag + outerNL)
	case hasChildren:
		if !n.nuke.inner {
			c.pushStatic("\n")
		}
		c.emitNodes(n.children)
		if n.nuke.inner {
			c.rstripOutput()
		}
		c.pushStatic(closeTag + outerNL)
	default:
		c.pushStatic(closeTag + outerNL)
	}
}

// renderTag builds the opening tag string (with resolved static attributes),
// the closing tag string, and whether the element is a void/self-closing tag.
// When the element has dynamic attributes, the opening tag is split so a Ruby
// attribute-render call is spliced in; renderTag handles the static case and
// emitElement's caller relies on pushStatic/emitRuby ordering — to keep it
// simple we resolve dynamic attributes here by emitting directly.
func (c *compiler) renderTag(n *node) (open, closeTag string, void bool) {
	void = isVoidTag(n.tag) || n.selfClose
	closeTag = "</" + n.tag + ">"
	closeAngle := ">"
	if void && c.xhtml() {
		closeAngle = " />"
	}

	if n.dynAttrRB == "" && n.objectRef == "" {
		return "<" + n.tag + c.renderStaticAttrs(n) + closeAngle, closeTag, void
	}
	// Dynamic attributes and/or an object reference: the whole attribute set
	// (including any static shorthand class/id) is rendered at eval time. Emit
	// "<tag", then a Ruby call that renders the merged hashes, then the close.
	c.pushStatic("<" + n.tag)
	c.emitRuby(c.bufVar + " << " + c.renderDynAttrCall(n))
	return closeAngle, closeTag, void
}

// renderDynAttrCall builds the Ruby expression that renders the element's
// dynamic attributes at eval time via the runtime helper the host provides
// (::Haml::HamlAttributes.render). Static shorthand classes/ids are folded into
// the first hash, an explicit attribute hash follows, and an object reference is
// appended as a final Haml::ObjectRef.parse hash. The helper accumulates
// class/id across every hash in order, matching the gem's attribute merging. The
// format is passed so boolean/void rendering matches the selected format.
func (c *compiler) renderDynAttrCall(n *node) string {
	var hashes []string

	// The static attributes (shorthand .class/#id always; literal hash attributes
	// only when no raw dynamic hash already carries them) form a leading hash. A
	// non-empty dynamic hash and an object reference follow as separate hashes so
	// the runtime helper accumulates class/id across them instead of a Ruby hash
	// literal colliding on a duplicate key.
	var classes, ids []string
	type kv struct{ name, val string }
	var others []kv
	includeLiteral := n.dynAttrRB == ""
	for _, sa := range n.staticAttr {
		switch {
		case sa.classShorthand:
			classes = append(classes, sa.value)
		case sa.idShorthand:
			ids = append(ids, sa.value)
		case includeLiteral:
			switch sa.name {
			case "class":
				classes = append(classes, strings.Fields(sa.value)...)
			case "id":
				ids = append(ids, sa.value)
			default:
				others = append(others, kv{sa.name, staticAttrRubyValue(sa)})
			}
		}
	}
	var entries []string
	if len(classes) > 0 {
		entries = append(entries, "class: "+rubyStrLit(strings.Join(classes, " ")))
	}
	if len(ids) > 0 {
		entries = append(entries, "id: "+rubyStrLit(strings.Join(ids, "_")))
	}
	for _, o := range others {
		entries = append(entries, rubyStrLit(o.name)+" => "+o.val)
	}
	if len(entries) > 0 {
		hashes = append(hashes, "{"+strings.Join(entries, ", ")+"}")
	}
	if n.dynAttrRB != "" {
		hashes = append(hashes, "{"+n.dynAttrRB+"}")
	}
	if n.objectRef != "" {
		hashes = append(hashes, "::Haml::ObjectRef.parse(["+n.objectRef+"])")
	}
	return "::Haml::HamlAttributes.render(" + rubyStrLit(c.format) + ", " + strings.Join(hashes, ", ") + ")"
}

// staticAttrRubyValue renders a resolved static attribute as the Ruby literal to
// splice into a dynamic attribute hash: booleans as true/false, an explicit nil,
// and everything else as a quoted string.
func staticAttrRubyValue(sa staticAttr) string {
	if sa.isBool {
		if sa.value == "\x00nil" {
			return "nil"
		}
		if sa.boolVal {
			return "true"
		}
		return "false"
	}
	return rubyStrLit(sa.value)
}

// renderStaticAttrs resolves the element's static attributes into an attribute
// string in Haml's canonical order: alphabetical by name, class values merged
// with spaces, id values merged with "_". Boolean-true attributes render as
// bare names; boolean-false/nil ones are omitted (for known boolean attrs) or
// rendered as name="" (for value attrs whose value is nil).
func (c *compiler) renderStaticAttrs(n *node) string {
	if len(n.staticAttr) == 0 {
		return ""
	}
	classes := []string{}
	ids := []string{}
	type kv struct {
		name    string
		val     string
		boolean bool
	}
	var others []kv
	seen := map[string]int{} // name -> index in others (last wins for value attrs)

	for _, sa := range n.staticAttr {
		switch sa.name {
		case "class":
			classes = append(classes, strings.Fields(sa.value)...)
		case "id":
			ids = append(ids, sa.value)
		default:
			if sa.isBool {
				if sa.value == "\x00nil" {
					// nil on a non-boolean attr => name="" ; on a boolean attr => omit.
					if isBooleanAttr(sa.name) {
						continue
					}
					if idx, ok := seen[sa.name]; ok {
						others[idx] = kv{sa.name, "", false}
					} else {
						seen[sa.name] = len(others)
						others = append(others, kv{sa.name, "", false})
					}
					continue
				}
				if sa.boolVal {
					if idx, ok := seen[sa.name]; ok {
						others[idx] = kv{sa.name, "", true}
					} else {
						seen[sa.name] = len(others)
						others = append(others, kv{sa.name, "", true})
					}
				}
				// boolean-false: omit.
			} else {
				if idx, ok := seen[sa.name]; ok {
					others[idx] = kv{sa.name, sa.val(), false}
				} else {
					seen[sa.name] = len(others)
					others = append(others, kv{sa.name, sa.val(), false})
				}
			}
		}
	}

	type outAttr struct {
		name    string
		val     string
		boolean bool
	}
	var out []outAttr
	if len(classes) > 0 {
		out = append(out, outAttr{"class", strings.Join(classes, " "), false})
	}
	if len(ids) > 0 {
		out = append(out, outAttr{"id", strings.Join(ids, "_"), false})
	}
	for _, o := range others {
		out = append(out, outAttr{o.name, o.val, o.boolean})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })

	var b strings.Builder
	for _, o := range out {
		if o.boolean {
			// html5/html4 render a bare boolean attribute; xhtml expands it to
			// name="name".
			if c.xhtml() {
				b.WriteString(" " + o.name + `="` + o.name + `"`)
			} else {
				b.WriteString(" " + o.name)
			}
		} else {
			b.WriteString(" " + o.name + `="` + attrEscape(o.val) + `"`)
		}
	}
	return b.String()
}

// val returns the raw attribute value; a nil-marker value renders as empty.
func (sa staticAttr) val() string {
	if sa.value == "\x00nil" {
		return ""
	}
	return sa.value
}

// emitFilter emits a ":name" filter block. Static filters (:plain, :css,
// :javascript, :escaped, :preserve) produce literal output; :ruby runs the body
// as code.
func (c *compiler) emitFilter(n *node) {
	body := strings.Join(n.filterBody, "\n")
	switch n.filterName {
	case "plain":
		c.pushStatic(interpolateStatic(body) + "\n")
	case "escaped":
		c.pushStatic(HTMLEscape(interpolateStatic(body)) + "\n")
	case "preserve":
		// Every newline in the block — including the final one — is preserved as
		// the &#x000A; entity, matching the gem.
		c.pushStatic(strings.ReplaceAll(body+"\n", "\n", "&#x000A;") + "\n")
	case "javascript":
		c.pushStatic("<script>\n" + indentBody(body) + "\n</script>\n")
	case "css":
		c.pushStatic("<style>\n" + indentBody(body) + "\n</style>\n")
	case "ruby":
		for _, line := range n.filterBody {
			c.emitRuby(line)
		}
	default:
		// Unknown filter: emit the raw body (best-effort), matching :plain.
		c.pushStatic(body + "\n")
	}
}

// indentBody re-indents a filter body by two spaces per line the way Haml's
// :javascript / :css filters wrap their content.
func indentBody(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}

// interpolateStatic returns body unchanged for the static-only compile path;
// "#{}" interpolation inside filters is a runtime concern handled by the eval
// seam and is left literal here.
func interpolateStatic(body string) string { return body }
