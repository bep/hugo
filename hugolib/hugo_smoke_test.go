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
)

// The most basic build test.
func TestHello(t *testing.T) {
	t.Parallel()
	b := newTestSitesBuilder(t)
	b.WithConfigFile("toml", `
baseURL="https://example.org"
disableKinds = ["term", "taxonomy", "section", "page"]
`)
	b.WithContent("p1", `
---
title: Page
---

`)
	b.WithTemplates("index.html", `Site: {{ .Site.Language.Lang | upper }}`)

	b.Build(BuildCfg{})

	b.AssertFileContent("public/index.html", `Site: EN`)
}

func TestSmoke(t *testing.T) {
	c := qt.New(t)

	files := `
-- config.toml --
baseURL = "https://example.com"
disableKinds=["home", "taxonomy", "term", "sitemap", "robotsTXT"]
[languages]
[languages.en]
weight = 1
title = "Title in English"
[languages.nn]
weight = 2
title = "Tittel på nynorsk"

[outputs]
	section = ['HTML']
	page = ['HTML']
-- content/mysection/_index.md --
---
title: "My Section"
---
-- content/mysection/p1.md --
-- content/mysection/mysectiontext.txt --
Hello Section!
-- content/mysection/d1/mysectiontext.txt --
Hello Section1!
-- content/mysection/mybundle1/index.md --
---
title: "My Bundle1"
---
-- content/mysection/mybundle1/index.nn.md --
---
title: "My Bundle1 NN"
---
-- content/mysection/mybundle2/index.en.md --
---
title: "My Bundle2"
---
-- content/mysection/mybundle1/mybundletext.txt --
Hello Bundle!
-- content/mysection/mybundle1/mybundletext.nn.txt --
Hallo Bundle!
-- content/mysection/mybundle1/d1/mybundletext2.txt --
Hello Bundle2!
-- layouts/_default/single.html --
Single: {{ .Path }}|{{ .Lang }}|{{ .Title }}|
Resources: {{ range .Resources }}{{ .RelPermalink }}|{{ .Content }}|{{ end }}
{{ with .File }}
File1: Filename: {{ .Filename}}|Path: {{ .Path }}|Dir: {{ .Dir }}|Extension: {{ .Extension }}|Ext: {{ .Ext }}|
File2: Lang: {{ .Lang }}|LogicalName: {{ .LogicalName }}|BaseFileName: {{ .BaseFileName }}|
File3: TranslationBaseName: {{ .TranslationBaseName }}|ContentBaseName: {{ .ContentBaseName }}|Section: {{ .Section}}|UniqueID: {{ .UniqueID}}|
{{ end }}
-- layouts/_default/list.html --
List: {{ .Path }}|{{ .Title }}|
Pages: {{ range .Pages }}{{ .RelPermalink }}|{{ end }}
Resources: {{ range .Resources }}{{ .RelPermalink }}|{{ end }}
`

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			NeedsOsFS:   false,
			TxtarString: files,
		}).Build()

	b.AssertFileContent("public/mysection/mybundle1/index.html", `
Single: /mysection/mybundle1|en|My Bundle1|
Resources: /mysection/mybundle1/d1/mybundletext2.txt|Hello Bundle2!|/mysection/mybundle1/mybundletext.txt|Hello Bundle!|
`)

	b.AssertFileContent("public/nn/mysection/mybundle1/index.html", `
Single: /mysection/mybundle1|nn|My Bundle1 NN|
Resources: /nn/mysection/mybundle1/d1/mybundletext2.txt|Hello Bundle2!|/nn/mysection/mybundle1/mybundletext.txt|Hallo Bundle!|
`)

	b.AssertFileContent("public/mysection/mybundle1/index.html", `
File1: Filename: content/mysection/mybundle1/index.md|Path: mysection/mybundle1/index.md|Dir: mysection/mybundle1/|Extension: md|Ext: md|
File2: Lang: |LogicalName: index.md|BaseFileName: index|
File3: TranslationBaseName: index|ContentBaseName: mybundle1|Section: mysection|UniqueID: cfe009c850a931b15e6b90d9d1d2d08b|
`)

	b.AssertFileContent("public/mysection/mybundle2/index.html", `
Single: /mysection/mybundle2|en|My Bundle2|
File1: Filename: content/mysection/mybundle2/index.en.md|Path: mysection/mybundle2/index.en.md|Dir: mysection/mybundle2/|Extension: md|Ext: md|
File2: Lang: en|LogicalName: index.en.md|BaseFileName: index.en|
File3: TranslationBaseName: index|ContentBaseName: mybundle2|Section: mysection|UniqueID: 514f2911b671de703d891fbc58e94792|
`)

	b.AssertFileContent("public/mysection/index.html", `
List: /mysection|My Section|
Pages: /mysection/p1/|/mysection/mybundle1/|/mysection/mybundle2/|
Resources: /mysection/mysectiontext.txt|
`)
}

// https://github.com/golang/go/issues/30286
func TestDataRace(t *testing.T) {
	const page = `
---
title: "The Page"
outputs: ["HTML", "JSON"]
---	

The content.
	

	`

	b := newTestSitesBuilder(t).WithSimpleConfigFile()
	for i := 1; i <= 50; i++ {
		b.WithContent(fmt.Sprintf("blog/page%d.md", i), page)
	}

	b.WithContent("_index.md", `
---
title: "The Home"
outputs: ["HTML", "JSON", "CSV", "RSS"]
---	

The content.
	

`)

	commonTemplate := `{{ .Data.Pages }}`

	b.WithTemplatesAdded("_default/single.html", "HTML Single: "+commonTemplate)
	b.WithTemplatesAdded("_default/list.html", "HTML List: "+commonTemplate)

	b.CreateSites().Build(BuildCfg{})
}
