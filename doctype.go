package haml

import "strings"

// doctypeTables mirrors Temple::HTML::Fast::DOCTYPES (the table the `haml` gem
// resolves a "!!!" line against), keyed by the internal format the gem maps a
// Haml format onto: html5/html4 -> "html", xhtml -> "xml".
var doctypeTables = map[string]map[string]string{
	"xml": {
		"1.1":          `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`,
		"5":            `<!DOCTYPE html>`,
		"html":         `<!DOCTYPE html>`,
		"strict":       `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`,
		"frameset":     `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Frameset//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-frameset.dtd">`,
		"mobile":       `<!DOCTYPE html PUBLIC "-//WAPFORUM//DTD XHTML Mobile 1.2//EN" "http://www.openmobilealliance.org/tech/DTD/xhtml-mobile12.dtd">`,
		"basic":        `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML Basic 1.1//EN" "http://www.w3.org/TR/xhtml-basic/xhtml-basic11.dtd">`,
		"transitional": `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">`,
		"svg":          `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">`,
	},
	"html": {
		"5":            `<!DOCTYPE html>`,
		"html":         `<!DOCTYPE html>`,
		"strict":       `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`,
		"frameset":     `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Frameset//EN" "http://www.w3.org/TR/html4/frameset.dtd">`,
		"transitional": `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">`,
	},
}

const rdfaDoctype = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML+RDFa 1.0//EN" "http://www.w3.org/MarkUp/DTD/xhtml-rdfa-1.dtd">`

// resolveDoctype turns a "!!!" doctype line into the exact string the gem emits
// for the given format, mirroring Haml::Compiler::DoctypeCompiler +
// Temple::HTML::Fast#on_html_doctype. An empty return means the gem emits no
// output at all (e.g. "!!! XML" under a non-xhtml format), which the caller must
// distinguish from a normal doctype so it also emits no trailing newline. err is
// non-nil for an unknown doctype name (the gem raises Temple::FilterError).
func resolveDoctype(line, format string) (string, error) {
	body := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "!!!")))
	version, typ := parseDoctypeSpec(body)
	table := "html"
	if format == "xhtml" {
		table = "xml"
	}
	switch typ {
	case "":
		return htmlDoctype(version, format, table), nil
	case "xml":
		if format == "xhtml" {
			return "<?xml version='1.0' encoding='utf-8' ?>", nil
		}
		return "", nil
	case "rdfa":
		return rdfaDoctype, nil
	default:
		if s, ok := doctypeTables[table][typ]; ok {
			return s, nil
		}
		return "", &SyntaxError{Line: line, Msg: "invalid doctype " + typ}
	}
}

// htmlDoctype resolves a version-only (no named type) doctype, mirroring
// DoctypeCompiler#html_doctype: html5 collapses every version to "<!DOCTYPE
// html>", html4 always emits transitional, and xhtml honours the version (a
// missing version defaulting to transitional).
func htmlDoctype(version, format, table string) string {
	switch format {
	case "html4":
		return doctypeTables["html"]["transitional"]
	case "xhtml":
		if version == "" {
			return doctypeTables["xml"]["transitional"]
		}
		return doctypeTables["xml"][version]
	default: // html5
		return doctypeTables["html"]["html"]
	}
}

// parseDoctypeSpec extracts the version (a leading digit token like "5" or
// "1.1") and the type name (a lowercased alphabetic token like "strict"/"xml"/
// "rdfa") from a doctype line body, mirroring Haml's DOCTYPE_REGEX.
func parseDoctypeSpec(body string) (version, typ string) {
	body = strings.TrimSpace(body)
	i := 0
	// Optional leading version: a digit optionally followed by ".digit".
	if i < len(body) && body[i] >= '0' && body[i] <= '9' {
		start := i
		i++
		if i+1 < len(body) && body[i] == '.' && body[i+1] >= '0' && body[i+1] <= '9' {
			i += 2
		}
		version = body[start:i]
	}
	for i < len(body) && body[i] == ' ' {
		i++
	}
	// Optional type: a run of ASCII letters.
	start := i
	for i < len(body) && (body[i] >= 'a' && body[i] <= 'z') {
		i++
	}
	typ = body[start:i]
	return version, typ
}
