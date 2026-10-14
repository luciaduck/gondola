# 🛶 Gondola — Technical Specification

**Tagline:** *11ty-inspired Go SSG designed for quick configuration | Let your site take a gondola ride.*

---

## 1. Vision & Architecture Overview

**Gondola** is a Go-based Static Site Generator (SSG) built with a "configuration over convention" philosophy[cite: 1]. Inspired by Eleventy (11ty), it brings high flexibility and explicit configuration while leveraging the performance and build speed of the Go ecosystem.

It is distributed as a ready-to-use **Docker** image containing the pre-compiled `dart-sass` binary, eliminating any external Node.js/npm dependencies.

```
       +-------------------------------------------------------+
       |                      gondola.yaml                     |
       +-------------------------------------------------------+
                                   |
                                   v
+------------------+     +-------------------+     +------------------+
|   Content (.md)  | --> |     Pipeline      | --> | Output (_site)   |
| Frontmatter+Text |     | Markdown (goldmark|     | Processed HTML   |
+------------------+     |  + Jet Templates) |     +------------------+
                         +-------------------+
                                   |
       +---------------------------+---------------------------+
       |                           |                           |
       v                           v                           v
+--------------+           +---------------+           +---------------+
|   Squero     |           |  godartsass   |           |    Sandolo    |
| (Fenced-bl.) |           |  (Dart Sass)  |           | (Dev Server)  |
+--------------+           +---------------+           +---------------+
```

---

## 2. Dependency Stack

| Module / Tool | Architectural Role |
| :--- | :--- |
| **`dart-sass`** *(Docker)* | Native Sass compiler pre-installed in the Docker container runtime. |
| **`bep/godartsass`** | Go bindings for the Dart Sass binary to compile SCSS/Sass assets. |
| **`CloudyKit/jet`** | Primary template engine[cite: 1], chosen for fast execution and expressive syntax. |
| **`yuin/goldmark`** | Fast and compliant Markdown parser converting content into HTML[cite: 1]. |
| **`adrg/frontmatter`** | Markdown frontmatter and `gondola.yaml` configuration parser. |
| **`gosimple/slug`** | Slug generator for clean permalinks derived from titles or paths. |
| **`luciaduck/squero`** | Custom handler for fenced blocks and `.squero` aliases[cite: 1]. |
| **`luciaduck/sandolo`** | Built-in HTTP server for local development and file serving[cite: 1]. |

---

## 3. Data Flow & Build Pipeline

### Phase 1: Configuration Loading
1. Parses `gondola.yaml` at startup using `adrg/frontmatter`.
2. Initializes global configuration structures (`config`, `input`, `output`, `default`).

### Phase 2: Asset Pipeline (Sass)
1. If `config.sass: true`, Gondola triggers `bep/godartsass` targeting partials defined in `input.partials` (e.g., `_components/sass`)[cite: 1].
2. Compiled CSS assets are written directly to `output.directory`[cite: 1].

### Phase 3: Content & Template Rendering
1. Scans Markdown files in `input.content`[cite: 1].
2. Extracts frontmatter (via `adrg/frontmatter`). Fallback defaults from the `default` section are applied if keys are missing (e.g., `title`, `author`, `date`, `permalink`, `sitemap`)[cite: 1].
3. Pre-processes custom extensions (if `config.fenced-blocks: true`, passes content to **`squero`** to process `.squero` aliases)[cite: 1].
4. Renders Markdown body using **`goldmark`** (if `config.markdown: true`)[cite: 1].
5. Wraps rendered content in **`jet`** templates located in `input.templates`, injecting global data from `input.data`[cite: 1].

### Phase 4: Routing & Output
1. Computes destination path based on `output.routing` (`file` vs `directory`)[cite: 1] and generates permalinks using `gosimple/slug` or `default.permalink`[cite: 1].
2. Writes final HTML files to `output.directory` (default: `_site`)[cite: 1].
3. Copies raw static assets specified in `output.passthrough` directly to output[cite: 1].
4. Generates `sitemap.xml` if `config.sitemap: true` and `page.sitemap != false`[cite: 1].

### Phase 5: Local Development Server (Sandolo)
1. When launched in serve mode (`config.serve: true`), Gondola spins up the **`sandolo`** HTTP server on the designated port (default: `1094`)[cite: 1].

---

## 4. Configuration Schema (`gondola.yaml`)

```yaml
config:
  site-name: ""          # Global site title
  version: ""            # Site/project version
  description: ""        # SEO default description
  canonical-url: ""     # Base URL for canonical links and sitemaps
  sass: true             # Enable/disable Sass compilation pipeline
  fenced-blocks: false   # Enable 'squero' custom .squero alias processing
  markdown: true         # Enable/disable Goldmark parsing
  engine: jet            # Template engine (default: Jet)
  sitemap: false         # Enable automated sitemap generation
  date-format: 1234567   # Native Go date formatting convention
  serve: false           # Enable local web server
  port: 1094             # Port used by 'sandolo' server

input:
  content: .             # Root folder for .md source files
  templates: _components/includes # Jet layout and component templates
  data: _data            # Global data directory (YAML/JSON)
  partials: _components/sass # SCSS/Sass source directory

output:
  routing: file          # URL routing strategy ('file' vs 'directory')
  directory: _site       # Output target directory
  passthrough: []        # Raw files/folders to copy as-is

default:                 # Fallback values passed to Jet engine
  layout: nil
  title: ""              # Prevents Jet engine from failing on missing keys
  author: ""
  date: ""
  permalink: path        # 'path', 'slug', 'date', or custom URL path
  sitemap: no            # Default sitemap inclusion status
  robots-agent: *        # Let the user decide which agent to follow to build the sitemap
```
