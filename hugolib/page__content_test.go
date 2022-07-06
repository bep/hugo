// Copyright 2022 The Hugo Authors. All rights reserved.
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
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestContentMultilingual(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	rnd := rand.New(rand.NewSource(32))

	files := `-- config.toml --
baseURL = "https://example.com"
disableKinds=["taxonomy", "term", "sitemap", "robotsTXT"]
defaultContentLanguage = "en"
defaultContentLanguageInSubdir = true
[outputs]
home = ['HTML']
page = ['HTML']
[languages]
[languages.en]
weight = 1
title = "Title in English"
contentDir = "content/en"
[languages.nn]
contentDir = "content/nn"
weight = 2
title = "Tittel på nynorsk"
-- layouts/shortcodes/myshort.html --
Shortcode
-- layouts/index.html --
HOME
-- layouts/_default/single.html --
{{ .Title }}|{{ .Content }}|
`

	for _, lang := range []string{"en", "nn"} {
		for i := 0; i < 50; i++ {
			files += fmt.Sprintf(`-- content/%s/p%d.md --
---
title: "Page %d"
---
%s
{{< myshort >}}
`, lang, i, i, strings.Repeat(fmt.Sprintf("hello %d ", i), rnd.Intn(100)))
		}

	}

	b := NewIntegrationTestBuilder(
		IntegrationTestConfig{
			T:           c,
			TxtarString: files,
		}).Build()

	b.AssertFileContent("public/en/p12/index.html", `
		hello 12
			`)

	b.AssertFileContent("public/nn/p1/index.html", `
hello 1
	`)
}
