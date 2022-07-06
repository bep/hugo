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
	"math/rand"
	"testing"

	qt "github.com/frankban/quicktest"
)

// bookmarkS2 2
func TestSmoke(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	files := `
	
-- config.toml --
title = "Hello World"
baseURL = "https://example.com"
defaultContentLanguage = "en"
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
title: Bundle 1
tags: ["a", "b", "c"]
---
-- content/s1/mybundle/index.nn.md --
---
title: Bundle 1 NN
tags: ["a", "b", "c"]
---
-- content/s1/mybundle/hugo.txt --
Hugo Rocks!
-- content/s1/mybundle/nynorskonly.nn.txt --
Nynorsk Rocks!
-- content/s1/foo/bar/p1.md --
---
title: Page S1 1
tags: ["a", "d"]
---

## Hooks

My favorite search engine is [Duck Duck Go](https://duckduckgo.com).

![The San Juan Mountains are beautiful!](/assets/images/san-juan-mountains.jpg "San Juan Mountains")

§§§foo
echo "foo";
§§§
-- content/s1/foo/bar/p1.nn.md --
---
title: Page S1 1 NN
---
-- content/s2/_index.md --
---
title: "Section # 2"
cascade:
- _target:
  background: yosemite.jpg
  color: #fff
---
-- content/s2/_index.nn.md --
---
title: "Section # 2 NN"
---
-- content/s2/p1.md --
---
title: Page S2 1
---
-- content/s2/p2.md --
---
title: Page S2 2
---
-- content/s2/s3/_index.md --
---
title: "Section # 3"
cascade:
- _target:
  foo: bar.jpg
---
-- content/s2/s3/p1.md --
---
title: Page S3 1
---
-- content/s2/s3/foo/p2.md --
---
title: Page S3 2
date: "2022-05-06"
---
-- content/s2/s4.md --
---
title: Page S2 S4
---
-- content/s2/s3/s4/_index.md --
---
title: "Section # 4"
cascade:
- _target:
  foo: section4.jpg
  background: section4.jpg
---
-- content/s2/s3/s4/p1.md --
---
title: "Section 4 P1"
---
-- layouts/_default/_markup/render-link.html --
Render Link: {{ .Destination | safeHTML }}
-- layouts/_default/_markup/render-image.html --
Render Image: {{ .Destination | safeHTML }}
-- layouts/_default/_markup/render-heading.html --
Render Heading: {{ .PlainText }}
-- layouts/_default/_markup/render-codeblock-foo.html --
Codeblock: {{ .Type }}
-- layouts/index.nn.html --
Nynorsk:
{{ $s1 := site.GetPage "s1" }}
{{ $p1 := site.GetPage "s1/foo/bar/p1" }}
{{ $s2 := site.GetPage "s2" }}
{{ $mybundle := site.GetPage "s1/mybundle" }}
P1: {{ template "print-info" $p1 }}
S1: {{ template "print-info" $s1 }}
S2: {{ template "print-info" $s2 }}
Mybundle: {{ template "print-info" $mybundle }}
Pages: {{ len site.Pages }}|
RegularPages: {{ len site.RegularPages }}|
-- layouts/index.html --
English:
{{ $home := site.GetPage "/" }}
{{ $p1 := site.GetPage "s1/foo/bar/p1" }}
{{ $s1 := site.GetPage "s1" }}
{{ $s2 := site.GetPage "s2" }}
{{ $s3 := site.GetPage "s2/s3" }}
{{ $foo2 := site.GetPage "s2/s3/foo/p2" }}
{{ $mybundle := site.GetPage "s1/mybundle" }}
{{ $mybundleTags := $mybundle.GetTerms "tags" }}
{{ $s2_p1 := site.GetPage "s2/p1" }}
{{ $s2_s3_p1 := site.GetPage "s2/s3/p1" }}
{{ $s2_s3_s4_p1 := site.GetPage "s2/s3/s4/p1" }}
{{ $tags := site.GetPage "tags" }}
{{ $taga := site.GetPage "tags/a" }}


Home: {{ template "print-info" . }}
P1: {{ template "print-info" $p1 }}
S1: {{ template "print-info" $s1 }}
S2: {{ template "print-info" $s2 }}
S3: {{ template "print-info" $s3 }}
TAGS: {{ template "print-info" $tags }}|
TAGA: {{ template "print-info" $taga }}|
MyBundle Tags: {{ template "list-pages" $mybundleTags }}
S3 IsAncestor S2: {{ $s3.IsAncestor $s2 }}
S2 IsAncestor S3: {{ $s2.IsAncestor $s3 }}
S3 IsDescendant S2: {{ $s3.IsDescendant $s2 }}
S2 IsDescendant S3: {{ $s2.IsDescendant $s3 }}
P1 CurrentSection: {{ $p1.CurrentSection }}
S1 CurrentSection: {{ $s1.CurrentSection }}
FOO2 FirstSection: {{ $foo2.FirstSection }}
S1 FirstSection: {{ $s1.FirstSection }}
Home FirstSection: {{ $home.FirstSection }}
InSection S1 P1: {{ $p1.InSection $s1 }}
InSection S1 S2: {{ $s1.InSection $s2 }}
Parent S1: {{ $s1.Parent }}|
Parent S2: {{ $s2.Parent }}|
Parent S3: {{ $s3.Parent }}|
Parent P1: {{ $p1.Parent }}|
Parent Home: {{ $home.Parent }}|
S2 RegularPages: {{ template "list-pages" $s2.RegularPages }}
S2 RegularPagesRecursive: {{ template "list-pages" $s2.RegularPagesRecursive }}
Site RegularPages: {{ template "list-pages" site.RegularPages }}
Site Pages: {{ template "list-pages" site.Pages }}
P1 Content: {{ $p1.Content }}
S2 Date: {{ $s2.Date.Format "2006-01-02" }}
Home Date: {{ $home.Date.Format "2006-01-02" }}
Site LastMod: {{ site.LastChange.Format "2006-01-02" }}
Pages: {{ len site.Pages }}|
RegularPages: {{ len site.RegularPages }}|
AllPages: {{ len site.AllPages }}|
AllRegularPages: {{ len site.AllRegularPages }}|
Mybundle: {{ template "print-info" $mybundle }}
Cascade S2: {{ $s2_p1.Params }}|
Cascade S3: {{ $s2_s3_p1.Params }}|
Cascade S3: {{ $s2_s3_s4_p1.Params }}|
{{ define "print-info" }}{{ with . }}{{ .Kind }}|{{ .Lang }}|{{ .Path }}|{{ .Title }}|Sections: {{ template "list-pages" .Sections }}|Pages: {{ template "list-pages" .Pages }}|Resources: {{ len .Resources }}{{ end }}{{ end }}
{{ define "list-pages" }}{{ len . }}:[{{ range $i, $e := . }}{{ if $i }}, {{ end }}"{{ .Path }}|{{ .Title }}"{{ end }}]{{ end }}
`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
		}).Build()

	b.AssertFileContent("public/index.html", `
S2 Date: 2022-05-06
Home Date: 2022-05-06
Site LastMod: 2022-05-06
S3 IsDescendant S2: true
S2 IsDescendant S3: false
P1 CurrentSection: Page(/s1)
S1 CurrentSection: Page(/s1)
FOO2 FirstSection: Page(/s2)
S2: section|en|/s2|Section # 2|Sections: 1:["/s2/s3|Section # 3"]|Pages: 4:["/s2/s3|Section # 3", "/s2/p1|Page S2 1", "/s2/p2|Page S2 2", "/s2/s4|Page S2 S4"]|Resources: 0
S2 RegularPages: 3:["/s2/p1|Page S2 1", "/s2/p2|Page S2 2", "/s2/s4|Page S2 S4"]
S2 RegularPagesRecursive: 6:["/s2/s3/foo/p2|Page S3 2", "/s2/p1|Page S2 1", "/s2/p2|Page S2 2", "/s2/s4|Page S2 S4", "/s2/s3/p1|Page S3 1", "/s2/s3/s4/p1|Section 4 P1"]
Site RegularPages: 8:["/s2/s3/foo/p2|Page S3 2", "/s1/mybundle|Bundle 1", "/s1/foo/bar/p1|Page S1 1", "/s2/p1|Page S2 1", "/s2/p2|Page S2 2", "/s2/s4|Page S2 S4", "/s2/s3/p1|Page S3 1", "/s2/s3/s4/p1|Section 4 P1"]
Site Pages: 19:["/s2/s3/foo/p2|Page S3 2", "/s2|Section # 2", "/s2/s3|Section # 3", "/|Title in English", "/tags/a|A", "/tags/b|B", "/s1/mybundle|Bundle 1", "/tags/c|C", "/categories|Categories", "/tags/d|D", "/s1/foo/bar/p1|Page S1 1", "/s2/p1|Page S2 1", "/s2/p2|Page S2 2", "/s2/s4|Page S2 S4", "/s2/s3/p1|Page S3 1", "/s1|S1s", "/s2/s3/s4|Section # 4", "/s2/s3/s4/p1|Section 4 P1", "/tags|Tags"]
Mybundle: page|en|/s1/mybundle|Bundle 1|Sections: 0:[]|Pages: 0:[]|Resources: 2
Pages: 19|
RegularPages: 8|
AllPages: 29|
AllRegularPages: 10|
Cascade S2: map[_target:&lt;nil&gt; background:yosemite.jpg color:&lt;nil&gt; draft:false iscjklanguage:false title:Page S2 1]|
Cascade S3: map[_target:&lt;nil&gt; background:yosemite.jpg color:&lt;nil&gt; draft:false foo:bar.jpg iscjklanguage:false title:Page S3 1]|
Cascade S3: map[_target:&lt;nil&gt; background:section4.jpg color:&lt;nil&gt; draft:false foo:section4.jpg iscjklanguage:false title:Section 4 P1]|

	`)

	content := b.FileContent("public/nn/index.html")
	fmt.Println(string(content))

	b.AssertFileContent("public/nn/index.html", `
P1: page|nn|/s1/foo/bar/p1|Page S1 1 NN|Sections: 0:[]|Pages: 0:[]|Resources: 0
S1: section|nn|/s1|S1s|Sections: 0:[]|Pages: 2:["/s1/mybundle|Bundle 1 NN", "/s1/foo/bar/p1|Page S1 1 NN"]|Resources: 0
S2: section|nn|/s2|Section # 2 NN|Sections: 0:[]|Pages: 0:[]|Resources: 0
Mybundle: page|nn|/s1/mybundle|Bundle 1 NN|Sections: 0:[]|Pages: 0:[]|Resources: 2
Pages: 10|
RegularPages: 2|

		
	`)

	// Assert taxononomies.
	b.AssertFileContent("public/index.html", `
TAGS: taxonomy|en|/tags|Tags|Sections: 0:[]|Pages: 4:["/tags/a|A", "/tags/b|B", "/tags/c|C", "/tags/d|D"]|Resources: 0|
TAGA: term|en|/tags/a|A|Sections: 0:[]|Pages: 2:["/s1/mybundle|Bundle 1", "/s1/foo/bar/p1|Page S1 1"]|Resources: 0|
MyBundle Tags: 3:["/tags/a|A", "/tags/b|B", "/tags/c|C"]
`)

}

func TestSmokeTranslations(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	files := `
	
-- config.toml --
title = "Hello World"
baseURL = "https://example.com"
defaultContentLanguage = "en"
[languages]
[languages.en]
title = "Title in English"
languageName = "English"
weight = 1
[languages.nn]
languageName = "Nynorsk"
weight = 2
title = "Tittel på nynorsk"
[languages.sv]
languageName = "Svenska"
weight = 3
title = "Tittel på svenska"
-- content/s1/p1.md --
---
title: P1 EN
---
-- content/s1/p1.nn.md --
---
title: P1 NN
---
-- content/s1/p1.sv.md --
---
title: P1 SV
---
-- layouts/index.html --
{{ $p1 := .Site.GetPage "s1/p1" }}

Translations: {{ len $p1.Translations }}
All Translations: {{ len $p1.AllTranslations }}


`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
		}).Build()

	b.AssertFileContent("public/index.html", `
	Translations: 2
	All Translations: 3

	`)

}

// This is just a test to verify that BenchmarkBaseline is working as intended.
func TestBenchmarkBaseline(t *testing.T) {
	cfg := IntegrationTestConfig{
		T:           t,
		TxtarString: benchmarkBaselineFiles(),
	}
	b := NewIntegrationTestBuilder(cfg).Build()

	b.Assert(len(b.H.Sites), qt.Equals, 4)
	b.Assert(len(b.H.Sites[0].RegularPages()), qt.Equals, 161)
	//b.Assert(len(b.H.Sites[0].Pages()), qt.Equals, 197) // TODO1
	b.Assert(len(b.H.Sites[2].RegularPages()), qt.Equals, 158)
	//b.Assert(len(b.H.Sites[2].Pages()), qt.Equals, 194)

}

func BenchmarkBaseline(b *testing.B) {
	cfg := IntegrationTestConfig{
		T:           b,
		TxtarString: benchmarkBaselineFiles(),
	}
	builders := make([]*IntegrationTestBuilder, b.N)

	for i := range builders {
		builders[i] = NewIntegrationTestBuilder(cfg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builders[i].Build()
	}
}

func benchmarkBaselineFiles() string {

	rnd := rand.New(rand.NewSource(32))

	files := `
-- config.toml --
baseURL = "https://example.com"
defaultContentLanguage = 'en'

[module]
[[module.mounts]]
source = 'content/en'
target = 'content/en'
lang = 'en'
[[module.mounts]]
source = 'content/nn'
target = 'content/nn'
lang = 'nn'
[[module.mounts]]
source = 'content/no'
target = 'content/no'
lang = 'no'
[[module.mounts]]
source = 'content/sv'
target = 'content/sv'
lang = 'sv'
[[module.mounts]]
source = 'layouts'
target = 'layouts'

[languages]
[languages.en]
title = "English"
weight = 1
[languages.nn]
title = "Nynorsk"
weight = 2
[languages.no]
title = "Norsk"
weight = 3
[languages.sv]
title = "Svenska"
weight = 4
-- layouts/_default/list.html --
{{ .Title }}
{{ .Content }}
-- layouts/_default/single.html --
{{ .Title }}
{{ .Content }}
-- layouts/shortcodes/myshort.html --
{{ .Inner }}
`

	contentTemplate := `
---
title: "Page %d"
date: "2018-01-01"
weight: %d
---

## Heading 1

Duis nisi reprehenderit nisi cupidatat cillum aliquip ea id eu esse commodo et.

## Heading 2

Aliqua labore enim et sint anim amet excepteur ea dolore.

{{< myshort >}}
Hello, World!
{{< /myshort >}}

Aliqua labore enim et sint anim amet excepteur ea dolore.


`

	for _, lang := range []string{"en", "nn", "no", "sv"} {
		files += fmt.Sprintf("\n-- content/%s/_index.md --\n"+contentTemplate, lang, 1, 1, 1)
		for i, root := range []string{"", "foo", "bar", "baz"} {
			for j, section := range []string{"posts", "posts/funny", "posts/science", "posts/politics", "posts/world", "posts/technology", "posts/world/news", "posts/world/news/europe"} {
				n := i + j + 1
				files += fmt.Sprintf("\n-- content/%s/%s/%s/_index.md --\n"+contentTemplate, lang, root, section, n, n, n)
				for k := 1; k < rnd.Intn(30)+1; k++ {
					n := n + k
					files += fmt.Sprintf("\n-- content/%s/%s/%s/p%d.md --\n"+contentTemplate, lang, root, section, n, n, n)
				}
			}
		}
	}

	return files
}
