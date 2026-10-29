---
title: Home
---
::: +welcome
Welcome to **{{ site.name }}**, kept by {{ data.siteInfo.owner }} since {{ data.siteInfo.since }}.

{{ include "snippets/note.md" }}
:::

## Every page

{{ range _, entry := fleet -}}
- [{{ entry.title }}]({{ entry.url }})
{{ end }}
## News

{{ range _, entry := collections["news"] -}}
- [{{ entry.title }}]({{ entry.url }}) ({{ entry.date }})
{{ end }}
## Team

{{ range _, member := data.team.members -}}
- {{ member.name }}
{{ end -}}
