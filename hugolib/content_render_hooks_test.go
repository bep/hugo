// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless requiredF by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hugolib

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestRenderHookEditNestedPartial(t *testing.T) {
	config := `
baseURL="https://example.org"
workingDir="/mywork"
`
	b := newTestSitesBuilder(t).WithWorkingDir("/mywork").WithConfigFile("toml", config).Running()

	b.WithTemplates("_default/single.html", "{{ .Content }}")
	b.WithTemplates("partials/mypartial1.html", `PARTIAL1 {{ partial "mypartial2.html" }}`)
	b.WithTemplates("partials/mypartial2.html", `PARTIAL2`)
	b.WithTemplates("_default/_markup/render-link.html", `Link {{ .Text | safeHTML }}|{{ partial "mypartial1.html" . }}END`)

	b.WithContent("p1.md", `---
title: P1
---

[First Link](https://www.google.com "Google's Homepage")

`)
	b.Build(BuildCfg{})

	b.AssertFileContent("public/p1/index.html", `Link First Link|PARTIAL1 PARTIAL2END`)

	b.EditFiles("layouts/partials/mypartial1.html", `PARTIAL1_EDITED {{ partial "mypartial2.html" }}`)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/p1/index.html", `Link First Link|PARTIAL1_EDITED PARTIAL2END`)

	b.EditFiles("layouts/partials/mypartial2.html", `PARTIAL2_EDITED`)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/p1/index.html", `Link First Link|PARTIAL1_EDITED PARTIAL2_EDITEDEND`)
}

func TestRenderHooks(t *testing.T) {
	files := `
-- config.toml --
baseURL="https://example.org"
workingDir="/mywork"
disableKinds=["home", "section", "taxonomy", "term", "sitemap", "robotsTXT"]
[outputs]
  page = ['HTML']
[markup]
[markup.goldmark]
[markup.goldmark.parser]
autoHeadingID = true
autoHeadingIDType = "github"
[markup.goldmark.parser.attribute]
block = true
title = true
-- content/blog/notempl1.md --
---
title: No Template
---

## Content
-- content/blog/notempl2.md --
---
title: No Template
---

## Content
-- content/blog/notempl3.md --
---
title: No Template
---

## Content
-- content/blog/p1.md --
---
title: Cool Page
---

[First Link](https://www.google.com "Google's Homepage")
<https://foo.bar/>
https://bar.baz/
<fake@example.com>
<mailto:fake2@example.com>

{{< myshortcode3 >}}

[Second Link](https://www.google.com "Google's Homepage")

Image:

![Drag Racing](/images/Dragster.jpg "image title")

Attributes:

## Some Heading {.text-serif #a-heading title="Hovered"}
-- content/blog/p2.md --
---
title: Cool Page2
layout: mylayout
---

{{< myshortcode1 >}}

[Some Text](https://www.google.com "Google's Homepage")

,[No Whitespace Please](https://gohugo.io),
-- content/blog/p3.md --
---
title: Cool Page3
---

{{< myshortcode2 >}}
-- content/blog/p4.md --
---
title: Cool Page With Image
---

Image:

![Drag Racing](/images/Dragster.jpg "image title")
-- content/blog/p5.md --
---
title: Cool Page With Markdownify
---

{{< myshortcode4 >}}
Inner Link: [Inner Link](https://www.google.com "Google's Homepage")
{{< /myshortcode4 >}}
-- content/blog/p6.md --
---
title: With RenderString
---

{{< myshortcode5 >}}Inner Link: [Inner Link](https://www.gohugo.io "Hugo's Homepage"){{< /myshortcode5 >}}
-- content/blog/p7.md --
---
title: With Headings
---

# Heading Level 1
some text

## Heading Level 2

### Heading Level 3
-- content/customview/p1.md --
---
title: Custom View
---

{{< myshortcode6 >}}
-- content/docs/docs1.md --
---
title: Docs 1
---
[Docs 1](https://www.google.com "Google's Homepage")
-- content/docs/p8.md --
---
title: Doc With Heading
---
# Docs lvl 1
-- data/hugo.toml --
slogan = "Hugo Rocks!"
-- layouts/_default/_markup/render-heading.html --
HEADING: {{ .Page.Title }}||Level: {{ .Level }}|Anchor: {{ .Anchor | safeURL }}|Text: {{ .Text | safeHTML }}|Attributes: {{ .Attributes }}|END
-- layouts/_default/_markup/render-image.html --
IMAGE: {{ .Page.Title }}||{{ .Destination | safeURL }}|Title: {{ .Title | safeHTML }}|Text: {{ .Text | safeHTML }}|END
-- layouts/_default/_markup/render-link.html --
{{ with .Page }}{{ .Title }}{{ end }}|{{ .Destination | safeURL }}|Title: {{ .Title | safeHTML }}|Text: {{ .Text | safeHTML }}|END
-- layouts/_default/single.html --
{{ .Content }}
-- layouts/customview/myrender.html --
myrender: {{ .Title }}|P4: {{ partial "mypartial4" }}
-- layouts/docs/_markup/render-heading.html --
Docs Level: {{ .Level }}|END
-- layouts/docs/_markup/render-link.html --
Link docs section: {{ .Text | safeHTML }}|END
-- layouts/partials/mypartial1.html --
PARTIAL1
-- layouts/partials/mypartial2.html --
PARTIAL2  {{ partial "mypartial3.html" }}
-- layouts/partials/mypartial3.html --
PARTIAL3
-- layouts/partials/mypartial4.html --
PARTIAL4
-- layouts/robots.txt --
robots|{{ .Lang }}|{{ .Title }}
-- layouts/shortcodes/lingo.fr.html --
LingoFrench
-- layouts/shortcodes/lingo.html --
LingoDefault
-- layouts/shortcodes/myshortcode1.html --
{{ partial "mypartial1" }}
-- layouts/shortcodes/myshortcode2.html --
{{ partial "mypartial2" }}
-- layouts/shortcodes/myshortcode3.html --
SHORT3|
-- layouts/shortcodes/myshortcode4.html --
<div class="foo">
{{ .Inner | markdownify }}
</div>
-- layouts/shortcodes/myshortcode5.html --
Inner Inline: {{ .Inner | .Page.RenderString }}
Inner Block: {{ .Inner | .Page.RenderString (dict "display" "block" ) }}
-- layouts/shortcodes/myshortcode6.html --
.Render: {{ .Page.Render "myrender" }}

	`

	c := qt.New(t)

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
			WorkingDir:  "/mywork",
			Running:     true,
		},
	).Build()

	b.AssertRenderCountContent(13)

	b.AssertFileContent("public/blog/p1/index.html", `
Cool Page|https://www.google.com|Title: Google's Homepage|Text: First Link|END
Cool Page|https://foo.bar/|Title: |Text: https://foo.bar/|END
Cool Page|https://bar.baz/|Title: |Text: https://bar.baz/|END
Cool Page|mailto:fake@example.com|Title: |Text: fake@example.com|END
Cool Page|mailto:fake2@example.com|Title: |Text: mailto:fake2@example.com|END
Text: Second
SHORT3|
<p>IMAGE: Cool Page||/images/Dragster.jpg|Title: image title|Text: Drag Racing|END</p>
`)

	b.AssertFileContent("public/customview/p1/index.html", `.Render: myrender: Custom View|P4: PARTIAL4`)
	b.AssertFileContent("public/blog/p2/index.html",
		`PARTIAL
,Cool Page2|https://gohugo.io|Title: |Text: No Whitespace Please|END,`,
	)
	b.AssertFileContent("public/blog/p3/index.html", `PARTIAL3`)
	// We may add type template support later, keep this for then. b.AssertFileContent("public/docs/docs1/index.html", `Link docs section: Docs 1|END`)
	b.AssertFileContent("public/blog/p4/index.html", `<p>IMAGE: Cool Page With Image||/images/Dragster.jpg|Title: image title|Text: Drag Racing|END</p>`)
	// markdownify
	b.AssertFileContent("public/blog/p5/index.html", "Inner Link: |https://www.google.com|Title: Google's Homepage|Text: Inner Link|END")

	b.AssertFileContent("public/blog/p6/index.html",
		"Inner Inline: Inner Link: With RenderString|https://www.gohugo.io|Title: Hugo's Homepage|Text: Inner Link|END",
		"Inner Block: <p>Inner Link: With RenderString|https://www.gohugo.io|Title: Hugo's Homepage|Text: Inner Link|END</p>",
	)

	b.EditFiles(
		"layouts/_default/_markup/render-link.html", `EDITED: {{ .Destination | safeURL }}|`,
		"layouts/_default/_markup/render-image.html", `IMAGE EDITED: {{ .Destination | safeURL }}|`,
		"layouts/docs/_markup/render-link.html", `DOCS EDITED: {{ .Destination | safeURL }}|`,
		"layouts/partials/mypartial1.html", `PARTIAL1_EDITED`,
		"layouts/partials/mypartial3.html", `PARTIAL3_EDITED`,
		"layouts/partials/mypartial4.html", `PARTIAL4_EDITED`,
		"layouts/shortcodes/myshortcode3.html", `SHORT3_EDITED|`,
	).Build()

	// Make sure that only content using the changed templates are re-rendered.
	// TODO1	b.AssertRenderCountContent(7)

	b.AssertFileContent("public/customview/p1/index.html", `.Render: myrender: Custom View|P4: PARTIAL4_EDITED`)
	b.AssertFileContent("public/blog/p1/index.html", `<p>EDITED: https://www.google.com|</p>`, "SHORT3_EDITED|")
	b.AssertFileContent("public/blog/p2/index.html", `PARTIAL1_EDITED`)
	b.AssertFileContent("public/blog/p3/index.html", `PARTIAL3_EDITED`)
	// We may add type template support later, keep this for then. b.AssertFileContent("public/docs/docs1/index.html", `DOCS EDITED: https://www.google.com|</p>`)
	b.AssertFileContent("public/blog/p6/index.html", "<p>Inner Link: EDITED: https://www.gohugo.io|</p>")
	b.AssertFileContent("public/blog/p4/index.html", `IMAGE EDITED: /images/Dragster.jpg|`)
	b.AssertFileContent("public/blog/p7/index.html", "HEADING: With Headings||Level: 1|Anchor: heading-level-1|Text: Heading Level 1|Attributes: map[id:heading-level-1]|END<p>some text</p>\nHEADING: With Headings||Level: 2|Anchor: heading-level-2|Text: Heading Level 2|Attributes: map[id:heading-level-2]|ENDHEADING: With Headings||Level: 3|Anchor: heading-level-3|Text: Heading Level 3|Attributes: map[id:heading-level-3]|END")

	// https://github.com/gohugoio/hugo/issues/7349
	b.AssertFileContent("public/docs/p8/index.html", "Docs Level: 1")
}

func TestRenderHooksDeleteTemplate(t *testing.T) {
	config := `
baseURL="https://example.org"
workingDir="/mywork"
`
	b := newTestSitesBuilder(t).WithWorkingDir("/mywork").WithConfigFile("toml", config).Running()
	b.WithTemplatesAdded("_default/single.html", `{{ .Content }}`)
	b.WithTemplatesAdded("_default/_markup/render-link.html", `html-render-link`)

	b.WithContent("p1.md", `---
title: P1
---
[First Link](https://www.google.com "Google's Homepage")

`)
	b.Build(BuildCfg{})

	b.AssertFileContent("public/p1/index.html", `<p>html-render-link</p>`)

	b.RemoveFiles(
		"layouts/_default/_markup/render-link.html",
	)

	b.Build(BuildCfg{})
	b.AssertFileContent("public/p1/index.html", `<p><a href="https://www.google.com" title="Google's Homepage">First Link</a></p>`)
}

func TestRenderHookAddTemplate(t *testing.T) {
	c := qt.New(t)

	files := `
-- config.toml --
baseURL="https://example.org"
workingDir="/mywork"
-- content/p1.md --
[First Link](https://www.google.com "Google's Homepage")
-- content/p2.md --
No link.
-- layouts/_default/single.html --
{{ .Content }}

	`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			WorkingDir:  "/mywork",
			TxtarString: files,
			Running:     true,
		}).Build()

	b.AssertFileContent("public/p1/index.html", `<p><a href="https://www.google.com" title="Google's Homepage">First Link</a></p>`)
	b.AssertRenderCountContent(2)

	b.EditFiles("layouts/_default/_markup/render-link.html", `html-render-link`).Build()

	b.AssertFileContent("public/p1/index.html", `<p>html-render-link</p>`)
	b.AssertRenderCountContent(1)
}

func TestRenderHooksRSS(t *testing.T) {
	b := newTestSitesBuilder(t)

	b.WithTemplates("index.html", `
{{ $p := site.GetPage "p1.md" }}
{{ $p2 := site.GetPage "p2.md" }}

P1: {{ $p.Content }}
P2: {{ $p2.Content }}
	
	`, "index.xml", `

{{ $p2 := site.GetPage "p2.md" }}
{{ $p3 := site.GetPage "p3.md" }}

P2: {{ $p2.Content }}
P3: {{ $p3.Content }}

	
	`,
		"_default/_markup/render-link.html", `html-link: {{ .Destination | safeURL }}|`,
		"_default/_markup/render-link.rss.xml", `xml-link: {{ .Destination | safeURL }}|`,
		"_default/_markup/render-heading.html", `html-heading: {{ .Text }}|`,
		"_default/_markup/render-heading.rss.xml", `xml-heading: {{ .Text }}|`,
	)

	b.WithContent("p1.md", `---
title: "p1"
---
P1. [I'm an inline-style link](https://www.gohugo.io)

# Heading in p1

`, "p2.md", `---
title: "p2"
---
P1. [I'm an inline-style link](https://www.bep.is)

# Heading in p2

`,
		"p3.md", `---
title: "p2"
outputs: ["rss"]
---
P3. [I'm an inline-style link](https://www.example.org)

`,
	)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/index.html", `
P1: <p>P1. html-link: https://www.gohugo.io|</p>
html-heading: Heading in p1|
html-heading: Heading in p2|
`)
	b.AssertFileContent("public/index.xml", `
P2: <p>P1. xml-link: https://www.bep.is|</p>
P3: <p>P3. xml-link: https://www.example.org|</p>
xml-heading: Heading in p2|
`)
}

// https://github.com/gohugoio/hugo/issues/6629
func TestRenderLinkWithMarkupInText(t *testing.T) {
	b := newTestSitesBuilder(t)
	b.WithConfigFile("toml", `

baseURL="https://example.org"

[markup]
  [markup.goldmark]
    [markup.goldmark.renderer]
      unsafe = true
    
`)

	b.WithTemplates("index.html", `
{{ $p := site.GetPage "p1.md" }}
P1: {{ $p.Content }}

	`,
		"_default/_markup/render-link.html", `html-link: {{ .Destination | safeURL }}|Text: {{ .Text | safeHTML }}|Plain: {{ .PlainText | safeHTML }}`,
		"_default/_markup/render-image.html", `html-image: {{ .Destination | safeURL }}|Text: {{ .Text | safeHTML }}|Plain: {{ .PlainText | safeHTML }}`,
	)

	b.WithContent("p1.md", `---
title: "p1"
---

START: [**should be bold**](https://gohugo.io)END

Some regular **markup**.

Image:

![Hello<br> Goodbye](image.jpg)END

`)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/index.html", `
  P1: <p>START: html-link: https://gohugo.io|Text: <strong>should be bold</strong>|Plain: should be boldEND</p>
<p>Some regular <strong>markup</strong>.</p>
<p>html-image: image.jpg|Text: Hello<br> Goodbye|Plain: Hello GoodbyeEND</p>
`)
}
