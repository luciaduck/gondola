# gondola

[![CI](https://github.com/luciaduck/gondola/actions/workflows/ci.yml/badge.svg)](https://github.com/luciaduck/gondola/actions/workflows/ci.yml)

> Note: this readme was written by a human, so it has some quirks and a bit of humor. I promise the content is serious though: AI approved it!

Gondola is a *gliding* **static site generator** written in Go. 

It is inspired by 11ty and it is kind of a love letter to its *awesome* configuration approach. However it aims at Go's speed and at the smallest number of dependencies possible.

## Tell me more

Gondola's philosophy is letting the developer configure whatever they care to have configured manually while giving them solid fallbacks for anything else. 

The config file is roughly **30 lines** long.

Gondola's ideal user has either used 11ty, or Jekyll or no SSG at all. Most conventional fallbacks are taken from those.

At the time of this release, Gondola uses Jet templates only. Liquid templates are planned to be added in the future. 

## How to install

`go install github.com/luciaduck/gondola/cmd/gondola@latest`

It needs Go 1.27 or newer. If your site uses sass, Gondola will need Dart Sass 1.63.0 or newer installed on the machine. Get it [here](https://sass-lang.com/install), not from npm, because Gondola can't manage it otherwise.

Go bin's folder must be in PATH.

## How to use Gondola

Launch it in the folder that holds the gondola.yaml configuration file (i.e. your site's repo).

See **Configuration** below to learn how to configure Gondola.

**Three commands:**
- `gondola ride`: it builds the site in the output directory

- `gondola serve`: it builds the site and serves it at port 1094 (or any port you chose in config)

- `gondola --help`: lists the commands *(I know, boring: one could expect a gondola coming to rescue you)*.


## License and contributing

WIP (I promise this will come soon! `go build ./cmd/gondola`)

## Configuration

The configuration file can be as short as 4 lines if you like all the proposed fallbacks written below. However, I really do recommend to at least decide on a default.layout and any passthroughs you need.

Copy this block and save it as gondola.yaml at the repo's root.

```yaml
config:
  site-name: "My site"                  # required
  version: "0.1.0"                      # required; the version of the *site*
  canonical-url: "https://example.com"  # required; absolute http(s) URL, trailing slash removed
  description: ""                       # optional
  sass: true
  fenced-blocks: false                  # true = run Squero; requires markdown: true
  markdown: true                        # false = .md files are not pages
  plugin: []                            # paths relative to the project root
  engine: jet                           # the only supported value today
  sitemap: false
  robots-agent: "*"                     # user-agent whose robots.txt rules filter the sitemap
  date-format: "2006-01-02"             # a Go time layout
  port: 1094                            # used by `gondola serve`

input:
  content: .
  templates: _components/includes       # a path, or a list of paths
  data: _data                           # a path, or a list of paths
  partials: _components/sass            # Sass load paths; a path, or a list of paths

output:
  routing: file                         # file | directory
  directory: _site
  passthrough: []

default:                                # values used when a page leaves a key empty
  layout: nil                           # "nil" = no layout; I recommend configure a real default
  title: ""
  author: ""
  date: ""
  permalink: path                       # path | slug | date
  ```
  If you plan to use fenced blocks, you might want to add an alias.squero file at the root of the repo. I will explain how Squero works in more detail soon. For now, I can link to its own repository, [whose readme](https://github.com/luciaduck/squero) was written by a cold LLM, so it's not as cool as this one. Sorry.
 
 Enjoy Gondola!
