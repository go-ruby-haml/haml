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

  # HamlAttributes.render renders a dynamic attribute hash the way Haml does:
  # class values merged with spaces, id values merged with "_", data hashes
  # expanded to data-<k>, boolean attributes emitted bare when truthy and omitted
  # when nil/false, and every pair sorted alphabetically with escaped values.
  module HamlAttributes
    BOOL = %w[disabled readonly multiple checked selected hidden required async
              defer novalidate autofocus open reversed ismap autofocus muted
              controls loop autoplay].freeze

    def self.render(h)
      pairs = {}
      h.each do |k, v|
        k = k.to_s
        if k == 'data' && v.is_a?(Hash)
          v.each { |dk, dv| pairs["data-#{dk}"] = dv }
        elsif k == 'class'
          existing = pairs['class']
          merged = [existing, (v.is_a?(Array) ? v.join(' ') : v)].compact.join(' ')
          pairs['class'] = merged
        elsif k == 'id'
          existing = pairs['id']
          pairs['id'] = [existing, v].compact.join('_')
        else
          pairs[k] = v
        end
      end
      out = +''
      pairs.keys.sort.each do |k|
        v = pairs[k]
        if BOOL.include?(k)
          out << " #{k}" if v && v != false
        else
          next if v.nil?
          out << %( #{k}="#{Haml::Util.escape_html(v.to_s)}")
        end
      end
      out
    end
  end
end
