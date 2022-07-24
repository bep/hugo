// Copyright 2018 The Hugo Authors. All rights reserved.
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

package helpers

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/hugofs"

	"github.com/gohugoio/hugo/langs"
)

func TestNewPathSpecFromConfig(t *testing.T) {
	c := qt.New(t)
	for _, baseURLWithPath := range []bool{false, true} {
		for _, baseURLWithTrailingSlash := range []bool{false, true} {
			c.Run(fmt.Sprintf("baseURLWithPath=%T-baseURLWithTrailingSlash=%T", baseURLWithPath, baseURLWithTrailingSlash), func(c *qt.C) {
				baseURL := "http://base.com"
				if baseURLWithPath {
					baseURL += "/foo"
				}

				if baseURLWithTrailingSlash {
					baseURL += "/"
				}

				v := newTestCfg()
				l := langs.NewLanguage("no", v)
				v.Set("disablePathToLower", true)
				v.Set("removePathAccents", true)
				v.Set("uglyURLs", true)
				v.Set("canonifyURLs", true)
				v.Set("paginatePath", "side")
				v.Set("baseURL", baseURL)
				v.Set("themesDir", "thethemes")
				v.Set("layoutDir", "thelayouts")
				v.Set("workingDir", "thework")
				v.Set("staticDir", "thestatic")
				v.Set("theme", "thetheme")
				langs.LoadLanguageSettings(v, nil)

				fs := hugofs.NewMem(v)
				fs.Source.MkdirAll(filepath.FromSlash("thework/thethemes/thetheme"), 0777)

				p, err := NewPathSpec(fs, l, nil)

				c.Assert(err, qt.IsNil)
				c.Assert(p.CanonifyURLs, qt.Equals, true)
				c.Assert(p.DisablePathToLower, qt.Equals, true)
				c.Assert(p.RemovePathAccents, qt.Equals, true)
				c.Assert(p.UglyURLs, qt.Equals, true)
				c.Assert(p.Language.Lang, qt.Equals, "no")
				c.Assert(p.PaginatePath, qt.Equals, "side")

				c.Assert(p.BaseURL.String(), qt.Equals, baseURL)
				c.Assert(p.BaseURLStringOrig, qt.Equals, baseURL)
				baseURLNoTrailingSlash := strings.TrimSuffix(baseURL, "/")
				c.Assert(p.BaseURLString, qt.Equals, baseURLNoTrailingSlash)
				c.Assert(p.BaseURLNoPathString, qt.Equals, strings.TrimSuffix(baseURLNoTrailingSlash, "/foo"))

				c.Assert(p.ThemesDir, qt.Equals, "thethemes")
				c.Assert(p.WorkingDir, qt.Equals, "thework")

			})

		}

	}
}
