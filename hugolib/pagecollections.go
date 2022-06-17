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
	"sync"

	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/resources/page"
)

// PageCollections contains the page collections for a site.
type PageCollections struct {
	pageMap *pageMap

	// Lazy initialized page collections
	pages           *lazyPagesFactory
	regularPages    *lazyPagesFactory
	allPages        *lazyPagesFactory
	allRegularPages *lazyPagesFactory
}

// Pages returns all pages.
// This is for the current language only.
func (c *PageCollections) Pages() page.Pages {
	return c.pages.get()
}

// RegularPages returns all the regular pages.
// This is for the current language only.
func (c *PageCollections) RegularPages() page.Pages {
	return c.regularPages.get()
}

// AllPages returns all pages for all languages.
func (c *PageCollections) AllPages() page.Pages {
	return c.allPages.get()
}

// AllPages returns all regular pages for all languages.
func (c *PageCollections) AllRegularPages() page.Pages {
	return c.allRegularPages.get()
}

type lazyPagesFactory struct {
	pages page.Pages

	init    sync.Once
	factory page.PagesFactory
}

func (l *lazyPagesFactory) get() page.Pages {
	l.init.Do(func() {
		l.pages = l.factory()
	})
	return l.pages
}

func newLazyPagesFactory(factory page.PagesFactory) *lazyPagesFactory {
	return &lazyPagesFactory{factory: factory}
}

func newPageCollections(m *pageMap) *PageCollections {
	if m == nil {
		panic("must provide a pageMap")
	}

	c := &PageCollections{pageMap: m}

	c.pages = newLazyPagesFactory(func() page.Pages {
		return m.CreateListAllPages()
	})

	c.regularPages = newLazyPagesFactory(func() page.Pages {
		return c.findPagesByKindIn(pagekinds.Page, c.pages.get())
	})

	return c
}

// This is an adapter func for the old API with Kind as first argument.
// This is invoked when you do .Site.GetPage. We drop the Kind and fails
// if there are more than 2 arguments, which would be ambiguous.
func (c *PageCollections) getPageOldVersion(ref ...string) (page.Page, error) {
	var refs []string
	for _, r := range ref {
		// A common construct in the wild is
		// .Site.GetPage "home" "" or
		// .Site.GetPage "home" "/"
		if r != "" && r != "/" {
			refs = append(refs, r)
		}
	}

	var key string

	if len(refs) > 2 {
		// This was allowed in Hugo <= 0.44, but we cannot support this with the
		// new API. This should be the most unusual case.
		return nil, fmt.Errorf(`too many arguments to .Site.GetPage: %v. Use lookups on the form {{ .Site.GetPage "/posts/mypage-md" }}`, ref)
	}

	if len(refs) == 0 || refs[0] == pagekinds.Home {
		key = "/"
	} else if len(refs) == 1 {
		if len(ref) == 2 && refs[0] == pagekinds.Section {
			// This is an old style reference to the "Home Page section".
			// Typically fetched via {{ .Site.GetPage "section" .Section }}
			// See https://github.com/gohugoio/hugo/issues/4989
			key = "/"
		} else {
			key = refs[0]
		}
	} else {
		key = refs[1]
	}

	key = filepath.ToSlash(key)
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}

	return c.getPageNew(nil, key)
}

// 	Only used in tests.
func (c *PageCollections) getPage(typ string, sections ...string) page.Page {
	refs := append([]string{typ}, path.Join(sections...))
	p, _ := c.getPageOldVersion(refs...)
	return p
}

// getPageRef resolves a Page from ref/relRef, with a slightly more comprehensive
// search path than getPageNew.
func (c *PageCollections) getPageRef(context page.Page, ref string) (page.Page, error) {
	n, err := c.getContentNode(context, true, ref)
	if err != nil || n == nil || n.p == nil {
		return nil, err
	}
	return n.p, nil
}

func (c *PageCollections) getPageNew(context page.Page, ref string) (page.Page, error) {
	n, err := c.getContentNode(context, false, filepath.ToSlash(ref))
	if err != nil || n == nil || n.p == nil {
		return nil, err
	}
	return n.p, nil
}

func (c *PageCollections) getContentNode(context page.Page, isReflink bool, ref string) (*contentNode, error) {
	const defaultContentExt = ".md"
	m := c.pageMap
	inRef := ref
	if ref == "" {
		ref = "/"
	}
	ref = paths.Sanitize(ref)

	if !paths.HasExt(ref) {
		// We are always looking for a content file and having an extension greatly simplifies the code that follows,
		// even in the case where the extension does not match this one.
		if ref == "/" {
			ref = "/_index" + defaultContentExt
		} else {
			ref = ref + defaultContentExt
		}
	}

	if context != nil && !strings.HasPrefix(ref, "/") {
		// Try the page-relative path first.
		// Branch pages: /mysection, "./mypage" => /mysection/mypage
		// Regular pages: /mysection/mypage.md, Path=/mysection/mypage, "./someotherpage" => /mysection/mypage/../someotherpage
		// Regular leaf bundles: /mysection/mypage/index.md, Path=/mysection/mypage, "./someotherpage" => /mysection/mypage/../someotherpage
		// Given the above, for regular pages we use the containing folder.
		var baseDir string

		if context.IsPage() {
			baseDir = filepath.Dir(context.Path())
		} else {
			baseDir = context.Path()
		}
		rel := path.Join(baseDir, inRef)
		if !paths.HasExt(rel) {
			// See comment above.
			rel += defaultContentExt
		}
		relPath := paths.Parse(rel)

		n, err := c.getContentNodeFromPath(relPath, ref)
		if n != nil || err != nil {
			return n, err
		}
	}

	if strings.HasPrefix(ref, ".") {
		// Page relative, no need to look further.
		return nil, nil
	}

	refPath := paths.Parse(ref)

	n, err := c.getContentNodeFromPath(refPath, ref)
	if n != nil || err != nil {
		return n, err
	}

	var doSimpleLookup bool
	if isReflink || context == nil {
		slashCount := strings.Count(inRef, "/")
		if slashCount <= 1 {
			doSimpleLookup = slashCount == 0 || ref[0] == '/'
		}
	}

	if !doSimpleLookup {
		return nil, nil
	}
	n = m.pageReverseIndex.Get(refPath.BaseNameNoIdentifier())
	if n == ambiguousContentNode {
		return nil, fmt.Errorf("page reference %q is ambiguous", inRef)
	}

	return n, nil
}

func (c *PageCollections) getContentNodeFromPath(refPath *paths.Path, ref string) (*contentNode, error) {
	m := c.pageMap
	s := refPath.Base()

	if s == "/" {
		// Home page.
		b := m.GetBranch("")
		if b == nil {
			return nil, nil
		}
		return b.n, nil
	}

	if refPath.BundleType() == paths.PathTypeBranch {
		b := m.GetBranch(s)
		if b == nil {
			return nil, nil
		}
		return b.n, nil
	} else if refPath.BundleType() == paths.PathTypeLeaf {
		n := m.GetLeaf(s)
		if n == nil {
			// TODO1 check this.
			n = m.GetLeaf(s + "/index")
		}
		if n != nil {
			return n, nil
		}
	} else {
		n := m.GetBranchOrLeaf(s)
		if n != nil {
			return n, nil
		}
	}

	rfs := m.s.BaseFs.Content.Fs.(hugofs.ReverseLookupProvider)
	// Try first with the ref as is. It may be a file mount.
	realToVirtual, err := rfs.ReverseLookup(ref)
	if err != nil {
		return nil, err
	}

	if realToVirtual == "" {
		realToVirtual, err = rfs.ReverseLookup(s)
		if err != nil {
			return nil, err
		}
	}

	if realToVirtual != "" {
		n := m.GetBranchOrLeaf(paths.Parse(realToVirtual).Base())
		if n != nil {
			return n, nil
		}
	}

	return nil, nil
}

func (*PageCollections) findPagesByKindIn(kind string, inPages page.Pages) page.Pages {
	var pages page.Pages
	for _, p := range inPages {
		if p.Kind() == kind {
			pages = append(pages, p)
		}
	}
	return pages
}
