// Copyright 2021 The Hugo Authors. All rights reserved.
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

package postcss_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	jww "github.com/spf13/jwalterweatherman"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/htesting"
<<<<<<< HEAD
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
)

const postCSSIntegrationTestFiles = `
-- assets/css/components/a.css --
/* A comment. */
/* Another comment. */
=======
	"github.com/gohugoio/hugo/hugolib"
)

func TestTransformPostCSS(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	files := `
-- assets/css/components/a.css --
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
class-in-a {
	color: blue;
}

-- assets/css/components/all.css --
@import "a.css";
@import "b.css";
-- assets/css/components/b.css --
@import "a.css";

class-in-b {
	color: blue;
}

-- assets/css/styles.css --
@tailwind base;
@tailwind components;
@tailwind utilities;
<<<<<<< HEAD
  @import "components/all.css";
=======
@import "components/all.css";
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
h1 {
	@apply text-2xl font-bold;
}

-- config.toml --
disablekinds = ['taxonomy', 'term', 'page']
<<<<<<< HEAD
baseURL = "https://example.com"
[build]
useResourceCacheWhen = 'never'
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
-- content/p1.md --
-- data/hugo.toml --
slogan = "Hugo Rocks!"
-- i18n/en.yaml --
hello:
   other: "Hello"
-- i18n/fr.yaml --
hello:
   other: "Bonjour"
-- layouts/index.html --
{{ $options := dict "inlineImports" true }}
{{ $styles := resources.Get "css/styles.css" | resources.PostCSS $options }}
Styles RelPermalink: {{ $styles.RelPermalink }}
{{ $cssContent := $styles.Content }}
Styles Content: Len: {{ len $styles.Content }}|
-- package.json --
{
	"scripts": {},

	"devDependencies": {
	"postcss-cli": "7.1.0",
	"tailwindcss": "1.2.0"
	}
}
-- postcss.config.js --
console.error("Hugo Environment:", process.env.HUGO_ENVIRONMENT );
// https://github.com/gohugoio/hugo/issues/7656
console.error("package.json:", process.env.HUGO_FILE_PACKAGE_JSON );
console.error("PostCSS Config File:", process.env.HUGO_FILE_POSTCSS_CONFIG_JS );

module.exports = {
	plugins: [
	require('tailwindcss')
	]
}

`

<<<<<<< HEAD
func TestTransformPostCSS(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	for _, s := range []string{"never", "always"} {

		repl := strings.NewReplacer(
			"https://example.com",
			"https://example.com/foo",
			"useResourceCacheWhen = 'never'",
			fmt.Sprintf("useResourceCacheWhen = '%s'", s),
		)

		files := repl.Replace(postCSSIntegrationTestFiles)

		fmt.Println("===>", s, files)

=======
	c.Run("Success", func(c *qt.C) {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		b := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:               c,
				NeedsOsFS:       true,
				NeedsNpmInstall: true,
				LogLevel:        jww.LevelInfo,
<<<<<<< HEAD
				WorkingDir:      tempDir,
				TxtarString:     files,
			}).Build()

		b.AssertFileContent("public/index.html", `
Styles RelPermalink: /foo/css/styles.css
Styles Content: Len: 770917|
`)

	}

}

// 9880
func TestTransformPostCSSError(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	s, err := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			TxtarString:     strings.ReplaceAll(postCSSIntegrationTestFiles, "color: blue;", "@apply foo;"), // Syntax error
		}).BuildE()

	s.AssertIsFileError(err)
	c.Assert(err.Error(), qt.Contains, "a.css:4:2")

}

// #9895
func TestTransformPostCSSImportError(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	s, err := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			LogLevel:        jww.LevelInfo,
			TxtarString:     strings.ReplaceAll(postCSSIntegrationTestFiles, `@import "components/all.css";`, `@import "components/doesnotexist.css";`),
		}).BuildE()

	s.AssertIsFileError(err)
	c.Assert(err.Error(), qt.Contains, "styles.css:4:3")
	c.Assert(err.Error(), qt.Contains, filepath.FromSlash(`failed to resolve CSS @import "css/components/doesnotexist.css"`))

}

func TestTransformPostCSSImporSkipInlineImportsNotFound(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	files := strings.ReplaceAll(postCSSIntegrationTestFiles, `@import "components/all.css";`, `@import "components/doesnotexist.css";`)
	files = strings.ReplaceAll(files, `{{ $options := dict "inlineImports" true }}`, `{{ $options := dict "inlineImports" true "skipInlineImportsNotFound" true }}`)

	s := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			LogLevel:        jww.LevelInfo,
			TxtarString:     files,
		}).Build()

	s.AssertFileContent("public/css/styles.css", `@import "components/doesnotexist.css";`)

}

// Issue 9787
func TestTransformPostCSSResourceCacheWithPathInBaseURL(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	for i := 0; i < 2; i++ {
		files := postCSSIntegrationTestFiles

		if i == 1 {
			files = strings.ReplaceAll(files, "https://example.com", "https://example.com/foo")
			files = strings.ReplaceAll(files, "useResourceCacheWhen = 'never'", "	useResourceCacheWhen = 'always'")
		}

		b := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:               c,
				NeedsOsFS:       true,
				NeedsNpmInstall: true,
				LogLevel:        jww.LevelInfo,
				TxtarString:     files,
				WorkingDir:      tempDir,
			}).Build()

		b.AssertFileContent("public/index.html", `
Styles Content: Len: 770917
`)

	}

=======
				TxtarString:     files,
			}).Build()

		b.AssertLogContains("Hugo Environment: production")
		b.AssertLogContains(filepath.FromSlash(fmt.Sprintf("PostCSS Config File: %s/postcss.config.js", b.Cfg.WorkingDir)))
		b.AssertLogContains(filepath.FromSlash(fmt.Sprintf("package.json: %s/package.json", b.Cfg.WorkingDir)))

		b.AssertFileContent("public/index.html", `
Styles RelPermalink: /css/styles.css
Styles Content: Len: 770875|
`)
	})

	c.Run("Error", func(c *qt.C) {
		s, err := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:               c,
				NeedsOsFS:       true,
				NeedsNpmInstall: true,
				TxtarString:     strings.ReplaceAll(files, "color: blue;", "@apply foo;"), // Syntax error
			}).BuildE()
		s.AssertIsFileError(err)
	})
}

// bookmark2
func TestIntegrationTestTemplate(t *testing.T) {
	c := qt.New(t)

	files := ``

	b := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       false,
			NeedsNpmInstall: false,
			TxtarString:     files,
		}).Build()

	b.Assert(true, qt.IsTrue)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}
