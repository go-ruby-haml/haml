package haml

import "strings"

// htmlEscapeSet are the characters Haml's Haml::Util.escape_html replaces when
// escaping interpolated content (the `=`, `&=`, escaped filters and default
// interpolation path). It maps &, <, >, " and ' to their entity references,
// matching the gem's Erubi-style table exactly (' becomes &#39;).
var htmlEscapeReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

// HTMLEscape replaces the five HTML-significant characters with their entity
// references, matching Haml::Util.escape_html exactly (note "'" becomes
// "&#39;"). It is exposed so a host embedding the compiled source can provide
// the runtime escape helper the emitted Ruby calls.
func HTMLEscape(s string) string {
	if !strings.ContainsAny(s, "&<>\"'") {
		return s
	}
	return htmlEscapeReplacer.Replace(s)
}

// attrEscapeReplacer escapes an attribute value. Haml escapes attribute values
// with the same five-character table as content, so a double-quoted attribute
// value never terminates early and entities render identically to the gem.
var attrEscapeReplacer = htmlEscapeReplacer

// attrEscape escapes an attribute value string the way Haml renders static
// attribute values.
func attrEscape(s string) string {
	if !strings.ContainsAny(s, "&<>\"'") {
		return s
	}
	return attrEscapeReplacer.Replace(s)
}
