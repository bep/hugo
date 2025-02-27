package tplimplv2_test

import (
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/tpl/tplimplv2"
)

// Old as in before Hugo v0.146.0.
func TestLayoutsOldSetup(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
defaultContentLanguage = "en"
defaultContentLanguageInSubdir = true
[languages]
[languages.en]
title = "Title in English"
weight = 1
[languages.nn]
title = "Tittel på nynorsk"
weight = 2
-- layouts/index.html --
Home.
{{ template "_internal/twitter_cards.html" . }}
-- layouts/_default/single.html --
Single.
-- layouts/_default/single.nn.html --
Single NN.
-- layouts/_default/list.html --
List HTML.
-- layouts/_default/list.json --
List JSON.
-- layouts/_default/list.rss.xml --
List RSS.
-- layouts/_default/list.nn.rss.xml --
List NN RSS.
-- layouts/_default/baseof.html --
Base.
-- layouts/partials/mypartial.html --
Partial.
-- layouts/shortcodes/myshortcode.html --
Shortcode.
-- content/docs/p1.md --
---
title: "P1"
---

	`

	hugolib.Test(t, files)
}

func TestLayoutsOldSetupCustomRSS(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ["taxonomy", "term", "page"]
[outputs]
home = ["rss"]
-- layouts/_default/list.rss.xml --
List RSS.
`
	b := hugolib.Test(t, files)
	b.AssertFileContent("public/index.xml", "List RSS.")
}

// TODO1 test defer in "main"
var newSetupTestSites = `
-- hugo.toml --
defaultContentLanguage = "en"
defaultContentLanguageInSubdir = true
[languages]
[languages.en]
title = "Title in English"
weight = 1
[languages.nn]
title = "Tittel på nynorsk"
weight = 2
[languages.fr]
title = "Titre en français"
weight = 3

[outputs]
home     = ["html", "rss", "redir"]

[outputFormats]
[outputFormats.redir]
mediatype   = "text/plain"
baseName    = "_redirects"
isPlainText = true
-- layouts/pages/404.html --
{{ define "main" }}
404.
{{ end }}
-- layouts/pages/home.html --
{{ define "main" }}
Home: {{ .Title }}|{{ .Content }}|
Inline Partial: {{ partial "my-inline-partial.html" . }}
{{ end }}
{{ define "hero" }}
Home hero.
{{ end }}
{{ define "partials/my-inline-partial.html" }}
{{ $value := 32 }}
{{ return $value }}
{{ end }}
-- layouts/pages/index.redir --
# TODO1 rename the template.
Redir.
-- layouts/pages/single.html --
{{ define "main" }}
Single needs base.
{{ end }}
-- layouts/pages/foo/bar/single.html --
{{ define "main" }}
Single sub path.
{{ end }}
-- layouts/pages/_markup/render-codeblock.html --
Render codeblock.
-- layouts/pages/_markup/render-blockquote.html --
Render blockquote.
-- layouts/pages/_markup/render-codeblock-go.html --
 Render codeblock go.
-- layouts/pages/_markup/render-link.html --
Link: {{ .Destination | safeURL }}
-- layouts/pages/foo/baseof.html --
Base sub path.{{ block "main" . }}{{ end }}
-- layouts/pages/foo/bar/baseof.page.html --
Base sub path.{{ block "main" . }}{{ end }}
-- layouts/pages/list.html --
{{ define "main" }}
List needs base.
{{ end }}
-- layouts/pages/section.html --
Section.
-- layouts/pages/mysectionlayout.section.fr.amp.html --
Section with layout.
-- layouts/pages/baseof.html --
Base.{{ block "main" . }}{{ end }}
Hero:{{ block "hero" . }}{{ end }}:
{{ with (templates.Defer (dict "key" "global")) }}
Defer Block.
{{ end }}
-- layouts/pages/baseof.fr.html --
Base fr.{{ block "main" . }}{{ end }}
-- layouts/pages/baseof.term.html --
Base term.
-- layouts/pages/baseof.section.fr.amp.html --
Base with identifiers.{{ block "main" . }}{{ end }}
-- layouts/partials/mypartial.html --
Partial. {{ partial "_inline/my-inline-partial-in-partial-with-no-ext" . }}
{{ define "partials/_inline/my-inline-partial-in-partial-with-no-ext" }}
Partial in partial.
{{ end }}
-- layouts/partials/returnfoo.html --
{{ $v := "foo" }}
{{ return $v }}
-- layouts/shortcodes/myshortcode.html --
Shortcode. {{ partial "mypartial.html" . }}|return:{{ partial "returnfoo.html" . }}|
-- content/_index.md --
---
title: Home sweet home!
---

{{< myshortcode >}}

> My blockquote.


Markdown link: [Foo](/foo)
-- content/p1.md --
---
title: "P1"
---
-- content/foo/bar/index.md --
---
title: "Foo Bar"
---

{{< myshortcode >}}

-- content/single-list.md --
---
title: "Single List"
layout: "list"
---

`

func TestLayoutsType(t *testing.T) {
	files := `
-- hugo.toml --
disableKinds = ["taxonomy", "term"]
-- layouts/pages/list.html --
List.
-- layouts/pages/mysection/single.html --
mysection/single|{{ .Title }}
-- layouts/pages/mytype/single.html --
mytype/single|{{ .Title }}
-- content/mysection/_index.md --
-- content/mysection/mysubsection/_index.md --
-- content/mysection/mysubsection/p1.md --
---
title: "P1"
---
-- content/mysection/mysubsection/p2.md --
---
title: "P2"
type: "mytype"
---

`

	b := hugolib.Test(t, files, hugolib.TestOptWarn())

	b.AssertLogContains("! WARN")

	b.AssertFileContent("public/mysection/mysubsection/p1/index.html", "mysection/single|P1")
	b.AssertFileContent("public/mysection/mysubsection/p2/index.html", "mytype/single|P2")
}

// New, as in from Hugo v0.146.0.
func TestLayoutsNewSetup(t *testing.T) {
	const numIterations = 1
	for range numIterations {

		b := hugolib.Test(t, newSetupTestSites, hugolib.TestOptWarn())

		b.AssertLogContains("! WARN")

		b.AssertFileContent("public/en/index.html",
			"Base.\nHome: Home sweet home!|",
			"|Shortcode.\n|",
			"<p>Markdown link: Link: /foo</p>",
			"|return:foo|",
			"Defer Block.",
			"Home hero.",
			"Render blockquote.",
		)

		b.AssertFileContent("public/en/p1/index.html", "Base.\nSingle needs base.\n\nHero::\n\nDefer Block.")
		b.AssertFileContent("public/en/404.html", "404.")
		b.AssertFileContent("public/nn/404.html", "404.")
		b.AssertFileContent("public/fr/404.html", "404.")

		// TODO1 enable below.
		if true {
			return
		}

		store := b.H.TemplateStore

		// Test shortcode lookup.
		// Note to self: For shortcodes we only use a subset of the descriptor fields.
		v := store.LookupShortcode("myshortcode", tplimplv2.TemplateDescriptor{Layout: "", Lang: "en", OutputFormat: "html"})
		b.Assert(v, qt.IsNotNil)
		b.Assert(v.Template.Name(), qt.Equals, "shortcodes/myshortcode.html")

		v = store.LookupPartial("mypartial", tplimplv2.TemplateDescriptor{Layout: "", Lang: "en", OutputFormat: "html"})
		b.Assert(v, qt.IsNotNil)
		b.Assert(v.Template.Name(), qt.Equals, "partials/mypartial.html")

		// Test page layout lookup.
		testOne := func(s string, category tplimplv2.Category, input, expected tplimplv2.TemplateDescriptor) {
			v := store.LookupPagesLayout(doctree.LockTypeRead, s, category, input)
			b.Assert(v, qt.IsNotNil)
			b.Assert(v.D, qt.DeepEquals, expected)
		}

		testOne("/", tplimplv2.CategoryBaseof,
			tplimplv2.TemplateDescriptor{Kind: "home", Layout: "baseof", Lang: "en", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "", Layout: "baseof", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)
		testOne("/", tplimplv2.CategoryBaseof,
			tplimplv2.TemplateDescriptor{Kind: "term", Layout: "baseof", Lang: "en", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "term", Layout: "baseof", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

		testOne("/", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "home", Layout: "", Lang: "en", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "home", Layout: "", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

		testOne("/sect1", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "", Lang: "en", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

		testOne("/foo/bar/mypage", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "page", Layout: "", Lang: "fr", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "", Layout: "single", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

		testOne("/sect1", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "", Lang: "fr", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

		// -- layouts/pages/mysectionlayout.section.fr.amp.html --
		testOne("/sect1", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "mysectionlayout", Lang: "fr", OutputFormat: "amp", MediaType: "text/html"},
			tplimplv2.TemplateDescriptor{Kind: "section", Layout: "mysectionlayout", Lang: "fr", OutputFormat: "amp", MediaType: "text/html"},
		)

		testOne("/categories/blue", tplimplv2.CategoryLayout,
			tplimplv2.TemplateDescriptor{Kind: "taxonomy", Layout: "", Lang: "fr", OutputFormat: "html"},
			tplimplv2.TemplateDescriptor{Kind: "", Layout: "list", Lang: "", OutputFormat: "html", MediaType: "text/html"},
		)

	}
}

func TestHomeRSSAndHTMLWithHTMLOnlyShortcode(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ["taxonomy", "term"]
[outputs]
home = ["html", "rss"]
-- layouts/pages/home.html --
Home: {{ .Title }}|{{ .Content }}|
-- layouts/pages/single.html --
Single: {{ .Title }}|{{ .Content }}|
-- layouts/shortcodes/myshortcode.html --
Myshortcode: Count: {{ math.Counter }}|
-- content/p1.md --
---
title: "P1"
---

{{< myshortcode >}}
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/p1/index.html", "Single: P1|Myshortcode: Count: 1|")
	b.AssertFileContent("public/index.xml", "Myshortcode: Count: 1")
}

func TestHomeRSSAndHTMLWithHTMLOnlyRenderHook(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ["taxonomy", "term"]
[outputs]
home = ["html", "rss"]
-- layouts/pages/home.html --
Home: {{ .Title }}|{{ .Content }}|
-- layouts/pages/single.html --
Single: {{ .Title }}|{{ .Content }}|
-- layouts/pages/_markup/render-link.html --
Render Link: {{ math.Counter }}|
-- content/p1.md --
---
title: "P1"
---

Link: [Foo](/foo)
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/p1/index.html", "Single: P1|<p>Link: Render Link: 1|<")
	b.AssertFileContent("public/index.xml", "Link: Render Link: 1|")
}

func TestRenderCodebblockSpecificity(t *testing.T) {
	files := `
-- hugo.toml --
-- layouts/pages/_markup/render-codeblock.html --
Render codeblock.|{{ .Inner }}|
-- layouts/pages/_markup/render-codeblock-go.html --
Render codeblock go.|{{ .Inner }}|
-- layouts/pages/single.html --
{{ .Title }}|{{ .Content }}|
-- content/p1.md --
---
title: "P1"
---

§§§
Basic
§§§

§§§ go
Go
§§§

`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/p1/index.html", "P1|Render codeblock.|Basic|Render codeblock go.|Go|")
}

func BenchmarkTv2NewSetup(b *testing.B) {
	bb := hugolib.Test(b, newSetupTestSites)
	store := bb.H.TemplateStore

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// store.LookupPageLayout("/sect1", tplimplv2.PageLayoutDescriptor{Kind: "section", Layout: "", Lang: "fr", OutputFormat: "html"})
		// store.LookupPageLayout("/", tplimplv2.PageLayoutDescriptor{Kind: "home", Layout: "baseof", Lang: "en", OutputFormat: "html"})
		store.LookupPagesLayout(doctree.LockTypeRead, "/categories/blue", tplimplv2.CategoryLayout, tplimplv2.TemplateDescriptor{Kind: "taxonomy", Layout: "", Lang: "fr", OutputFormat: "html"})
	}
}

func TestPrintUnusedTemplates(t *testing.T) {
	t.Parallel()

	files := `
-- config.toml --
baseURL = 'http://example.com/'
printUnusedTemplates=true
-- content/p1.md --
---
title: "P1"
---
{{< usedshortcode >}}
-- layouts/baseof.html --
{{ block "main" . }}{{ end }}
-- layouts/baseof.json --
{{ block "main" . }}{{ end }}
-- layouts/index.html --
{{ define "main" }}FOO{{ end }}
-- layouts/_default/single.json --
-- layouts/_default/single.html --
{{ define "main" }}MAIN{{ end }}
-- layouts/post/single.html --
{{ define "main" }}MAIN{{ end }}
-- layouts/partials/usedpartial.html --
-- layouts/partials/unusedpartial.html --
-- layouts/shortcodes/usedshortcode.html --
{{ partial "usedpartial.html" }}
-- layouts/shortcodes/unusedshortcode.html --

	`

	b := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
			NeedsOsFS:   true,
		},
	)
	b.Build()

	unused := b.H.Tmpl().Unused()
	var names []string
	for _, tmpl := range unused {
		if fi := tmpl.Fi; fi != nil {
			names = append(names, fi.Meta().PathInfo.PathNoLeadingSlash())
		}
	}

	b.Assert(len(unused), qt.Equals, 5, qt.Commentf("%#v", names))
	b.Assert(names, qt.DeepEquals, []string{"baseof.json", "_default/single.json", "partials/unusedpartial.html", "shortcodes/unusedshortcode.html"})
}

// Verify that the new keywords in Go 1.18 is available.
func TestGo18Constructs(t *testing.T) {
	t.Parallel()

	files := `
-- config.toml --
baseURL = 'http://example.com/'
disableKinds = ["section", "home", "rss", "taxonomy",  "term", "rss"]
-- content/p1.md --
---
title: "P1"
---
-- layouts/partials/counter.html --
{{ if .Scratch.Get "counter" }}{{ .Scratch.Add "counter" 1 }}{{ else }}{{ .Scratch.Set "counter" 1 }}{{ end }}{{ return true }}
-- layouts/_default/single.html --
continue:{{ range seq 5 }}{{ if eq . 2 }}{{continue}}{{ end }}{{ . }}{{ end }}:END:
break:{{ range seq 5 }}{{ if eq . 2 }}{{break}}{{ end }}{{ . }}{{ end }}:END:
continue2:{{ range seq 5 }}{{ if eq . 2 }}{{ continue }}{{ end }}{{ . }}{{ end }}:END:
break2:{{ range seq 5 }}{{ if eq . 2 }}{{ break }}{{ end }}{{ . }}{{ end }}:END:

counter1: {{ partial "counter.html" . }}/{{ .Scratch.Get "counter" }}
and1: {{ if (and false (partial "counter.html" .)) }}true{{ else }}false{{ end }}
or1: {{ if (or true (partial "counter.html" .)) }}true{{ else }}false{{ end }}
and2: {{ if (and true (partial "counter.html" .)) }}true{{ else }}false{{ end }}
or2: {{ if (or false (partial "counter.html" .)) }}true{{ else }}false{{ end }}


counter2: {{ .Scratch.Get "counter" }}


	`

	b := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
			NeedsOsFS:   true,
		},
	)
	b.Build()

	b.AssertFileContent("public/p1/index.html", `
continue:1345:END:
break:1:END:
continue2:1345:END:
break2:1:END:
counter1: true/1
and1: false
or1: true
and2: true
or2: true
counter2: 3
`)
}

func TestGo23ElseWith(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
title = "Hugo"
-- layouts/index.html --
{{ with false }}{{ else with .Site }}{{ .Title }}{{ end }}|
`
	b := hugolib.Test(t, files)

	b.AssertFileContent("public/index.html", "Hugo|")
}

// Issue 10495
func TestCommentsBeforeBlockDefinition(t *testing.T) {
	t.Parallel()

	files := `
-- config.toml --
baseURL = 'http://example.com/'
-- content/s1/p1.md --
---
title: "S1P1"
---
-- content/s2/p1.md --
---
title: "S2P1"
---
-- content/s3/p1.md --
---
title: "S3P1"
---
-- layouts/_default/baseof.html --
{{ block "main" . }}{{ end }}
-- layouts/s1/single.html --
{{/* foo */}}
{{ define "main" }}{{ .Title }}{{ end }}
-- layouts/s2/single.html --
{{- /* foo */}}
{{ define "main" }}{{ .Title }}{{ end }}
-- layouts/s3/single.html --
{{- /* foo */ -}}
{{ define "main" }}{{ .Title }}{{ end }}
	`

	b := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
		},
	)
	b.Build()

	b.AssertFileContent("public/s1/p1/index.html", `S1P1`)
	b.AssertFileContent("public/s2/p1/index.html", `S2P1`)
	b.AssertFileContent("public/s3/p1/index.html", `S3P1`)
}

func TestGoTemplateBugs(t *testing.T) {
	t.Run("Issue 11112", func(t *testing.T) {
		t.Parallel()

		files := `
-- config.toml --
-- layouts/index.html --
{{ $m := dict "key" "value" }}
{{ $k := "" }}
{{ $v := "" }}
{{ range $k, $v = $m }}
{{ $k }} = {{ $v }}
{{ end }}
	`

		b := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:           t,
				TxtarString: files,
			},
		)
		b.Build()

		b.AssertFileContent("public/index.html", `key = value`)
	})
}

func TestSecurityAllowActionJSTmpl(t *testing.T) {
	filesTemplate := `
-- config.toml --
SECURITYCONFIG
-- layouts/index.html --
<script>
var a = §§{{.Title }}§§;
</script>
	`

	files := strings.ReplaceAll(filesTemplate, "SECURITYCONFIG", "")

	b, err := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
		},
	).BuildE()

	// This used to fail, but not in >= Hugo 0.121.0.
	b.Assert(err, qt.IsNil)
}

func TestGoogleAnalyticsTemplate(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ['page','section','rss','sitemap','taxonomy','term']
[privacy.googleAnalytics]
disable = false
respectDoNotTrack = true
[services.googleAnalytics]
id = 'G-0123456789'
-- layouts/index.html --
{{ template "_internal/google_analytics.html" . }}
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/index.html",
		`<script async src="https://www.googletagmanager.com/gtag/js?id=G-0123456789"></script>`,
		`var dnt = (navigator.doNotTrack || window.doNotTrack || navigator.msDoNotTrack);`,
	)
}

func TestDisqusTemplate(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ['page','section','rss','sitemap','taxonomy','term']
[services.disqus]
shortname = 'foo'
[privacy.disqus]
disable = false
-- layouts/index.html --
{{ template "_internal/disqus.html" . }}
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/index.html",
		`s.src = '//' + "foo" + '.disqus.com/embed.js';`,
	)
}

func TestSitemap(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
disableKinds = ['home','rss','section','taxonomy','term']
[sitemap]
disable = true
-- content/p1.md --
---
title: p1
sitemap:
  p1_disable: foo
---
-- content/p2.md --
---
title: p2

---
-- layouts/_default/single.html --
{{ .Title }}
`

	// Test A: Exclude all pages via site config.
	b := hugolib.Test(t, files)
	b.AssertFileContentExact("public/sitemap.xml",
		"<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"\n  xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n  \n</urlset>\n",
	)

	// Test B: Include all pages via site config.
	files_b := strings.ReplaceAll(files, "disable = true", "disable = false")
	b = hugolib.Test(t, files_b)
	b.AssertFileContentExact("public/sitemap.xml",
		"<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"\n  xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n  <url>\n    <loc>/p1/</loc>\n  </url><url>\n    <loc>/p2/</loc>\n  </url>\n</urlset>\n",
	)

	// Test C: Exclude all pages via site config, but include p1 via front matter.
	files_c := strings.ReplaceAll(files, "p1_disable: foo", "disable: false")
	b = hugolib.Test(t, files_c)
	b.AssertFileContentExact("public/sitemap.xml",
		"<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"\n  xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n  <url>\n    <loc>/p1/</loc>\n  </url>\n</urlset>\n",
	)

	// Test D:  Include all pages via site config, but exclude p1 via front matter.
	files_d := strings.ReplaceAll(files_b, "p1_disable: foo", "disable: true")
	b = hugolib.Test(t, files_d)
	b.AssertFileContentExact("public/sitemap.xml",
		"<?xml version=\"1.0\" encoding=\"utf-8\" standalone=\"yes\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\"\n  xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n  <url>\n    <loc>/p2/</loc>\n  </url>\n</urlset>\n",
	)
}

// Issue 12418
func TestOpengraph(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
capitalizeListTitles = false
disableKinds = ['rss','sitemap']
languageCode = 'en-US'
[markup.goldmark.renderer]
unsafe = true
[params]
description = "m <em>n</em> and **o** can't."
[params.social]
facebook_admin = 'foo'
[taxonomies]
series = 'series'
tag = 'tags'
-- layouts/_default/list.html --
{{ template "_internal/opengraph.html" . }}
-- layouts/_default/single.html --
{{ template "_internal/opengraph.html" . }}
-- content/s1/p1.md --
---
title: p1
date: 2024-04-24T08:00:00-07:00
lastmod: 2024-04-24T11:00:00-07:00
images: [a.jpg,b.jpg]
audio: [c.mp3,d.mp3]
videos: [e.mp4,f.mp4]
series: [series-1]
tags: [t1,t2]
---
a <em>b</em> and **c** can't.
-- content/s1/p2.md --
---
title: p2
series: [series-1]
---
d <em>e</em> and **f** can't.
<!--more-->
-- content/s1/p3.md --
---
title: p3
series: [series-1]
summary: g <em>h</em> and **i** can't.
---
-- content/s1/p4.md --
---
title: p4
series: [series-1]
description: j <em>k</em> and **l** can't.
---
-- content/s1/p5.md --
---
title: p5
series: [series-1]
---
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/s1/p1/index.html", `
		<meta property="og:url" content="/s1/p1/">
		<meta property="og:title" content="p1">
		<meta property="og:description" content="a b and c can’t.">
		<meta property="og:locale" content="en_US">
		<meta property="og:type" content="article">
		<meta property="article:section" content="s1">
		<meta property="article:published_time" content="2024-04-24T08:00:00-07:00">
		<meta property="article:modified_time" content="2024-04-24T11:00:00-07:00">
		<meta property="article:tag" content="t1">
		<meta property="article:tag" content="t2">
		<meta property="og:image" content="/a.jpg">
		<meta property="og:image" content="/b.jpg">
		<meta property="og:audio" content="/c.mp3">
		<meta property="og:audio" content="/d.mp3">
		<meta property="og:video" content="/e.mp4">
		<meta property="og:video" content="/f.mp4">
		<meta property="og:see_also" content="/s1/p2/">
		<meta property="og:see_also" content="/s1/p3/">
		<meta property="og:see_also" content="/s1/p4/">
		<meta property="og:see_also" content="/s1/p5/">
		<meta property="fb:admins" content="foo">
		`,
	)

	b.AssertFileContent("public/s1/p2/index.html",
		`<meta property="og:description" content="d e and f can’t.">`,
	)

	b.AssertFileContent("public/s1/p3/index.html",
		`<meta property="og:description" content="g h and i can’t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p4/index.html",
		`<meta property="og:description" content="j k and **l** can&#39;t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p5/index.html",
		`<meta property="og:description" content="m n and **o** can&#39;t.">`,
	)
}

// Issue 12432
func TestSchema(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
capitalizeListTitles = false
disableKinds = ['rss','sitemap']
[markup.goldmark.renderer]
unsafe = true
[params]
description = "m <em>n</em> and **o** can't."
[taxonomies]
tag = 'tags'
-- layouts/_default/list.html --
{{ template "_internal/schema.html" . }}
-- layouts/_default/single.html --
{{ template "_internal/schema.html" . }}
-- content/s1/p1.md --
---
title: p1
date: 2024-04-24T08:00:00-07:00
lastmod: 2024-04-24T11:00:00-07:00
images: [a.jpg,b.jpg]
tags: [t1,t2]
---
a <em>b</em> and **c** can't.
-- content/s1/p2.md --
---
title: p2
---
d <em>e</em> and **f** can't.
<!--more-->
-- content/s1/p3.md --
---
title: p3
summary: g <em>h</em> and **i** can't.
---
-- content/s1/p4.md --
---
title: p4
description: j <em>k</em> and **l** can't.
---
-- content/s1/p5.md --
---
title: p5
---
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/s1/p1/index.html", `
		<meta itemprop="name" content="p1">
		<meta itemprop="description" content="a b and c can’t.">
		<meta itemprop="datePublished" content="2024-04-24T08:00:00-07:00">
		<meta itemprop="dateModified" content="2024-04-24T11:00:00-07:00">
		<meta itemprop="wordCount" content="5">
		<meta itemprop="image" content="/a.jpg">
		<meta itemprop="image" content="/b.jpg">
		<meta itemprop="keywords" content="t1,t2">
  		`,
	)

	b.AssertFileContent("public/s1/p2/index.html",
		`<meta itemprop="description" content="d e and f can’t.">`,
	)

	b.AssertFileContent("public/s1/p3/index.html",
		`<meta itemprop="description" content="g h and i can’t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p4/index.html",
		`<meta itemprop="description" content="j k and **l** can&#39;t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p5/index.html",
		`<meta itemprop="description" content="m n and **o** can&#39;t.">`,
	)
}

// Issue 12433
func TestTwitterCards(t *testing.T) {
	t.Parallel()

	files := `
-- hugo.toml --
capitalizeListTitles = false
disableKinds = ['rss','sitemap','taxonomy','term']
[markup.goldmark.renderer]
unsafe = true
[params]
description = "m <em>n</em> and **o** can't."
[params.social]
twitter = 'foo'
-- layouts/_default/list.html --
{{ template "_internal/twitter_cards.html" . }}
-- layouts/_default/single.html --
{{ template "_internal/twitter_cards.html" . }}
-- content/s1/p1.md --
---
title: p1
images: [a.jpg,b.jpg]
---
a <em>b</em> and **c** can't.
-- content/s1/p2.md --
---
title: p2
---
d <em>e</em> and **f** can't.
<!--more-->
-- content/s1/p3.md --
---
title: p3
summary: g <em>h</em> and **i** can't.
---
-- content/s1/p4.md --
---
title: p4
description: j <em>k</em> and **l** can't.
---
-- content/s1/p5.md --
---
title: p5
---
`

	b := hugolib.Test(t, files)

	b.AssertFileContent("public/s1/p1/index.html", `
		<meta name="twitter:card" content="summary_large_image">
		<meta name="twitter:image" content="/a.jpg">
		<meta name="twitter:title" content="p1">
		<meta name="twitter:description" content="a b and c can’t.">
		<meta name="twitter:site" content="@foo">
		`,
	)

	b.AssertFileContent("public/s1/p2/index.html",
		`<meta name="twitter:card" content="summary">`,
		`<meta name="twitter:description" content="d e and f can’t.">`,
	)

	b.AssertFileContent("public/s1/p3/index.html",
		`<meta name="twitter:description" content="g h and i can’t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p4/index.html",
		`<meta name="twitter:description" content="j k and **l** can&#39;t.">`,
	)

	// The markdown is intentionally not rendered to HTML.
	b.AssertFileContent("public/s1/p5/index.html",
		`<meta name="twitter:description" content="m n and **o** can&#39;t.">`,
	)
}

// Issue 12963
func TestEditBaseofParseAfterExecute(t *testing.T) {
	files := `
-- hugo.toml --
baseURL = "https://example.com"
disableLiveReload = true
disableKinds = ["taxonomy", "term", "rss", "404", "sitemap"]
[internal]
fastRenderMode = true
-- layouts/_default/baseof.html --
Baseof!
{{ block "main" . }}default{{ end }}
{{ with (templates.Defer (dict "key" "global")) }}
Now. {{ now }}
{{ end }}
-- layouts/_default/single.html --
{{ define "main" }}
Single.
{{ end }}
-- layouts/_default/list.html --
{{ define "main" }}
List.
{{ .Content }}
{{ range .Pages }}{{ .Title }}{{ end }}|
{{ end }}
-- content/mybundle1/index.md --
---
title: "My Bundle 1"
---
-- content/mybundle2/index.md --
---
title: "My Bundle 2"
---
-- content/_index.md --
---
title: "Home"
---
Home!
`

	b := hugolib.TestRunning(t, files)
	b.AssertFileContent("public/index.html", "Home!")
	b.EditFileReplaceAll("layouts/_default/baseof.html", "Baseof", "Baseof!").Build()
	b.BuildPartial("/")
	b.AssertFileContent("public/index.html", "Baseof!")
	b.BuildPartial("/mybundle1/")
	b.AssertFileContent("public/mybundle1/index.html", "Baseof!")
}
