---
title: Fish & chips
social:
  name: Lucia
  first-name: L
---
# {{ title }}

Written by {{ social.name }} ({{ social["first-name"] }}), at [{{ page.url }}]({{ page.canonical }}).

{{ include "snippets/card.md" }}

Write `{{ "{{" }} title }}` to show the title.

{{ range _, entry := fleet -}}
- [{{ entry.title }}]({{ entry.url }})
{{ end -}}
