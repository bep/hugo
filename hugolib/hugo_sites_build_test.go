package hugolib

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/htesting"
	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/spf13/afero"
)

func TestMultiSitesMainLangInRoot(t *testing.T) {
	t.Parallel()
	for _, b := range []bool{false} {
		doTestMultiSitesMainLangInRoot(t, b)
	}
}

func doTestMultiSitesMainLangInRoot(t *testing.T, defaultInSubDir bool) {
	c := qt.New(t)

	siteConfig := map[string]any{
		"DefaultContentLanguage":         "fr",
		"DefaultContentLanguageInSubdir": defaultInSubDir,
	}

	b := newMultiSiteTestBuilder(t, "toml", multiSiteTOMLConfigTemplate, siteConfig)

	pathMod := func(s string) string {
		return s
	}

	if !defaultInSubDir {
		pathMod = func(s string) string {
			return strings.Replace(s, "/fr/", "/", -1)
		}
	}

	b.CreateSites()
	b.Build(BuildCfg{})

	sites := b.H.Sites
	c.Assert(len(sites), qt.Equals, 4)

	enSite := sites[0]
	frSite := sites[1]

	c.Assert(enSite.LanguagePrefix(), qt.Equals, "/en")

	if defaultInSubDir {
		c.Assert(frSite.LanguagePrefix(), qt.Equals, "/fr")
	} else {
		c.Assert(frSite.LanguagePrefix(), qt.Equals, "")
	}

	c.Assert(enSite.PathSpec.RelURL("foo", true), qt.Equals, "/blog/en/foo")

	doc1en := enSite.RegularPages()[0]
	doc1fr := frSite.RegularPages()[0]

	enPerm := doc1en.Permalink()
	enRelPerm := doc1en.RelPermalink()
	c.Assert(enPerm, qt.Equals, "http://example.com/blog/en/sect/doc1-slug/")
	c.Assert(enRelPerm, qt.Equals, "/blog/en/sect/doc1-slug/")

	frPerm := doc1fr.Permalink()
	frRelPerm := doc1fr.RelPermalink()

	b.AssertFileContent(pathMod("public/fr/sect/doc1/index.html"), "Single", "Bonjour")
	b.AssertFileContent("public/en/sect/doc1-slug/index.html", "Single", "Hello")

	if defaultInSubDir {
		c.Assert(frPerm, qt.Equals, "http://example.com/blog/fr/sect/doc1/")
		c.Assert(frRelPerm, qt.Equals, "/blog/fr/sect/doc1/")

		// should have a redirect on top level.
		b.AssertFileContent("public/index.html", `<meta http-equiv="refresh" content="0; url=http://example.com/blog/fr">`)
	} else {
		// Main language in root
		c.Assert(frPerm, qt.Equals, "http://example.com/blog/sect/doc1/")
		c.Assert(frRelPerm, qt.Equals, "/blog/sect/doc1/")

		// should have redirect back to root
		b.AssertFileContent("public/fr/index.html", `<meta http-equiv="refresh" content="0; url=http://example.com/blog">`)
	}
	b.AssertFileContent(pathMod("public/fr/index.html"), "Home", "Bonjour")
	b.AssertFileContent("public/en/index.html", "Home", "Hello")

	// Check list pages
	b.AssertFileContent(pathMod("public/fr/sect/index.html"), "List", "Bonjour")
	b.AssertFileContent("public/en/sect/index.html", "List", "Hello")
	// TODO1	b.AssertFileContent(pathMod("public/fr/plaques/FRtag1/index.html"), "Taxonomy List", "Bonjour")
	b.AssertFileContent("public/en/tags/tag1/index.html", "Taxonomy List", "Hello")

	// Check sitemaps
	// Sitemaps behaves different: In a multilanguage setup there will always be a index file and
	// one sitemap in each lang folder.
	b.AssertFileContent("public/sitemap.xml",
		"<loc>http://example.com/blog/en/sitemap.xml</loc>",
		"<loc>http://example.com/blog/fr/sitemap.xml</loc>")

	if defaultInSubDir {
		b.AssertFileContent("public/fr/sitemap.xml", "<loc>http://example.com/blog/fr/</loc>")
	} else {
		b.AssertFileContent("public/fr/sitemap.xml", "<loc>http://example.com/blog/</loc>")
	}
	b.AssertFileContent("public/en/sitemap.xml", "<loc>http://example.com/blog/en/</loc>")

	// Check rss
	b.AssertFileContent(pathMod("public/fr/index.xml"), pathMod(`<atom:link href="http://example.com/blog/fr/index.xml"`),
		`rel="self" type="application/rss+xml"`)
	b.AssertFileContent("public/en/index.xml", `<atom:link href="http://example.com/blog/en/index.xml"`)
	b.AssertFileContent(
		pathMod("public/fr/sect/index.xml"),
		pathMod(`<atom:link href="http://example.com/blog/fr/sect/index.xml"`))
	b.AssertFileContent("public/en/sect/index.xml", `<atom:link href="http://example.com/blog/en/sect/index.xml"`)
	// TODO1	b.AssertFileContent(
	// TODO1		pathMod("public/fr/plaques/FRtag1/index.xml"),
	// TODO1		pathMod(`<atom:link href="http://example.com/blog/fr/plaques/FRtag1/index.xml"`))
	b.AssertFileContent("public/en/tags/tag1/index.xml", `<atom:link href="http://example.com/blog/en/tags/tag1/index.xml"`)

	// Check paginators
	b.AssertFileContent(pathMod("public/fr/page/1/index.html"), pathMod(`refresh" content="0; url=http://example.com/blog/fr/"`))
	b.AssertFileContent("public/en/page/1/index.html", `refresh" content="0; url=http://example.com/blog/en/"`)
	b.AssertFileContent(pathMod("public/fr/page/2/index.html"), "Home Page 2", "Bonjour", pathMod("http://example.com/blog/fr/"))
	b.AssertFileContent("public/en/page/2/index.html", "Home Page 2", "Hello", "http://example.com/blog/en/")
	b.AssertFileContent(pathMod("public/fr/sect/page/1/index.html"), pathMod(`refresh" content="0; url=http://example.com/blog/fr/sect/"`))
	b.AssertFileContent("public/en/sect/page/1/index.html", `refresh" content="0; url=http://example.com/blog/en/sect/"`)
	b.AssertFileContent(pathMod("public/fr/sect/page/2/index.html"), "List Page 2", "Bonjour", pathMod("http://example.com/blog/fr/sect/"))
	b.AssertFileContent("public/en/sect/page/2/index.html", "List Page 2", "Hello", "http://example.com/blog/en/sect/")
	// TODO1b.AssertFileContent(
	// TODO1	pathMod("public/fr/plaques/FRtag1/page/1/index.html"),
	// TODO1	pathMod(`refresh" content="0; url=http://example.com/blog/fr/plaques/FRtag1/"`))
	b.AssertFileContent("public/en/tags/tag1/page/1/index.html", `refresh" content="0; url=http://example.com/blog/en/tags/tag1/"`)
	// TODO1	b.AssertFileContent(
	// TODO1	pathMod("public/fr/plaques/FRtag1/page/2/index.html"), "List Page 2", "Bonjour",
	// TODO1	pathMod("http://example.com/blog/fr/plaques/FRtag1/"))
	b.AssertFileContent("public/en/tags/tag1/page/2/index.html", "List Page 2", "Hello", "http://example.com/blog/en/tags/tag1/")
	// nn (Nynorsk) and nb (Bokmål) have custom pagePath: side ("page" in Norwegian)
	b.AssertFileContent("public/nn/side/1/index.html", `refresh" content="0; url=http://example.com/blog/nn/"`)
	b.AssertFileContent("public/nb/side/1/index.html", `refresh" content="0; url=http://example.com/blog/nb/"`)
}

func TestMultiSitesWithTwoLanguages(t *testing.T) {
	t.Parallel()

	c := qt.New(t)
	b := newTestSitesBuilder(t).WithConfigFile("toml", `

defaultContentLanguage = "nn"

[languages]
[languages.nn]
languageName = "Nynorsk"
weight = 1
title = "Tittel på Nynorsk"
[languages.nn.params]
p1 = "p1nn"

[languages.en]
title = "Title in English"
languageName = "English"
weight = 2
[languages.en.params]
p1 = "p1en"
`)

	b.CreateSites()
	b.Build(BuildCfg{SkipRender: true})
	sites := b.H.Sites

	c.Assert(len(sites), qt.Equals, 2)

	nnSite := sites[0]
	nnHome := nnSite.getPage(pagekinds.Home)
	c.Assert(len(nnHome.AllTranslations()), qt.Equals, 2)
	c.Assert(len(nnHome.Translations()), qt.Equals, 1)
	c.Assert(nnHome.IsTranslated(), qt.Equals, true)

	enHome := sites[1].getPage(pagekinds.Home)

	p1, err := enHome.Param("p1")
	c.Assert(err, qt.IsNil)
	c.Assert(p1, qt.Equals, "p1en")

	p1, err = nnHome.Param("p1")
	c.Assert(err, qt.IsNil)
	c.Assert(p1, qt.Equals, "p1nn")
}

func TestMultiSitesBuild(t *testing.T) {
	for _, config := range []struct {
		content string
		suffix  string
	}{
		{multiSiteTOMLConfigTemplate, "toml"},
		{multiSiteYAMLConfigTemplate, "yml"},
		{multiSiteJSONConfigTemplate, "json"},
	} {
		config := config
		t.Run(config.suffix, func(t *testing.T) {
			if config.suffix != "toml" {
				t.Skip("TODO1: fix this test")
			}
			t.Parallel()
			doTestMultiSitesBuild(t, config.content, config.suffix)
		})
	}
}

func doTestMultiSitesBuild(t *testing.T, configTemplate, configSuffix string) {
	c := qt.New(t)

	b := newMultiSiteTestBuilder(t, configSuffix, configTemplate, nil)
	b.CreateSites()

	sites := b.H.Sites
	c.Assert(len(sites), qt.Equals, 4)

	b.Build(BuildCfg{})

	// Check site config
	for _, s := range sites {
		c.Assert(s.conf.DefaultContentLanguageInSubdir, qt.Equals, true)
		c.Assert(s.conf.C.DisabledKinds, qt.Not(qt.IsNil))
	}

	gp1 := b.H.GetContentPage(filepath.FromSlash("content/sect/doc1.en.md"))
	c.Assert(gp1, qt.Not(qt.IsNil))
	c.Assert(gp1.Title(), qt.Equals, "doc1")
	gp2 := b.H.GetContentPage(filepath.FromSlash("content/dummysect/notfound.md"))
	c.Assert(gp2, qt.IsNil)

	enSite := sites[0]
	enSiteHome := enSite.getPage(pagekinds.Home)
	c.Assert(enSiteHome.IsTranslated(), qt.Equals, true)

	c.Assert(enSite.language.Lang, qt.Equals, "en")

	c.Assert(len(enSite.RegularPages()), qt.Equals, 5)

	// Check 404s
	b.AssertFileContent("public/en/404.html", "404|en|404 Page not found")
	b.AssertFileContent("public/fr/404.html", "404|fr|404 Page not found")

	// Check robots.txt
	// the domain root is the public directory, so the robots.txt has to be created there and not in the language directories
	b.AssertFileContent("public/robots.txt", "robots")
	b.AssertFileDoesNotExist("public/en/robots.txt")
	b.AssertFileDoesNotExist("public/nn/robots.txt")

	b.AssertFileContent("public/en/sect/doc1-slug/index.html", "Permalink: http://example.com/blog/en/sect/doc1-slug/")
	b.AssertFileContent("public/en/sect/doc2/index.html", "Permalink: http://example.com/blog/en/sect/doc2/")
	b.AssertFileContent("public/superbob/index.html", "Permalink: http://example.com/blog/superbob/")

	doc2 := enSite.RegularPages()[1]
	doc3 := enSite.RegularPages()[2]
	c.Assert(doc3, qt.Equals, doc2.Prev())
	doc1en := enSite.RegularPages()[0]
	doc1fr := doc1en.Translations()[0]
	b.AssertFileContent("public/fr/sect/doc1/index.html", "Permalink: http://example.com/blog/fr/sect/doc1/")

	c.Assert(doc1fr, qt.Equals, doc1en.Translations()[0])
	c.Assert(doc1en, qt.Equals, doc1fr.Translations()[0])
	c.Assert(doc1fr.Language().Lang, qt.Equals, "fr")

	doc4 := enSite.AllPages()[4]
	c.Assert(len(doc4.Translations()), qt.Equals, 0)

	// Taxonomies and their URLs
	c.Assert(len(enSite.Taxonomies()), qt.Equals, 1)
	tags := enSite.Taxonomies()["tags"]
	c.Assert(len(tags), qt.Equals, 2)
	c.Assert(doc1en, qt.Equals, tags["tag1"][0].Page)

	frSite := sites[1]
	c.Assert(frSite.language.Lang, qt.Equals, "fr")
	c.Assert(len(frSite.RegularPages()), qt.Equals, 4)
	c.Assert(frSite.home.Title(), qt.Equals, "Le Français")
	c.Assert(len(frSite.AllPages()), qt.Equals, 32)

	for _, frenchPage := range frSite.RegularPages() {
		p := frenchPage
		c.Assert(p.Language().Lang, qt.Equals, "fr")
	}

	// See https://github.com/gohugoio/hugo/issues/4285
	// Before Hugo 0.33 you had to be explicit with the content path to get the correct Page, which
	// isn't ideal in a multilingual setup. You want a way to get the current language version if available.
	// Now you can do lookups with translation base name to get that behaviour.
	// Let us test all the regular page variants:
	getPageDoc1En := enSite.getPage(pagekinds.Page, filepath.ToSlash(doc1en.File().Path()))
	getPageDoc1EnBase := enSite.getPage(pagekinds.Page, "sect/doc1")
	getPageDoc1Fr := frSite.getPage(pagekinds.Page, filepath.ToSlash(doc1fr.File().Path()))
	getPageDoc1FrBase := frSite.getPage(pagekinds.Page, "sect/doc1")
	c.Assert(getPageDoc1En, qt.Equals, doc1en)
	c.Assert(getPageDoc1Fr, qt.Equals, doc1fr)
	c.Assert(getPageDoc1EnBase, qt.Equals, doc1en)
	c.Assert(getPageDoc1FrBase, qt.Equals, doc1fr)

	// Check redirect to main language, French
	b.AssertFileContent("public/index.html", "0; url=http://example.com/blog/fr")

	// check home page content (including data files rendering)
	b.AssertFileContent("public/en/index.html", "Default Home Page 1", "Hello", "Hugo Rocks!")
	b.AssertFileContent("public/fr/index.html", "French Home Page 1", "Bonjour", "Hugo Rocks!")

	// check single page content
	b.AssertFileContent("public/fr/sect/doc1/index.html", "Single", "Shortcode: Bonjour", "LingoFrench")
	b.AssertFileContent("public/en/sect/doc1-slug/index.html", "Single", "Shortcode: Hello", "LingoDefault")

	// Check node translations
	homeEn := enSite.getPage(pagekinds.Home)
	c.Assert(homeEn, qt.Not(qt.IsNil))
	c.Assert(len(homeEn.Translations()), qt.Equals, 3)
	c.Assert(homeEn.Translations()[0].Language().Lang, qt.Equals, "fr")
	c.Assert(homeEn.Translations()[1].Language().Lang, qt.Equals, "nn")
	c.Assert(homeEn.Translations()[1].Title(), qt.Equals, "På nynorsk")
	c.Assert(homeEn.Translations()[2].Language().Lang, qt.Equals, "nb")
	c.Assert(homeEn.Translations()[2].Title(), qt.Equals, "På bokmål")
	c.Assert(homeEn.Translations()[2].Language().LanguageName, qt.Equals, "Bokmål")

	sectFr := frSite.getPage("sect")
	c.Assert(sectFr, qt.Not(qt.IsNil))

	sectEn := enSite.getPage("sect")
	c.Assert(sectEn, qt.Not(qt.IsNil))

	c.Assert(sectFr.Language().Lang, qt.Equals, "fr")
	c.Assert(len(sectFr.Translations()), qt.Equals, 1)
	c.Assert(sectFr.Translations()[0].Language().Lang, qt.Equals, "en")
	c.Assert(sectFr.Translations()[0].Title(), qt.Equals, "Sects")

	nnSite := sites[2]
	c.Assert(nnSite.language.Lang, qt.Equals, "nn")
	taxNn := nnSite.getPage(pagekinds.Taxonomy, "lag")
	c.Assert(taxNn, qt.Not(qt.IsNil))
	c.Assert(len(taxNn.Translations()), qt.Equals, 1)
	c.Assert(taxNn.Translations()[0].Language().Lang, qt.Equals, "nb")

	taxTermNn := nnSite.getPage(pagekinds.Term, "lag", "sogndal")
	c.Assert(taxTermNn, qt.Not(qt.IsNil))
	c.Assert(nnSite.getPage(pagekinds.Term, "LAG", "SOGNDAL"), qt.Equals, taxTermNn)
	c.Assert(len(taxTermNn.Translations()), qt.Equals, 1)
	c.Assert(taxTermNn.Translations()[0].Language().Lang, qt.Equals, "nb")

	// Check sitemap(s)
	b.AssertFileContent("public/sitemap.xml",
		"<loc>http://example.com/blog/en/sitemap.xml</loc>",
		"<loc>http://example.com/blog/fr/sitemap.xml</loc>")
	b.AssertFileContent("public/en/sitemap.xml", "http://example.com/blog/en/sect/doc2/")
	b.AssertFileContent("public/fr/sitemap.xml", "http://example.com/blog/fr/sect/doc1/")

	// Check taxonomies
	enTags := enSite.Taxonomies()["tags"]
	frTags := frSite.Taxonomies()["plaques"]
	c.Assert(len(enTags), qt.Equals, 2, qt.Commentf("Tags in en: %v", enTags))
	c.Assert(len(frTags), qt.Equals, 2, qt.Commentf("Tags in fr: %v", frTags))
	c.Assert(enTags["tag1"], qt.Not(qt.IsNil))
	// TODO1 create issue about this slightly breaking change. Also consider Section().
	frtag1 := frTags["frtag1"]
	c.Assert(frtag1, qt.Not(qt.IsNil))
	c.Assert(frtag1.Page().Title(), qt.Equals, "FRtag1")
	// TODO1	b.AssertFileContent("public/fr/plaques/FRtag1/index.html", "FRtag1|Bonjour|http://example.com/blog/fr/plaques/FRtag1/")

	// en and nn have custom site menus
	c.Assert(len(frSite.Menus()), qt.Equals, 0)
	c.Assert(len(enSite.Menus()), qt.Equals, 1)
	c.Assert(len(nnSite.Menus()), qt.Equals, 1)

	c.Assert(enSite.Menus()["main"].ByName()[0].Name, qt.Equals, "Home")
	c.Assert(nnSite.Menus()["main"].ByName()[0].Name, qt.Equals, "Heim")

	// Issue #3108
	prevPage := enSite.RegularPages()[0].Prev()
	c.Assert(prevPage, qt.Not(qt.IsNil))
	c.Assert(prevPage.Kind(), qt.Equals, pagekinds.Page)

	for {
		if prevPage == nil {
			break
		}
		c.Assert(prevPage.Kind(), qt.Equals, pagekinds.Page)
		prevPage = prevPage.Prev()
	}

	// Check bundles
	b.AssertFileContent("public/fr/bundles/b1/index.html", "RelPermalink: /blog/fr/bundles/b1/|")
	bundleFr := frSite.getPage(pagekinds.Page, "bundles/b1/index.md")
	c.Assert(bundleFr, qt.Not(qt.IsNil))
	c.Assert(len(bundleFr.Resources()), qt.Equals, 1)
	logoFr := bundleFr.Resources().GetMatch("logo*")
	logoFrGet := bundleFr.Resources().Get("logo.png")
	c.Assert(logoFrGet, qt.Equals, logoFr)
	c.Assert(logoFr, qt.Not(qt.IsNil))

	b.AssertFileContent("public/fr/bundles/b1/index.html", "Resources: image/png: /blog/fr/bundles/b1/logo.png")
	b.AssertFileContent("public/fr/bundles/b1/logo.png", "PNG Data")

	bundleEn := enSite.getPage(pagekinds.Page, "bundles/b1/index.en.md")
	c.Assert(bundleEn, qt.Not(qt.IsNil))
	b.AssertFileContent("public/en/bundles/b1/index.html", "RelPermalink: /blog/en/bundles/b1/|")
	c.Assert(len(bundleEn.Resources()), qt.Equals, 1)
	logoEn := bundleEn.Resources().GetMatch("logo*")
	c.Assert(logoEn, qt.Not(qt.IsNil))
	b.AssertFileContent("public/en/bundles/b1/index.html", "Resources: image/png: /blog/en/bundles/b1/logo.png")
	b.AssertFileContent("public/en/bundles/b1/logo.png", "PNG Data")
}

// https://github.com/gohugoio/hugo/issues/4706
func TestContentStressTest(t *testing.T) {
	b := newTestSitesBuilder(t)

	numPages := 500

	contentTempl := `
---
%s
title: %q
weight: %d
multioutput: %t
---

# Header

CONTENT

The End.
`

	contentTempl = strings.Replace(contentTempl, "CONTENT", strings.Repeat(`
	
## Another header

Some text. Some more text.

`, 100), -1)

	var content []string
	defaultOutputs := `outputs: ["html", "json", "rss" ]`

	for i := 1; i <= numPages; i++ {
		outputs := defaultOutputs
		multioutput := true
		if i%3 == 0 {
			outputs = `outputs: ["json"]`
			multioutput = false
		}
		section := "s1"
		if i%10 == 0 {
			section = "s2"
		}
		content = append(content, []string{fmt.Sprintf("%s/page%d.md", section, i), fmt.Sprintf(contentTempl, outputs, fmt.Sprintf("Title %d", i), i, multioutput)}...)
	}

	content = append(content, []string{"_index.md", fmt.Sprintf(contentTempl, defaultOutputs, fmt.Sprintf("Home %d", 0), 0, true)}...)
	content = append(content, []string{"s1/_index.md", fmt.Sprintf(contentTempl, defaultOutputs, fmt.Sprintf("S %d", 1), 1, true)}...)
	content = append(content, []string{"s2/_index.md", fmt.Sprintf(contentTempl, defaultOutputs, fmt.Sprintf("S %d", 2), 2, true)}...)

	b.WithSimpleConfigFile()
	b.WithTemplates("layouts/_default/single.html", `Single: {{ .Content }}|RelPermalink: {{ .RelPermalink }}|Permalink: {{ .Permalink }}`)
	b.WithTemplates("layouts/_default/myview.html", `View: {{ len .Content }}`)
	b.WithTemplates("layouts/_default/single.json", `Single JSON: {{ .Content }}|RelPermalink: {{ .RelPermalink }}|Permalink: {{ .Permalink }}`)
	b.WithTemplates("layouts/_default/list.html", `
Page: {{ .Paginator.PageNumber }}
P: {{ with .File }}{{ path.Join .Path }}{{ end }}
List: {{ len .Paginator.Pages }}|List Content: {{ len .Content }}
{{ $shuffled :=  where .Site.RegularPages "Params.multioutput" true | shuffle }}
{{ $first5 := $shuffled | first 5 }}
L1: {{ len .Site.RegularPages }} L2: {{ len $first5 }}
{{ range $i, $e := $first5 }}
Render {{ $i }}: {{ .Render "myview" }}
{{ end }}
END
`)

	b.WithContent(content...)

	b.CreateSites().Build(BuildCfg{})

	contentMatchers := []string{"<h2 id=\"another-header\">Another header</h2>", "<h2 id=\"another-header-99\">Another header</h2>", "<p>The End.</p>"}

	for i := 1; i <= numPages; i++ {
		if i%3 != 0 {
			section := "s1"
			if i%10 == 0 {
				section = "s2"
			}
			checkContent(b, fmt.Sprintf("public/%s/page%d/index.html", section, i), contentMatchers...)
		}
	}

	for i := 1; i <= numPages; i++ {
		section := "s1"
		if i%10 == 0 {
			section = "s2"
		}
		checkContent(b, fmt.Sprintf("public/%s/page%d/index.json", section, i), contentMatchers...)
	}

	checkContent(b, "public/s1/index.html", "P: s1/_index.md\nList: 10|List Content: 8132\n\n\nL1: 500 L2: 5\n\nRender 0: View: 8132\n\nRender 1: View: 8132\n\nRender 2: View: 8132\n\nRender 3: View: 8132\n\nRender 4: View: 8132\n\nEND\n")
	checkContent(b, "public/s2/index.html", "P: s2/_index.md\nList: 10|List Content: 8132", "Render 4: View: 8132\n\nEND")
	checkContent(b, "public/index.html", "P: _index.md\nList: 10|List Content: 8132", "4: View: 8132\n\nEND")

	// Check paginated pages
	for i := 2; i <= 9; i++ {
		checkContent(b, fmt.Sprintf("public/page/%d/index.html", i), fmt.Sprintf("Page: %d", i), "Content: 8132\n\n\nL1: 500 L2: 5\n\nRender 0: View: 8132", "Render 4: View: 8132\n\nEND")
	}
}

func checkContent(s *sitesBuilder, filename string, matches ...string) {
	s.T.Helper()
	content := readWorkingDir(s.T, s.Fs, filename)
	for _, match := range matches {
		if !strings.Contains(content, match) {
			s.Fatalf("No match for\n%q\nin content for %s\n%q\nDiff:\n%s", match, filename, content, htesting.DiffStrings(content, match))
		}
	}
}

func TestTranslationsFromContentToNonContent(t *testing.T) {
	b := newTestSitesBuilder(t)
	b.WithConfigFile("toml", `

baseURL = "http://example.com/"

defaultContentLanguage = "en"

[languages]
[languages.en]
weight = 10
contentDir = "content/en"
[languages.nn]
weight = 20
contentDir = "content/nn"


`)

	b.WithContent("en/mysection/_index.md", `
---
Title: My Section
---

`)

	b.WithContent("en/_index.md", `
---
Title: My Home
---

`)

	b.WithContent("en/categories/mycat/_index.md", `
---
Title: My MyCat
---

`)

	b.WithContent("en/categories/_index.md", `
---
Title: My categories
---

`)

	for _, lang := range []string{"en", "nn"} {
		b.WithContent(lang+"/mysection/page.md", `
---
Title: My Page
categories: ["mycat"]
---

`)
	}

	b.Build(BuildCfg{})

	for _, path := range []string{
		"/",
		"/mysection",
		"/categories",
		"/categories/mycat",
	} {
		t.Run(path, func(t *testing.T) {
			c := qt.New(t)

			s1, _ := b.H.Sites[0].getPageNew(nil, path)
			s2, _ := b.H.Sites[1].getPageNew(nil, path)

			c.Assert(s1, qt.Not(qt.IsNil))
			c.Assert(s2, qt.Not(qt.IsNil))

			c.Assert(len(s1.Translations()), qt.Equals, 1)
			c.Assert(len(s2.Translations()), qt.Equals, 1)
			c.Assert(s1.Translations()[0], qt.Equals, s2)
			c.Assert(s2.Translations()[0], qt.Equals, s1)

			m1 := s1.Translations().MergeByLanguage(s2.Translations())
			m2 := s2.Translations().MergeByLanguage(s1.Translations())

			c.Assert(len(m1), qt.Equals, 1)
			c.Assert(len(m2), qt.Equals, 1)
		})
	}
}

var multiSiteTOMLConfigTemplate = `
baseURL = "http://example.com/blog"

paginate = 1
disablePathToLower = true
defaultContentLanguage = "{{ .DefaultContentLanguage }}"
defaultContentLanguageInSubdir = {{ .DefaultContentLanguageInSubdir }}
enableRobotsTXT = true

[permalinks]
other = "/somewhere/else/:filename"

[Taxonomies]
tag = "tags"

[Languages]
[Languages.en]
weight = 10
title = "In English"
languageName = "English"
[[Languages.en.menu.main]]
url    = "/"
name   = "Home"
weight = 0

[Languages.fr]
weight = 20
title = "Le Français"
languageName = "Français"
[Languages.fr.Taxonomies]
plaque = "plaques"

[Languages.nn]
weight = 30
title = "På nynorsk"
languageName = "Nynorsk"
paginatePath = "side"
[Languages.nn.Taxonomies]
lag = "lag"
[[Languages.nn.menu.main]]
url    = "/"
name   = "Heim"
weight = 1

[Languages.nb]
weight = 40
title = "På bokmål"
languageName = "Bokmål"
paginatePath = "side"
[Languages.nb.Taxonomies]
lag = "lag"
`

var multiSiteYAMLConfigTemplate = `
baseURL: "http://example.com/blog"

disablePathToLower: true
paginate: 1
defaultContentLanguage: "{{ .DefaultContentLanguage }}"
defaultContentLanguageInSubdir: {{ .DefaultContentLanguageInSubdir }}
enableRobotsTXT: true

permalinks:
    other: "/somewhere/else/:filename"

Taxonomies:
    tag: "tags"

Languages:
    en:
        weight: 10
        title: "In English"
        languageName: "English"
        menu:
            main:
                - url: "/"
                  name: "Home"
                  weight: 0
    fr:
        weight: 20
        title: "Le Français"
        languageName: "Français"
        Taxonomies:
            plaque: "plaques"
    nn:
        weight: 30
        title: "På nynorsk"
        languageName: "Nynorsk"
        paginatePath: "side"
        Taxonomies:
            lag: "lag"
        menu:
            main:
                - url: "/"
                  name: "Heim"
                  weight: 1
    nb:
        weight: 40
        title: "På bokmål"
        languageName: "Bokmål"
        paginatePath: "side"
        Taxonomies:
            lag: "lag"

`

// TODO(bep) clean move
var multiSiteJSONConfigTemplate = `
{
  "baseURL": "http://example.com/blog",
  "paginate": 1,
  "disablePathToLower": true,
  "defaultContentLanguage": "{{ .DefaultContentLanguage }}",
  "defaultContentLanguageInSubdir": true,
  "enableRobotsTXT": true,
  "permalinks": {
    "other": "/somewhere/else/:filename"
  },
  "Taxonomies": {
    "tag": "tags"
  },
  "Languages": {
    "en": {
      "weight": 10,
      "title": "In English",
      "languageName": "English",
	  "menu": {
        "main": [
			{
			"url": "/",
			"name": "Home",
			"weight": 0
			}
		]
      }
    },
    "fr": {
      "weight": 20,
      "title": "Le Français",
      "languageName": "Français",
      "Taxonomies": {
        "plaque": "plaques"
      }
    },
    "nn": {
      "weight": 30,
      "title": "På nynorsk",
      "paginatePath": "side",
      "languageName": "Nynorsk",
      "Taxonomies": {
        "lag": "lag"
      },
	  "menu": {
        "main": [
			{
        	"url": "/",
			"name": "Heim",
			"weight": 1
			}
      	]
      }
    },
    "nb": {
      "weight": 40,
      "title": "På bokmål",
      "paginatePath": "side",
      "languageName": "Bokmål",
      "Taxonomies": {
        "lag": "lag"
      }
    }
  }
}
`

func writeSource(t testing.TB, fs *hugofs.Fs, filename, content string) {
	t.Helper()
	writeToFs(t, fs.Source, filename, content)
}

func writeToFs(t testing.TB, fs afero.Fs, filename, content string) {
	t.Helper()
	if err := afero.WriteFile(fs, filepath.FromSlash(filename), []byte(content), 0755); err != nil {
		t.Fatalf("Failed to write file: %s", err)
	}
}

func readWorkingDir(t testing.TB, fs *hugofs.Fs, filename string) string {
	t.Helper()
	return readFileFromFs(t, fs.WorkingDirReadOnly, filename)
}

func workingDirExists(fs *hugofs.Fs, filename string) bool {
	b, err := helpers.Exists(filename, fs.WorkingDirReadOnly)
	if err != nil {
		panic(err)
	}
	return b
}

func readSource(t *testing.T, fs *hugofs.Fs, filename string) string {
	return readFileFromFs(t, fs.Source, filename)
}

func readFileFromFs(t testing.TB, fs afero.Fs, filename string) string {
	t.Helper()
	filename = filepath.Clean(filename)
	b, err := afero.ReadFile(fs, filename)
	if err != nil {
		// Print some debug info
		hadSlash := strings.HasPrefix(filename, helpers.FilePathSeparator)
		start := 0
		if hadSlash {
			start = 1
		}
		end := start + 1

		parts := strings.Split(filename, helpers.FilePathSeparator)
		if parts[start] == "work" {
			end++
		}

		/*
			root := filepath.Join(parts[start:end]...)
			if hadSlash {
				root = helpers.FilePathSeparator + root
			}

			helpers.PrintFs(fs, root, os.Stdout)
		*/

		t.Fatalf("Failed to read file: %s", err)
	}
	return string(b)
}

const testPageTemplate = `---
title: "%s"
publishdate: "%s"
weight: %d
---
# Doc %s
`

func newTestPage(title, date string, weight int) string {
	return fmt.Sprintf(testPageTemplate, title, date, weight, title)
}

type multiSiteTestBuilder struct {
	configData   any
	config       string
	configFormat string

	*sitesBuilder
}

func newMultiSiteTestDefaultBuilder(t testing.TB) *multiSiteTestBuilder {
	return newMultiSiteTestBuilder(t, "", "", nil)
}

func (b *multiSiteTestBuilder) WithNewConfig(config string) *multiSiteTestBuilder {
	b.WithConfigTemplate(b.configData, b.configFormat, config)
	return b
}

func (b *multiSiteTestBuilder) WithNewConfigData(data any) *multiSiteTestBuilder {
	b.WithConfigTemplate(data, b.configFormat, b.config)
	return b
}

func newMultiSiteTestBuilder(t testing.TB, configFormat, config string, configData any) *multiSiteTestBuilder {
	if configData == nil {
		configData = map[string]any{
			"DefaultContentLanguage":         "fr",
			"DefaultContentLanguageInSubdir": true,
		}
	}

	if config == "" {
		config = multiSiteTOMLConfigTemplate
	}

	if configFormat == "" {
		configFormat = "toml"
	}

	b := newTestSitesBuilder(t).WithConfigTemplate(configData, configFormat, config)
	b.WithContent("root.en.md", `---
title: root
weight: 10000
slug: root
publishdate: "2000-01-01"
---
# root
`,
		"sect/doc1.en.md", `---
title: doc1
weight: 1
slug: doc1-slug
tags:
 - tag1
publishdate: "2000-01-01"
---
# doc1
*some "content"*

{{< shortcode >}}

{{< lingo >}}

NOTE: slug should be used as URL
`,
		"sect/doc1.fr.md", `---
title: doc1
weight: 1
plaques:
 - FRtag1
 - FRtag2
publishdate: "2000-01-04"
---
# doc1
*quelque "contenu"*

{{< shortcode >}}

{{< lingo >}}

NOTE: should be in the 'en' Page's 'Translations' field.
NOTE: date is after "doc3"
`,
		"sect/doc2.en.md", `---
title: doc2
weight: 2
publishdate: "2000-01-02"
---
# doc2
*some content*
NOTE: without slug, "doc2" should be used, without ".en" as URL
`,
		"sect/doc3.en.md", `---
title: doc3
weight: 3
publishdate: "2000-01-03"
aliases: [/en/al/alias1,/al/alias2/]
tags:
 - tag2
 - tag1
url: /superbob/
---
# doc3
*some content*
NOTE: third 'en' doc, should trigger pagination on home page.
`,
		"sect/doc4.md", `---
title: doc4
weight: 4
plaques:
 - FRtag1
publishdate: "2000-01-05"
---
# doc4
*du contenu francophone*
NOTE: should use the defaultContentLanguage and mark this doc as 'fr'.
NOTE: doesn't have any corresponding translation in 'en'
`,
		"other/doc5.fr.md", `---
title: doc5
weight: 5
publishdate: "2000-01-06"
---
# doc5
*autre contenu francophone*
NOTE: should use the "permalinks" configuration with :filename
`,
		// Add some for the stats
		"stats/expired.fr.md", `---
title: expired
publishdate: "2000-01-06"
expiryDate: "2001-01-06"
---
# Expired
`,
		"stats/future.fr.md", `---
title: future
weight: 6
publishdate: "2100-01-06"
---
# Future
`,
		"stats/expired.en.md", `---
title: expired
weight: 7
publishdate: "2000-01-06"
expiryDate: "2001-01-06"
---
# Expired
`,
		"stats/future.en.md", `---
title: future
weight: 6
publishdate: "2100-01-06"
---
# Future
`,
		"stats/draft.en.md", `---
title: expired
publishdate: "2000-01-06"
draft: true
---
# Draft
`,
		"stats/tax.nn.md", `---
title: Tax NN
weight: 8
publishdate: "2000-01-06"
weight: 1001
lag:
- Sogndal
---
# Tax NN
`,
		"stats/tax.nb.md", `---
title: Tax NB
weight: 8
publishdate: "2000-01-06"
weight: 1002
lag:
- Sogndal
---
# Tax NB
`,
		// Bundle
		"bundles/b1/index.en.md", `---
title: Bundle EN
publishdate: "2000-01-06"
weight: 2001
---
# Bundle Content EN
`,
		"bundles/b1/index.md", `---
title: Bundle Default
publishdate: "2000-01-06"
weight: 2002
---
# Bundle Content Default
`,
		"bundles/b1/logo.png", `
PNG Data
`)

	i18nContent := func(id, value string) string {
		return fmt.Sprintf(`
[%s]
other = %q
`, id, value)
	}

	b.WithSourceFile("i18n/en.toml", i18nContent("hello", "Hello"))
	b.WithSourceFile("i18n/fr.toml", i18nContent("hello", "Bonjour"))
	b.WithSourceFile("i18n/nb.toml", i18nContent("hello", "Hallo"))
	b.WithSourceFile("i18n/nn.toml", i18nContent("hello", "Hallo"))

	return &multiSiteTestBuilder{sitesBuilder: b, configFormat: configFormat, config: config, configData: configData}
}
