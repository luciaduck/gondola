// This is a stub. Not ready for production. Can change.

const (
    configFile       = "gondola.yaml"
    defaultSass      = true
    defaultSquero    = false
    defaultMd        = true
    defaultEngine    = "jet"
    defaultSitemap   = false
    defaultFormat    = 1234567
    defaultServe     = false
    defaultPort      = 1094
    defaultContent   = "."
    defaultTemplates = "_components/includes"
    defaultJsonYaml  = "_data"
    defaultScss      = "_components/sass"
    defaultRouting   = "file"
    defaultDir       = "_site"
    defaultLayout    = "nil"
    defaultPermalink = "path"
    defaultRobots    = "*"
    anyOtherDef      = ""
    contentVar       = "copy" // "copy" is used instead of "content" (reserved in Jet).
    // NOTE: Templates must use {{ copy|raw }} since Jet escapes HTML by default.
    // Do NOT use |raw on plain text variables like "title".
)

type Config struct {
    Config struct {
        SiteName     string `yaml:"site-name"`
        Version      string `yaml:"version"`
        Description  string `yaml:"description"`
        CanonicalURL string `yaml:"canonical-url"`
        Sass         bool   `yaml:"sass"`
        Squero       bool   `yaml:"fenced-blocks"`
        Markdown     bool   `yaml:"markdown"`
        Engine       string `yaml:"engine"`
        Sitemap      bool   `yaml:"sitemap"`
        DateFormat   int    `yaml:"date-format"`
        Serve        bool   `yaml:"serve"`
        Port         int    `yaml:"port"`
    } `yaml:"config"`

    Input struct {
        Content   string `yaml:"content"`
        Templates string `yaml:"templates"`
        Data      string `yaml:"data"`
        Partials  string `yaml:"partials"`
    } `yaml:"input"`

    Output struct {
        Routing     string   `yaml:"routing"`
        Directory   string   `yaml:"directory"`
        Passthrough []string `yaml:"passthrough"`
    } `yaml:"output"`

    Default struct {
        Layout      string `yaml:"layout"`
        Title       string `yaml:"title"`
        Author      string `yaml:"author"`
        Date        string `yaml:"date"`
        Permalink   string `yaml:"permalink"`
        RobotsAgent string `yaml:"robots-agent"`
    } `yaml:"default"`
}

func LoadConfig() (Config, error) {
    // Step 1: Pre-populate the struct with your constant defaults.
    // If a user doesn't specify it in YAML, these values stay intact.
    cfg := Config{}

    // Mapping constants to the config defaults:
    cfg.Config.Sass = defaultSass
    cfg.Config.Squero = defaultSquero
    cfg.Config.Markdown = defaultMd
    cfg.Config.Engine = defaultEngine
    cfg.Config.Sitemap = defaultSitemap
    cfg.Config.DateFormat = defaultFormat
    cfg.Config.Serve = defaultServe
    cfg.Config.Port = defaultPort

    cfg.Input.Content = defaultContent
    cfg.Input.Templates = defaultTemplates
    cfg.Input.Data = defaultJsonYaml
    cfg.Input.Partials = defaultScss

    cfg.Output.Routing = defaultRouting
    cfg.Output.Directory = defaultDir

    cfg.Default.Layout = defaultLayout
    cfg.Default.Permalink = defaultPermalink
    cfg.Default.RobotsAgent = defaultRobots

    // Step 2: Read the YAML file from disk
    fileData, err := os.ReadFile(configFile)
    if err != nil {
        // If gondola.yaml doesn't exist, it's not a fatal error; 
        // the program can just run entirely on your default constants!
        return cfg, nil
    }

    // Step 3: Parse the YAML over the defaults.
    // This overwrites only the fields the user actually wrote in their YAML.
    err = yaml.Unmarshal(fileData, &cfg)
    if err != nil {
        return cfg, err
    }

    return cfg, nil
}
