# haml examples

Runnable pure-Ruby usage of the `haml` template engine, verified under the
[rbgo](https://github.com/go-embedded-ruby/ruby) interpreter.

```sh
rbgo examples/haml_usage.rb
```

| File | Shows |
| --- | --- |
| `haml_usage.rb` | Compiling a Haml template with `Haml::Template.new`, rendering to HTML via `#render(scope, locals)` with bound locals, tag/`.class`/`#id` shorthand, the doctype, `- do`/`= expr` embedded Ruby, escaped vs. raw (`!=`) output, and inspecting the compiled source with `#src`. |
