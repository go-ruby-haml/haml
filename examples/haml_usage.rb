# frozen_string_literal: true

# Haml template engine: compile an indentation-structured Haml template to Ruby
# source, then render it to HTML with locals bound in the render scope.
require "haml"

template = <<~HAML
  !!!
  %html
    %body
      #main.content
        %h1= title
        %ul
          - items.each do |item|
            %li= item
        %p{class: "note"} Escaped: #{"<3"}
        != "<em>raw, unescaped</em>"
HAML

engine = Haml::Template.new(template)

# render(scope, locals) evaluates the compiled source with locals bound.
html = engine.render(Object.new, title: "Fruit", items: %w[apple pear])
puts html

# The compiled Ruby source is available on #src (what the engine eval's).
puts "--- compiled source ---"
puts engine.src
