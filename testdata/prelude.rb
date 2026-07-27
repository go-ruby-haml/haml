# prelude.rb — the minimal runtime the compiled Haml source expects a host to
# provide. go-embedded-ruby/rbgo ships the production versions; this file is the
# reference used by the differential oracle test to eval our compiled source and
# compare its rendered HTML against the `haml` gem.
module Haml
  module Util
    # escape_html mirrors Haml::Util.escape_html: the five-character HTML entity
    # table (' becomes &#39;).
    def self.escape_html(s)
      s.to_s.gsub(/[&<>"']/, '&' => '&amp;', '<' => '&lt;', '>' => '&gt;',
                  '"' => '&quot;', "'" => '&#39;')
    end
  end

  # ObjectRef mirrors Haml::ObjectRef: it derives the class/id attributes of an
  # element written with the "[obj]" / "[obj, prefix]" object-reference syntax.
  module ObjectRef
    def self.parse(args)
      object, prefix = args
      return {} unless object
      suffix = object.respond_to?(:haml_object_ref) ? object.haml_object_ref : underscore(object.class)
      {
        'class' => [prefix, suffix].compact.join('_'),
        'id'    => [prefix, suffix, object.id || 'new'].compact.join('_'),
      }
    end

    def self.underscore(camel_cased_word)
      word = camel_cased_word.to_s.dup
      word.gsub!(/::/, '_')
      word.gsub!(/([A-Z]+)([A-Z][a-z])/, '\1_\2')
      word.gsub!(/([a-z\d])([A-Z])/, '\1_\2')
      word.tr!('-', '_')
      word.downcase!
      word
    end
  end

  # HamlAttributes.render renders a set of dynamic attribute hashes the way Haml
  # does: class values merged with spaces, id values merged with "_", data hashes
  # expanded to data-<k>, boolean attributes emitted bare (html) or as
  # name="name" (xhtml) when truthy and omitted when nil/false, and every pair
  # sorted alphabetically with escaped values. The class/id merge accumulates
  # across every hash in argument order, matching the gem's attribute merging.
  module HamlAttributes
    BOOL = %w[disabled readonly multiple checked selected hidden required async
              defer novalidate autofocus open reversed ismap muted controls loop
              autoplay allowfullscreen default inert itemscope pubdate scoped
              seamless truespeed formnovalidate autobuffer download].freeze

    def self.render(format, *hashes)
      xhtml = (format == 'xhtml')
      pairs = {}
      hashes.each do |h|
        h.each do |k, v|
          k = k.to_s
          if k == 'data' && v.is_a?(Hash)
            v.each { |dk, dv| pairs["data-#{dk}"] = dv }
          elsif k == 'class'
            val = v.is_a?(Array) ? v.compact.join(' ') : v
            pairs['class'] = [pairs['class'], val].compact.reject { |x| x.to_s.empty? }.join(' ')
          elsif k == 'id'
            pairs['id'] = [pairs['id'], v].compact.reject { |x| x.to_s.empty? }.join('_')
          else
            pairs[k] = v
          end
        end
      end
      out = +''
      pairs.keys.sort.each do |k|
        v = pairs[k]
        if BOOL.include?(k)
          next unless v && v != false
          out << (xhtml ? %( #{k}="#{k}") : " #{k}")
        else
          next if v.nil?
          out << %( #{k}="#{Haml::Util.escape_html(v.to_s)}")
        end
      end
      out
    end
  end
end
