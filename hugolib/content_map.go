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
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gohugoio/hugo/common/hugio"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/resources"
	"github.com/gohugoio/hugo/source"

	"github.com/gohugoio/hugo/resources/page/pagekinds"
	"github.com/gohugoio/hugo/resources/resource"

	"github.com/gohugoio/hugo/hugofs"
)

// Used to mark ambiguous keys in reverse index lookups.
var ambiguousContentNode = &pageState{}

var (
	_ contentKindProvider = (*contentBundleViewInfo)(nil)
	_ viewInfoTrait       = (*contentBundleViewInfo)(nil)
)

var trimCutsetDotSlashSpace = func(r rune) bool {
	return r == '.' || r == '/' || unicode.IsSpace(r)
}

type contentBundleViewInfo struct {
	clname viewName
	term   string
}

func (c *contentBundleViewInfo) Kind() string {
	if c.term != "" {
		return pagekinds.Term
	}
	return pagekinds.Taxonomy
}

func (c *contentBundleViewInfo) Term() string {
	return c.term
}

func (c *contentBundleViewInfo) ViewInfo() *contentBundleViewInfo {
	if c == nil {
		panic("ViewInfo() called on nil")
	}
	return c
}

type contentKindProvider interface {
	Kind() string
}

type contentMapConfig struct {
	lang                 string
	taxonomyConfig       taxonomiesConfigValues
	taxonomyDisabled     bool
	taxonomyTermDisabled bool
	pageDisabled         bool
	isRebuild            bool
}

type resourceSource struct {
	path   *paths.Path
	opener resource.OpenReadSeekCloser
	fi     hugofs.FileMetaDirEntry
}

func (cfg contentMapConfig) getTaxonomyConfig(s string) (v viewName) {
	for _, n := range cfg.taxonomyConfig.views {
		if strings.HasPrefix(s, n.pluralTreeKey) {
			return n
		}
	}
	return
}

// TODO1 https://github.com/gohugoio/hugo/issues/10406 (taxo weight sort)
func (m *pageMap) AddFi(fi hugofs.FileMetaDirEntry, isBranch bool) error {
	pi := fi.Meta().PathInfo

	insertResource := func(pi *paths.Path, fim hugofs.FileMetaDirEntry) {
		key := pi.Base()
		tree := m.treeResources

		commit := tree.Lock(true)
		defer commit()

		var lazyslice *doctree.LazySlice[*resourceSource, resource.Resource]
		n, ok := tree.GetRaw(key)
		if ok {
			lazyslice = n.(*doctree.LazySlice[*resourceSource, resource.Resource])
		} else {
			lazyslice = doctree.NewLazySlice[*resourceSource, resource.Resource](len(m.s.h.Sites))
			tree.Insert(key, lazyslice)
		}

		r := func() (hugio.ReadSeekCloser, error) {
			return fim.Meta().Open()
		}

		dim := m.s.h.resolveDimension(pageTreeDimensionLanguage, pi)
		if dim.IsZero() {
			panic(fmt.Sprintf("failed to resolve dimension for %q", pi.Path()))
		}
		lazyslice.SetSource(dim.Index, &resourceSource{path: pi, opener: r, fi: fim})
	}

	switch pi.BundleType() {
	case paths.PathTypeFile, paths.PathTypeContentResource:
		insertResource(pi, fi)
	default:
		// A content file.
		f, err := source.NewFileInfo(fi)
		if err != nil {
			return err
		}

		p, err := m.s.h.newPage(
			&pageMeta{
				f:        f,
				pathInfo: pi,
				bundled:  false,
			},
		)
		if err != nil {
			return err
		}

		if pi.IsHTML() {
			// This may be a raw HTML template file, and need to trigger a parse of the source to determine that.
			if _, err := p.content.initContentMap(); err != nil {
				return err
			}
		}
		m.treePages.InsertWithLock(pi.Base(), p)
	}
	return nil

}

func (m *pageMap) newResource(ownerPath *paths.Path, fim hugofs.FileMetaDirEntry) (resource.Resource, error) {

	// TODO(bep) consolidate with multihost logic + clean up
	/*outputFormats := owner.m.outputFormats()
	seen := make(map[string]bool)
	var targetBasePaths []string

	// Make sure bundled resources are published to all of the output formats'
	// sub paths.
	/*for _, f := range outputFormats {
		p := f.Path
		if seen[p] {
			continue
		}
		seen[p] = true
		targetBasePaths = append(targetBasePaths, p)

	}*/

	resourcePath := fim.Meta().PathInfo
	meta := fim.Meta()
	r := func() (hugio.ReadSeekCloser, error) {
		return meta.Open()
	}

	return resources.NewResourceLazyInit(resourcePath, r), nil
}

type viewInfoTrait interface {
	Kind() string
	ViewInfo() *contentBundleViewInfo
}

// The home page is represented with the zero string.
// All other keys starts with a leading slash. No trailing slash.
// Slashes are Unix-style.
func cleanTreeKey(elem ...string) string {
	var s string
	if len(elem) > 0 {
		s = elem[0]
		if len(elem) > 1 {
			s = path.Join(elem...)
		}
	}
	s = strings.TrimFunc(s, trimCutsetDotSlashSpace)
	s = filepath.ToSlash(strings.ToLower(paths.Sanitize(s)))
	if s == "" || s == "/" {
		return ""
	}
	if s[0] != '/' {
		s = "/" + s
	}
	return s
}
