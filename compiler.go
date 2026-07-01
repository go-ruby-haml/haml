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
	src      strings.Builder
	pending  strings.Builder // coalesced static literal text awaiting flush
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
		c.pushStatic("<!DOCTYPE html>\n")
	}
}

// emitText emits a plain-text node: literal text, honouring "#{}" interpolation
// by emitting an interpolated (unescaped) Ruby string when present.
func (c *compiler) emitText(n *node) {
	if strings.Contains(n.text, "#{") {
		c.emitRuby(c.bufVar + " << " + rubyInterp(n.text) + "; " + c.bufVar + ` << "\n"`)
		return
	}
	c.pushStatic(n.text + "\n")
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

// emitElement emits an element node and its subtree.
func (c *compiler) emitElement(n *node) {
	open, closeTag, void := c.renderTag(n)
	c.pushStatic(open)
	if void {
		c.pushStatic("\n")
		return
	}

	hasChildren := len(n.children) > 0
	hasInline := n.text != ""

	switch {
	case hasInline && n.text == "\x00expr":
		// Inline expression content: <tag>EXPR</tag>\n on one line.
		c.emitExpr(n.codeExpr, n.textKind, false)
		c.pushStatic(closeTag + "\n")
	case hasInline:
		// Inline literal/interpolated text.
		if strings.Contains(n.text, "#{") {
			c.emitRuby(c.bufVar + " << " + rubyInterp(n.text))
			c.pushStatic(closeTag + "\n")
		} else {
			c.pushStatic(n.text + closeTag + "\n")
		}
	case hasChildren:
		c.pushStatic("\n")
		c.emitNodes(n.children)
		c.pushStatic(closeTag + "\n")
	default:
		c.pushStatic(closeTag + "\n")
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

	if n.dynAttrRB == "" {
		return "<" + n.tag + c.renderStaticAttrs(n) + ">", closeTag, void
	}
	// Dynamic attributes: the whole attribute set (including any static shorthand
	// class/id, which renderDynAttrCall folds into the hash) is rendered at eval
	// time. Emit "<tag", then a Ruby call that renders the merged hash, then ">".
	c.pushStatic("<" + n.tag)
	c.emitRuby(c.bufVar + " << " + c.renderDynAttrCall(n))
	return ">", closeTag, void
}

// renderDynAttrCall builds the Ruby expression that renders the element's
// dynamic attribute hash at eval time via the runtime helper the host provides
// (Haml.render_attributes). Static shorthand classes/ids are merged in.
func (c *compiler) renderDynAttrCall(n *node) string {
	var pre []string
	for _, sa := range n.staticAttr {
		if sa.classShorthand {
			pre = append(pre, "class: "+rubyStrLit(sa.value))
		} else if sa.idShorthand {
			pre = append(pre, "id: "+rubyStrLit(sa.value))
		}
	}
	hash := n.dynAttrRB
	if len(pre) > 0 {
		hash = strings.Join(pre, ", ") + ", " + hash
	}
	return "::Haml::HamlAttributes.render({" + hash + "})"
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
			b.WriteString(" " + o.name)
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
		c.pushStatic(strings.ReplaceAll(body, "\n", "&#x000A;") + "\n")
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
