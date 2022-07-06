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
	"path"
	"path/filepath"
	"strings"

	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/resources/page"
)

// pageFinder provides ways to find a Page in a Site.
type pageFinder struct {
	pageMap *pageMap
}

func newPageFinder(m *pageMap) *pageFinder {
	if m == nil {
		panic("must provide a pageMap")
	}
	c := &pageFinder{pageMap: m}
	return c
}

// This is an adapter func for the old API with Kind as first argument.
// This is invoked when you do .Site.GetPage. We drop the Kind and fails
// if there are more than 2 arguments, which would be ambiguous.
func (c *pageFinder) getPageOldVersion(ref ...string) (page.Page, error) {
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

// Only used in tests.
func (c *pageFinder) getPage(typ string, sections ...string) page.Page {
	refs := append([]string{typ}, path.Join(sections...))
	p, _ := c.getPageOldVersion(refs...)
	return p
}

// getPageRef resolves a Page from ref/relRef, with a slightly more comprehensive
// search path than getPageNew.
func (c *pageFinder) getPageRef(context page.Page, ref string) (page.Page, error) {
	n, err := c.getContentNode(context, true, ref)
	if err != nil {
		return nil, err
	}
	if p, ok := n.(page.Page); ok {
		return p, nil
	}
	return nil, nil
}

func (c *pageFinder) getPageNew(context page.Page, ref string) (page.Page, error) {
	n, err := c.getContentNode(context, false, filepath.ToSlash(ref))
	if err != nil {
		return nil, err
	}
	if p, ok := n.(page.Page); ok {
		return p, nil
	}
	return nil, nil
}

func (c *pageFinder) getContentNode(context page.Page, isReflink bool, ref string) (contentNodeI, error) {
	const defaultContentExt = ".md"
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
		if context.File() != nil {
			baseDir = context.File().FileInfo().Meta().PathInfo.Dir()
		}

		// TODO1 BundleType

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

	// TODO1

	n = c.pageMap.pageReverseIndex.Get(refPath.BaseNameNoIdentifier())
	if n == ambiguousContentNode {
		return nil, fmt.Errorf("page reference %q is ambiguous", inRef)
	}

	return n, nil
}

func (c *pageFinder) getContentNodeFromPath(refPath *paths.Path, ref string) (contentNodeI, error) {
	m := c.pageMap
	s := refPath.Base()

	n := c.pageMap.treePages.Get(s)
	if n != nil {
		return n, nil
	}

	// Do a reverse lookup assuming this is mounted from somewhere else.
	fi, err := m.s.BaseFs.Content.Fs.Stat(ref + hugofs.SuffixReverseLookup)

	if err == nil {
		meta := fi.(hugofs.MetaProvider).Meta()
		if meta.PathInfo == nil {
			panic("meta.PathInfo is nil")

		}

		n := c.pageMap.treePages.Get(meta.PathInfo.Base())
		if n != nil {
			return n, nil
		}
	}

	return nil, nil
}
