package server

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"image/jpeg"
)

// pageFiles holds the template of the waiting pages and their photos. The
// photos are under CC BY-SA licences; pages/credit.txt names their authors.
//
//go:embed pages/waiting.html pages/*.jpg
var pageFiles embed.FS

// waitingTemplatePath is the template that both waiting pages share.
const waitingTemplatePath = "pages/waiting.html"

// pageTitlePrefix starts the title of both pages; Gondola's version follows.
const pageTitlePrefix = "Gondola SSG "

// Seconds before a waiting page reloads itself. A build is usually over in
// a second; after a failure nothing changes until the user types g, so the
// page asks less often.
const (
	buildingRefreshSeconds = 1
	failedRefreshSeconds   = 2
)

// photo is a picture from Wikimedia Commons with what its licence asks to
// show: the title, the author, the licence, and links to both.
type photo struct {
	// FilePath is the embedded file, such as pages/Barena_bassa_Saccagnana.jpg.
	FilePath    string
	Alt         string
	Title       string
	Author      string
	CommonsURL  string
	LicenseName string
	LicenseURL  string
}

// The photos of the two pages (SPEC §5.16, point 10).
var (
	buildingPhoto = photo{
		FilePath:    "pages/Venice_Gondola_Grand_Canal.jpg",
		Alt:         "A gondolier in a striped shirt and straw hat rows a black gondola with red seats down the Grand Canal in Venice, between old palaces under a blue sky.",
		Title:       "Venice Gondola Grand Canal",
		Author:      "Saffron Blaze",
		CommonsURL:  "https://commons.wikimedia.org/wiki/File:Venice_Gondola_Grand_Canal.jpg",
		LicenseName: "CC BY-SA 3.0",
		LicenseURL:  "https://creativecommons.org/licenses/by-sa/3.0/",
	}
	failedPhoto = photo{
		FilePath:    "pages/Barena_bassa_Saccagnana.jpg",
		Alt:         "Low salt marshes in the Venetian lagoon in the late afternoon: tufts of grass rise from still, shallow water that reflects the sun.",
		Title:       "Barena bassa Saccagnana",
		Author:      "AlbertoMara",
		CommonsURL:  "https://commons.wikimedia.org/wiki/File:Barena_bassa_Saccagnana.jpg",
		LicenseName: "CC BY-SA 4.0",
		LicenseURL:  "https://creativecommons.org/licenses/by-sa/4.0/",
	}
)

// waitingPage is what the template of a waiting page needs.
type waitingPage struct {
	// Kind is the class of the body, which sets the color of the heading.
	Kind           string
	Title          string
	RefreshSeconds int
	Heading        string
	// Lead is an optional line in bold under the heading.
	Lead  string
	Text  string
	Photo inlinedPhoto
}

// inlinedPhoto is a photo ready for the page: its file is a data: URI, so
// the page needs no URL of its own that could clash with the user's site.
type inlinedPhoto struct {
	photo
	// SourceAttribute is the whole src attribute with a data: URI. It has
	// the type template.HTMLAttr because, as a template.URL, html/template
	// would write every "+" of the base64 text as "&#43;"; base64 holds no
	// quote, so the attribute is safe as it is.
	SourceAttribute template.HTMLAttr
	Width           int
	Height          int
}

// waitingPages are the two pages rendered, once per serve.
type waitingPages struct {
	building []byte
	failed   []byte
}

// renderWaitingPages renders both pages for Gondola's version.
func renderWaitingPages(gondolaVersion string) (waitingPages, error) {
	pageTemplate, err := template.ParseFS(pageFiles, waitingTemplatePath)
	if err != nil {
		return waitingPages{}, fmt.Errorf("reading the waiting page: %w", err)
	}
	title := pageTitlePrefix + gondolaVersion
	building, err := renderWaitingPage(pageTemplate, waitingPage{
		Kind:           "building",
		Title:          title,
		RefreshSeconds: buildingRefreshSeconds,
		Heading:        "Your site is being shipped with Gondola.",
		Text:           "Enjoy the view while the build runs.",
	}, buildingPhoto)
	if err != nil {
		return waitingPages{}, err
	}
	failed, err := renderWaitingPage(pageTemplate, waitingPage{
		Kind:           "failed",
		Title:          title,
		RefreshSeconds: failedRefreshSeconds,
		Heading:        "Build failed",
		Lead:           "Aground on the barene.",
		Text:           "The gondola ran onto the salt marshes of the lagoon: the last build failed. The errors are in the terminal; fix them, then type g and Enter.",
	}, failedPhoto)
	if err != nil {
		return waitingPages{}, err
	}
	return waitingPages{building: building, failed: failed}, nil
}

// renderWaitingPage renders one page with its photo inlined.
func renderWaitingPage(pageTemplate *template.Template, page waitingPage, source photo) ([]byte, error) {
	inlined, err := inlinePhoto(source)
	if err != nil {
		return nil, err
	}
	page.Photo = inlined
	var rendered bytes.Buffer
	if err := pageTemplate.Execute(&rendered, page); err != nil {
		return nil, fmt.Errorf("rendering the %s page: %w", page.Kind, err)
	}
	return rendered.Bytes(), nil
}

// inlinePhoto reads the photo's file and measures it, so that the browser
// keeps its space before it is decoded.
func inlinePhoto(source photo) (inlinedPhoto, error) {
	content, err := pageFiles.ReadFile(source.FilePath)
	if err != nil {
		return inlinedPhoto{}, fmt.Errorf("reading %s: %w", source.FilePath, err)
	}
	size, err := jpeg.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return inlinedPhoto{}, fmt.Errorf("reading the size of %s: %w", source.FilePath, err)
	}
	return inlinedPhoto{
		photo:           source,
		SourceAttribute: template.HTMLAttr(`src="data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(content) + `"`),
		Width:           size.Width,
		Height:          size.Height,
	}, nil
}
