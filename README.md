<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-haml/brand/main/social/go-ruby-haml-haml.png" alt="go-ruby-haml/haml" width="720"></p>

# haml — go-ruby-haml

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-haml.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the Ruby [Haml](https://haml.info)
template engine** (the `haml` gem) — the deterministic, interpreter-independent
core that turns an indentation-structured Haml template into the **Ruby source
that renders it**, producing the same HTML the gem produces for the same locals.

It is the Haml backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime.

> **What it is — and isn't.** Compiling a template to Ruby source (indentation
> nesting, `%tag`/`.class`/`#id` shorthand, attribute hashes, filters, comments,
> the doctype) is fully deterministic and needs **no interpreter**, so it lives
> here as pure Go. The final `eval(compiled_src)` that runs any embedded Ruby
> (`=` expressions, `-` control, `#{}` interpolation, dynamic attribute hashes)
> **does** need a Ruby interpreter and stays in the consumer (e.g. rbgo) — this
> library **compiles**, the host **evaluates**. This mirrors the sibling
> [go-ruby-erb](https://github.com/go-ruby-erb/erb) design exactly.

Everything static — element structure, literal attributes, `.class`/`#id`
shorthand, the doctype, HTML/conditional comments, and the `:plain`/`:css`/
`:javascript`/`:escaped`/`:preserve` filters — is resolved **at compile time**
into literal HTML runs, so a template with no embedded Ruby renders with **no
interpreter at all**.

## Features

Validated against the `haml` gem (7.x, Ruby ≥ 4.0) on every supported platform:

- **Elements & shorthand** — `%tag`, `.class`/`#id` (div default),
  `%tag.c1.c2#id`, class-merge (space) and id-merge (`_`) between shorthand and
  attribute hashes.
- **Attributes** — Ruby-hash `%a{href: "x"}` and HTML-style `%a(href="x")`;
  symbol keys, hashrocket keys, `data:` nested-hash expansion to `data-*`,
  numeric/string/`true`/`false`/`nil` literals, boolean attributes (bare when
  truthy, omitted when falsy), alphabetical ordering, escaped values. Non-literal
  values are handled at eval time via `::Haml::HamlAttributes.render`.
- **Content** — inline text, `=` (HTML-escaped Ruby), `!=`/`&=` (unescaped),
  `~` (preserve), `-` (control, no output), `#{}` interpolation, `\` escape,
  `|` multiline continuation.
- **Control flow** — `- if/elsif/else`, `- case/when`, `- begin/rescue/ensure`
  and `- … do |x|` blocks nest correctly and share a single emitted `end`.
- **Filters** — `:plain`, `:javascript`, `:css`, `:escaped`, `:preserve`,
  `:ruby`.
- **Comments & doctype** — HTML comments `/`, silent comments `-#`, conditional
  comments `/[if IE]`, `!!!` doctype.
- **Void / self-closing** — the HTML5 void set (`br`, `img`, `input`, …) and the
  explicit `%tag/` marker render as `<tag>` with no content.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

### Deferred, honestly

The `>`/`<` whitespace-removal markers parse without error but their
surrounding-whitespace trimming is not yet applied to the emitted output (the
element structure still compiles correctly). Everything else in the feature list
above matches the gem's rendered HTML byte-for-byte in the test corpus.

## Install

```sh
go get github.com/go-ruby-haml/haml
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-haml/haml"
)

func main() {
	src, err := haml.Compile("%p= name\n", haml.Options{})
	if err != nil {
		panic(err)
	}
	fmt.Println(src)
	// _hamlout = ::String.new
	// _hamlout << "<p>"
	// _hamlout << ::Haml::Util.escape_html((name).to_s)
	// _hamlout << "</p>\n"
	// _hamlout
	//
	// Hand it to a Ruby interpreter with `name` in scope:
	//   name = "World"; eval(src)  ->  "<p>World</p>\n"

	// A fully-static template compiles to a single literal append — no interpreter needed:
	src, _ = haml.Compile(".card\n  %h1 Title\n  %p Body", haml.Options{})
	// _hamlout << "<div class=\"card\">\n<h1>Title</h1>\n<p>Body</p>\n</div>\n"
}
```

Render through a pluggable evaluator (the rbgo seam):

```go
out, err := haml.Render("%p= greeting",
	map[string]string{"greeting": `"hi"`},
	func(rubySrc string, locals map[string]string) (string, error) {
		// go-embedded-ruby/rbgo binds the locals and eval's rubySrc here.
		return rbgo.Eval(rubySrc, locals)
	})
```

## API

```go
type Options struct {
	BufVar   string // output-buffer var name; default "_hamlout"
	EscapeFn string // Ruby escape helper for "="; default "::Haml::Util.escape_html"
}

// Compile returns the Ruby source that, when eval'd with the template's locals
// in scope, builds and returns the rendered HTML string, matching the `haml` gem.
func Compile(template string, opts Options) (src string, err error)

// Render = Compile + a pluggable Ruby-eval seam (nil eval returns the source).
func Render(template string, locals map[string]string, eval Evaluator) (string, error)
type Evaluator func(rubySource string, locals map[string]string) (string, error)

func HTMLEscape(s string) string // Haml::Util.escape_html

// SyntaxError mirrors the gem's Haml::SyntaxError (e.g. an unterminated
// attribute list).
type SyntaxError struct{ Line, Msg string }
```

### What the host (rbgo) provides at eval time

The compiled source references two runtime symbols the host supplies:

- `::Haml::Util.escape_html(s)` — the five-character HTML escape (overridable via
  `Options.EscapeFn`);
- `::Haml::HamlAttributes.render(hash)` — renders a **dynamic** attribute hash
  (class/id merge, `data:` expansion, boolean handling, alphabetical order).

The reference implementations used by the differential oracle live in
[`testdata/prelude.rb`](testdata/prelude.rb).

## Tests & coverage

The suite includes a **differential oracle**: a wide template corpus (elements,
shorthand, static & dynamic attributes, filters, comments, control flow,
interpolation, multiline) is rendered both by the system `haml` gem and by
eval'ing our compiled source under the reference prelude, comparing the HTML
**byte-for-byte**. The deterministic, ruby-free golden-source tests alone hold
coverage at **100%**, so the no-ruby lanes still pass the gate.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

The oracle tests skip themselves where `ruby`/`haml` is not available (e.g. the
qemu arch lanes), so the cross-arch builds still validate the compiler itself.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-haml/haml authors.
