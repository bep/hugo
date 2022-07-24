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

package paths

import (
	"path/filepath"
	"testing"

	"github.com/gohugoio/hugo/htesting"

	qt "github.com/frankban/quicktest"
)

func TestParse(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		name   string
		path   string
		assert func(c *qt.C, p *Path)
	}{
		{
			"Basic text file",
			"/a/b.txt",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b.txt")
				c.Assert(p.Base(), qt.Equals, "/a/b.txt")
				c.Assert(p.Dir(), qt.Equals, "/a")
				c.Assert(p.Ext(), qt.Equals, "txt")
			},
		},
		{
			"Basic text file, upper case",
			"/A/B.txt",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b.txt")
				c.Assert(p.NameNoExt(), qt.Equals, "b")
				c.Assert(p.NameNoIdentifier(), qt.Equals, "b")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "b")
				c.Assert(p.Base(), qt.Equals, "/a/b.txt")
				c.Assert(p.Ext(), qt.Equals, "txt")
			},
		},
		{
			"Basic Markdown file",
			"/a/b/c.md",
			func(c *qt.C, p *Path) {
				c.Assert(p.IsContent(), qt.IsTrue)
				c.Assert(p.IsLeafBundle(), qt.IsFalse)
				c.Assert(p.Name(), qt.Equals, "c.md")
				c.Assert(p.Base(), qt.Equals, "/a/b/c")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "c")
				c.Assert(p.Path(), qt.Equals, "/a/b/c.md")
				c.Assert(p.Dir(), qt.Equals, "/a/b")
				c.Assert(p.Container(), qt.Equals, "b")
				c.Assert(p.ContainerDir(), qt.Equals, "/a/b")
				c.Assert(p.Ext(), qt.Equals, "md")
			},
		},
		{
			"Content resource",
			"/a/b.md",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b.md")
				c.Assert(p.Base(), qt.Equals, "/a/b")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "b")

				// Reclassify it as a content resource.
				ModifyPathBundleTypeResource(p)
				c.Assert(p.BundleType(), qt.Equals, PathTypeContentResource)
				c.Assert(p.IsContent(), qt.IsTrue)
				c.Assert(p.Name(), qt.Equals, "b.md")
				c.Assert(p.Base(), qt.Equals, "/a/b.md")
			},
		},
		{
			"No ext",
			"/a/b",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b")
				c.Assert(p.NameNoExt(), qt.Equals, "b")
				c.Assert(p.Base(), qt.Equals, "/a/b")
				c.Assert(p.Ext(), qt.Equals, "")
			},
		},
		{
			"No ext, trailing slash",
			"/a/b/",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b")
				c.Assert(p.Base(), qt.Equals, "/a/b")
				c.Assert(p.Ext(), qt.Equals, "")
			},
		},
		{
			"Identifiers",
			"/a/b.a.b.c.txt",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "b.a.b.c.txt")
				c.Assert(p.NameNoIdentifier(), qt.Equals, "b")
				c.Assert(p.NameNoLang(), qt.Equals, "b.a.b.txt")
				c.Assert(p.Identifiers(), qt.DeepEquals, []string{"txt", "c", "b", "a"})
				c.Assert(p.Base(), qt.Equals, "/a/b.txt")
				c.Assert(p.Ext(), qt.Equals, "txt")
			},
		},
		{
			"Index content file",
			"/a/index.md",
			func(c *qt.C, p *Path) {
				c.Assert(p.Base(), qt.Equals, "/a")
				c.Assert(p.Dir(), qt.Equals, "/a")
				c.Assert(p.Ext(), qt.Equals, "md")
				c.Assert(p.Container(), qt.Equals, "a")
				c.Assert(p.Section(), qt.Equals, "a")
				c.Assert(p.NameNoExt(), qt.Equals, "index")
				c.Assert(p.NameNoLang(), qt.Equals, "index.md")
				c.Assert(p.NameNoIdentifier(), qt.Equals, "index")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "a")
				c.Assert(p.Identifiers(), qt.DeepEquals, []string{"md"})
				c.Assert(p.IsLeafBundle(), qt.IsTrue)
				c.Assert(p.IsBundle(), qt.IsTrue)
				c.Assert(p.IsBranchBundle(), qt.IsFalse)
			},
		},
		{
			"Index content file with lang",
			"/a/b/index.no.md",
			func(c *qt.C, p *Path) {
				c.Assert(p.Base(), qt.Equals, "/a/b")
				c.Assert(p.Dir(), qt.Equals, "/a/b")
				c.Assert(p.Ext(), qt.Equals, "md")
				c.Assert(p.Container(), qt.Equals, "b")
				c.Assert(p.ContainerDir(), qt.Equals, "/a")
				c.Assert(p.Section(), qt.Equals, "a")
				c.Assert(p.NameNoExt(), qt.Equals, "index.no")
				c.Assert(p.NameNoLang(), qt.Equals, "index.md")
				c.Assert(p.NameNoIdentifier(), qt.Equals, "index")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "b")
				c.Assert(p.Identifiers(), qt.DeepEquals, []string{"md", "no"})
				c.Assert(p.IsLeafBundle(), qt.IsTrue)
				c.Assert(p.IsBundle(), qt.IsTrue)
				c.Assert(p.IsBranchBundle(), qt.IsFalse)
			},
		},
		{
			"Index branch content file",
			"/a/b/_index.no.md",
			func(c *qt.C, p *Path) {
				c.Assert(p.Base(), qt.Equals, "/a/b")
				c.Assert(p.Container(), qt.Equals, "b")
				c.Assert(p.NameNoExt(), qt.Equals, "_index.no")
				c.Assert(p.NameNoLang(), qt.Equals, "_index.md")
				c.Assert(p.BaseNameNoIdentifier(), qt.Equals, "b")
				c.Assert(p.Ext(), qt.Equals, "md")
				c.Assert(p.Identifiers(), qt.DeepEquals, []string{"md", "no"})
				c.Assert(p.IsBranchBundle(), qt.IsTrue)
				c.Assert(p.IsLeafBundle(), qt.IsFalse)
				c.Assert(p.IsBundle(), qt.IsTrue)
			},
		},
		{
			"Index text file",
			"/a/b/index.no.txt",
			func(c *qt.C, p *Path) {
				c.Assert(p.Base(), qt.Equals, "/a/b/index.txt")
				c.Assert(p.Ext(), qt.Equals, "txt")
				c.Assert(p.IsLeafBundle(), qt.IsFalse)
				c.Assert(p.Identifiers(), qt.DeepEquals, []string{"txt", "no"})
			},
		},

		{
			"Empty",
			"",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "")
				c.Assert(p.Base(), qt.Equals, "/")
				c.Assert(p.Ext(), qt.Equals, "")
			},
		},
		{
			"Slash",
			"/",
			func(c *qt.C, p *Path) {
				c.Assert(p.Name(), qt.Equals, "")
				c.Assert(p.Base(), qt.Equals, "/")
				c.Assert(p.Ext(), qt.Equals, "")
			},
		},
	}
	for _, test := range tests {
		c.Run(test.name, func(c *qt.C) {
			if test.name != "Basic Markdown file" {
				//c.Skip()
			}
			test.assert(c, Parse(test.path))
		})
	}

	// Errors
	c.Run("File separator", func(c *qt.C) {
		if !htesting.IsWindows() {
			c.Skip()
		}
		_, err := parse(filepath.FromSlash("/a/b/c"))
		c.Assert(err, qt.IsNotNil)
	})
}

func TestHasExt(t *testing.T) {
	c := qt.New(t)

	c.Assert(HasExt("/a/b/c.txt"), qt.IsTrue)
	c.Assert(HasExt("/a/b.c/d.txt"), qt.IsTrue)
	c.Assert(HasExt("/a/b/c"), qt.IsFalse)
	c.Assert(HasExt("/a/b.c/d"), qt.IsFalse)
}
