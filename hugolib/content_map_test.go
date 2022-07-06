// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hugolib

import (
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/hugolib/doctree"
)

// TODO1
func _TestContentMapSite(t *testing.T) {
	b := newTestSitesBuilder(t)

	pageTempl := `
---
title: "Page %d"
date: "2019-06-0%d"	
lastMod: "2019-06-0%d"
categories: [%q]
---

Page content.
`
	createPage := func(i int) string {
		return fmt.Sprintf(pageTempl, i, i, i+1, "funny")
	}

	createPageInCategory := func(i int, category string) string {
		return fmt.Sprintf(pageTempl, i, i, i+1, category)
	}

	draftTemplate := `---
title: "Draft"
draft: true
---

`

	b.WithContent("_index.md", `
---
title: "Hugo Home"
cascade:
    description: "Common Description"
    
---

Home Content.
`)

	b.WithContent("blog/page1.md", createPage(1))
	b.WithContent("blog/page2.md", createPage(2))
	b.WithContent("blog/page3.md", createPage(3))
	b.WithContent("blog/bundle/index.md", createPage(12))
	b.WithContent("blog/bundle/data.json", "data")
	b.WithContent("blog/bundle/page.md", createPage(99))
	b.WithContent("blog/subsection/_index.md", createPage(3))
	b.WithContent("blog/subsection/subdata.json", "data")
	b.WithContent("blog/subsection/page4.md", createPage(8))
	b.WithContent("blog/subsection/page5.md", createPage(10))
	b.WithContent("blog/subsection/draft/index.md", draftTemplate)
	b.WithContent("blog/subsection/draft/data.json", "data")
	b.WithContent("blog/draftsection/_index.md", draftTemplate)
	b.WithContent("blog/draftsection/page/index.md", createPage(12))
	b.WithContent("blog/draftsection/page/folder/data.json", "data")
	b.WithContent("blog/draftsection/sub/_index.md", createPage(12))
	b.WithContent("blog/draftsection/sub/page.md", createPage(13))
	b.WithContent("docs/page6.md", createPage(11))
	b.WithContent("tags/_index.md", createPageInCategory(32, "sad"))
	b.WithContent("overlap/_index.md", createPageInCategory(33, "sad"))
	b.WithContent("overlap2/_index.md", createPage(34))

	b.WithTemplatesAdded("layouts/index.html", `
Num Regular: {{ len .Site.RegularPages }}
Main Sections: {{ .Site.Params.mainSections }}
Pag Num Pages: {{ len .Paginator.Pages }}
{{ $home := .Site.Home }}
{{ $blog := .Site.GetPage "blog" }}
{{ $categories := .Site.GetPage "categories" }}
{{ $funny := .Site.GetPage "categories/funny" }}
{{ $blogSub := .Site.GetPage "blog/subsection" }}
{{ $page := .Site.GetPage "blog/page1" }}
{{ $page2 := .Site.GetPage "blog/page2" }}
{{ $page4 := .Site.GetPage "blog/subsection/page4" }}
{{ $bundle := .Site.GetPage "blog/bundle" }}
{{ $overlap1 := .Site.GetPage "overlap" }}
{{ $overlap2 := .Site.GetPage "overlap2" }}

Home: {{ template "print-page" $home }}
Blog Section: {{ template "print-page" $blog }}
Blog Sub Section: {{ template "print-page" $blogSub }}
Page: {{ template "print-page" $page }}
Bundle: {{ template "print-page" $bundle }}
IsDescendant: true: {{ $page.IsDescendant $blog }} true: {{ $blogSub.IsDescendant $blog }} true: {{ $bundle.IsDescendant $blog }} true: {{ $page4.IsDescendant $blog }} true: {{ $blog.IsDescendant $home }} false: {{ $blog.IsDescendant $blog }} false: {{ $home.IsDescendant $blog }}
IsAncestor: true: {{ $blog.IsAncestor $page }} true: {{ $home.IsAncestor $blog }} true: {{ $blog.IsAncestor $blogSub }} true: {{ $blog.IsAncestor $bundle }} true: {{ $blog.IsAncestor $page4 }} true: {{ $home.IsAncestor $page }} false: {{ $blog.IsAncestor $blog }} false: {{ $page.IsAncestor $blog }} false: {{ $blog.IsAncestor $home }}  false: {{ $blogSub.IsAncestor $blog }}
IsDescendant overlap1: false: {{ $overlap1.IsDescendant $overlap2 }}
IsDescendant overlap2: false: {{ $overlap2.IsDescendant $overlap1 }}
IsAncestor overlap1: false: {{ $overlap1.IsAncestor $overlap2 }}
IsAncestor overlap2: false: {{ $overlap2.IsAncestor $overlap1 }}
FirstSection: {{ $blogSub.FirstSection.RelPermalink }} {{ $blog.FirstSection.RelPermalink }} {{ $home.FirstSection.RelPermalink }} {{ $page.FirstSection.RelPermalink }}
InSection: true: {{ $page.InSection $blog }} false: {{ $page.InSection $blogSub }} 
Next: {{ $page2.Next.RelPermalink }}
NextInSection: {{ $page2.NextInSection.RelPermalink }}
Pages: {{ range $blog.Pages }}{{ .RelPermalink }}|{{ end }}
Sections: {{ range $home.Sections }}{{ .RelPermalink }}|{{ end }}:END
Categories: {{ range .Site.Taxonomies.categories }}{{ .Page.RelPermalink }}; {{ .Page.Title }}; {{ .Count }}|{{ end }}:END
Category Terms:  {{ $categories.Kind}}: {{ range $categories.Data.Terms.Alphabetical }}{{ .Page.RelPermalink }}; {{ .Page.Title }}; {{ .Count }}|{{ end }}:END
Category Funny:  {{ $funny.Kind}}; {{ $funny.Data.Term }}: {{ range $funny.Pages }}{{ .RelPermalink }};|{{ end }}:END
Pag Num Pages: {{ len .Paginator.Pages }}
Pag Blog Num Pages: {{ len $blog.Paginator.Pages }}
Blog Num RegularPages: {{ len $blog.RegularPages }}|{{ range $blog.RegularPages }}P: {{ .RelPermalink }}|{{ end }}
Blog Num Pages: {{ len $blog.Pages }}

Draft1: {{ if (.Site.GetPage "blog/subsection/draft") }}FOUND{{ end }}|
Draft2: {{ if (.Site.GetPage "blog/draftsection") }}FOUND{{ end }}|
Draft3: {{ if (.Site.GetPage "blog/draftsection/page") }}FOUND{{ end }}|
Draft4: {{ if (.Site.GetPage "blog/draftsection/sub") }}FOUND{{ end }}|
Draft5: {{ if (.Site.GetPage "blog/draftsection/sub/page") }}FOUND{{ end }}|

{{ define "print-page" }}{{ .Title }}|{{ .RelPermalink }}|{{ .Date.Format "2006-01-02" }}|Current Section: {{ with .CurrentSection }}{ .SectionsPath }}{{ else }}NIL{{ end }}|Resources: {{ range .Resources }}{{ .ResourceType }}: {{ .RelPermalink }}|{{ end }}{{ end }}
`)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/index.html",

		`
	 Num Regular: 7
        Main Sections: [blog]
        Pag Num Pages: 7
        
      Home: Hugo Home|/|2019-06-08|Current Section: |Resources: 
        Blog Section: Blogs|/blog/|2019-06-08|Current Section: blog|Resources: 
        Blog Sub Section: Page 3|/blog/subsection/|2019-06-03|Current Section: blog/subsection|Resources: application: /blog/subsection/subdata.json|
        Page: Page 1|/blog/page1/|2019-06-01|Current Section: blog|Resources: 
        Bundle: Page 12|/blog/bundle/|0001-01-01|Current Section: blog|Resources: application: /blog/bundle/data.json|page: |
        IsDescendant: true: true true: true true: true true: true true: true false: false false: false
        IsAncestor: true: true true: true true: true true: true true: true true: true false: false false: false false: false  false: false
        IsDescendant overlap1: false: false
        IsDescendant overlap2: false: false
        IsAncestor overlap1: false: false
        IsAncestor overlap2: false: false
        FirstSection: /blog/ /blog/ / /blog/
        InSection: true: true false: false 
        Next: /blog/page3/
        NextInSection: /blog/page3/
        Pages: /blog/page3/|/blog/subsection/|/blog/page2/|/blog/page1/|/blog/bundle/|
        Sections: /blog/|/docs/|/overlap/|/overlap2/|:END
		Categories: /categories/funny/; funny; 9|/categories/sad/; sad; 2|:END
        Category Terms:  taxonomy: /categories/funny/; funny; 9|/categories/sad/; sad; 2|:END
		Category Funny:  term; funny: /blog/subsection/page4/;|/blog/page3/;|/blog/subsection/;|/blog/page2/;|/blog/page1/;|/blog/subsection/page5/;|/docs/page6/;|/blog/bundle/;|/overlap2/;|:END
 		Pag Num Pages: 7
        Pag Blog Num Pages: 4
        Blog Num RegularPages: 4
        Blog Num Pages: 5
        
        Draft1: |
        Draft2: |
        Draft3: |
        Draft4: |
        Draft5: |
           
`)
}

func TestResolveDimension(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	files := `
-- config.toml --
defaultContentLanguage = "en"
defaultContentLanguageInSubdir = true
[languages]
[languages.en]
title = "Title in English"
languageName = "English"
weight = 1
[languages.nn]
languageName = "Nynorsk"
weight = 2
title = "Tittel på nynorsk"
-- content/s1/mybundle/index.md --
---
title: "My Bundle EN"
---
-- content/s1/mybundle/index.nn.md --
---
title: "My Bundle NN"
---
-- content/s1/mybundle/hugo.txt --
Hugo Rocks!
-- content/s1/mybundle/contentresource.md --
--- 
title: "Content Resource"
---
-- content/s1/mybundle/nynorskonly.nn.txt --
nynorskonly
-- content/s1/mybundle/unknownlang.ab.txt --
unknownlang
-- layouts/index.html --
{{ .Title }}
-- layouts/_default/single.html --
{{ .Title }}|Contentresource: {{ with .Resources.Get "contentresource.md" }}Title: {{ .Title }}{{ else }}NOT FOUND{{ end }}|
		
	`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
		}).Build()

	//	printInfoAboutHugoSites(b.H)

	// See https://github.com/gohugoio/hugo/issues/7437
	b.AssertFileContent("public/en/s1/mybundle/nynorskonly.txt", "nynorskonly")
	b.AssertFileContent("public/nn/s1/mybundle/nynorskonly.txt", "nynorskonly")

	// TODO1 ab is detected as a language, which is tempting as it would make everything simpler.
	b.AssertFileContent("public/en/s1/mybundle/unknownlang.txt", "unknownlang")
	b.AssertFileContent("public/nn/s1/mybundle/unknownlang.txt", "unknownlang")

	b.AssertFileContent("public/nn/s1/mybundle/index.html", "My Bundle NN|Contentresource: NOT FOUND|")
	b.AssertFileContent("public/en/s1/mybundle/index.html", "My Bundle EN|Contentresource: Title: Content Resource|")

	enDim := doctree.Dimension{
		Name:      "en",
		Dimension: pageTreeDimensionLanguage,
		Index:     0,
		Size:      2,
	}

	nnDim := doctree.Dimension{
		Name:      "nn",
		Dimension: pageTreeDimensionLanguage,
		Index:     1,
		Size:      2,
	}

	b.Assert(b.H.resolveDimension(pageTreeDimensionLanguage, b.H.Sites[0]), qt.Equals, enDim)
	b.Assert(b.H.resolveDimension(pageTreeDimensionLanguage, b.H.Sites[0].home), qt.Equals, enDim)
	b.Assert(b.H.resolveDimension(pageTreeDimensionLanguage, b.H.Sites[1]), qt.Equals, nnDim)
	b.Assert(b.H.resolveDimension(pageTreeDimensionLanguage, b.H.Sites[1].home), qt.Equals, nnDim)

}

func TestIntegrationTestTemplate(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	files := `
-- config.toml --
title = "Integration Test"
disableKinds=["page", "section", "taxonomy", "term", "sitemap", "robotsTXT", "RSS"]
-- layouts/index.html --
Home: {{ .Title }}|
		
	`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
		}).Build()

	b.AssertFileContent("public/index.html", "Home: Integration Test|")

}
