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
	"context"
	"fmt"
	"strings"

	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/resources/page"
)

// pageTree holds the treen navigational method for a Page.
type pageTree struct {
	p *pageState
}

func (pt pageTree) IsAncestor(other any) bool {
	n, ok := other.(contentNodeI)
	if !ok {
		return false
	}

	if n.Path() == pt.p.Path() {
		return false
	}

	return strings.HasPrefix(n.Path(), helpers.AddTrailingSlash(pt.p.Path()))
}

func (pt pageTree) IsDescendant(other any) bool {
	n, ok := other.(contentNodeI)
	if !ok {
		return false
	}

	if n.Path() == pt.p.Path() {
		return false
	}

	return strings.HasPrefix(pt.p.Path(), helpers.AddTrailingSlash(n.Path()))
}

// 2 TODO1 create issue: CurrentSection should navigate sideways for all branch nodes.
func (pt pageTree) CurrentSection() page.Page {
	if pt.p.isContentNodeBranch() {
		return pt.p
	}
	_, n := pt.p.s.pageMap.treePages.LongestPrefix(paths.Dir(pt.p.Path()), func(n contentNodeI) bool { return n.isContentNodeBranch() })
	if n != nil {
		return n.(page.Page)
	}

	printInfoAboutHugoSites(pt.p.s.h)

	panic(fmt.Sprintf("CurrentSection not found for %q in lang %s", pt.p.Path(), pt.p.Lang()))
}

func (pt pageTree) FirstSection() page.Page {
	s := pt.p.Path()
	if !pt.p.isContentNodeBranch() {
		s = paths.Dir(s)
	}

	for {
		k, n := pt.p.s.pageMap.treePages.LongestPrefix(s, func(n contentNodeI) bool { return n.isContentNodeBranch() })
		if n == nil {
			return nil
		}

		// /blog
		if strings.Count(k, "/") <= 1 {
			return n.(page.Page)
		}

		if s == "" {
			return nil
		}

		s = paths.Dir(s)

	}
}

func (pt pageTree) InSection(other any) bool {
	if pt.p == nil || types.IsNil(other) {
		return false
	}

	p, ok := other.(page.Page)
	if !ok {
		return false
	}

	return pt.CurrentSection() == p.CurrentSection()

}

func (pt pageTree) Parent() page.Page {
	if pt.p.IsHome() {
		return nil
	}
	_, n := pt.p.s.pageMap.treePages.LongestPrefix(paths.Dir(pt.p.Path()), nil)
	if n != nil {
		return n.(page.Page)
	}
	return nil
}

func (pt pageTree) Sections() page.Pages {
	var (
		pages       page.Pages
		otherBranch string
		prefix      = helpers.AddTrailingSlash(pt.p.Path())
	)

	pt.p.s.pageMap.treePages.Walk(context.TODO(), doctree.WalkConfig[contentNodeI]{
		Prefix: prefix,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
			if otherBranch == "" || !strings.HasPrefix(key, otherBranch) {
				if p, ok := n.(*pageState); ok && p.IsSection() && p.m.shouldList(false) {
					pages = append(pages, p)
				}
			}
			if n.isContentNodeBranch() {
				otherBranch = key
			}
			return false, nil
		},
	})

	page.SortByDefault(pages)
	return pages
}

func (pt pageTree) Page() page.Page {
	return pt.p
}

func (p pageTree) SectionsEntries() []string {
	return strings.Split(p.SectionsPath(), "/")[1:]
}

func (p pageTree) SectionsPath() string {
	return p.CurrentSection().Path()
}
