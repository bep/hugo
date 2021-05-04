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

	"github.com/gohugoio/hugo/helpers"

	"github.com/gohugoio/hugo/resources/page"

	"github.com/gohugoio/hugo/hugofs/files"

	"github.com/gohugoio/hugo/hugofs"
)

// TODO1 remove one
const (
	contentMapNodeSeparator = "/"
	cmLeafSeparator         = "/"
	// The home page.
	contentMapRoot = ""
)

// Used to mark ambiguous keys in reverse index lookups.
var ambiguousContentNode = &contentNode{}

type contentBundleViewInfo struct {
	ordinal    int
	name       viewName
	termKey    string
	termOrigin string
	weight     int
	ref        *contentNode
}

func (c *contentBundleViewInfo) term() string {
	if c.termOrigin != "" {
		return c.termOrigin
	}

	return c.termKey
}

func (m *pageMap) getBundleDir(meta hugofs.FileMeta) string {
	dir := cleanTreeKey(filepath.Dir(meta.Path()))

	switch meta.Classifier() {
	case files.ContentClassContent:
		return path.Join(dir, meta.TranslationBaseName())
	default:
		return dir
	}
}

func (m *pageMap) newContentNodeFromFi(fi hugofs.FileMetaInfo) *contentNode {
	return &contentNode{
		fi: fi,
		// TODO1 used for?
		path: strings.TrimPrefix(filepath.ToSlash(fi.Meta().Path()), "/"),
	}
}

func (m *sectionMap) getFirstSection(s string) (string, *contentNode) {
	for {
		k, v, found := m.sections.LongestPrefix(s)

		if !found {
			return "", nil
		}

		// /blog
		if strings.Count(k, "/") <= 1 {
			return k, v.(*contentBranchNode).n
		}

		s = path.Dir(s)

	}

}

// The home page is represented with the zero string.
// All other keys starts with a leading slash. No leading slash.
// Slashes are Unix-style.
func cleanTreeKey(k string) string {
	k = strings.ToLower(strings.TrimFunc(path.Clean(filepath.ToSlash(k)), func(r rune) bool {
		return r == '.' || r == '/'
	}))
	if k == "" || k == "/" {
		return ""
	}
	return helpers.AddLeadingSlash(k)
}

type contentMapConfig struct {
	lang                 string
	taxonomyConfig       taxonomiesConfigValues
	taxonomyDisabled     bool
	taxonomyTermDisabled bool
	pageDisabled         bool
	isRebuild            bool
}

func (cfg contentMapConfig) getTaxonomyConfig(s string) (v viewName) {
	s = strings.TrimPrefix(s, "/")
	if s == "" {
		return
	}
	for _, n := range cfg.taxonomyConfig.views {
		if strings.HasPrefix(s, n.plural) {
			return n
		}
	}

	return
}

type contentNode struct {
	p *pageState

	// Set for taxonomy nodes.
	viewInfo *contentBundleViewInfo

	// Set if source is a file.
	// We will soon get other sources.
	fi hugofs.FileMetaInfo

	// The source path. Unix slashes. No leading slash.
	// TODO1 check usage
	path string
}

// Returns whether this is a view node (a taxonomy or a term).
func (b *contentNode) isView() bool {
	return b.viewInfo != nil
}

func (b *contentNode) rootSection() string {
	if b.path == "" {
		return ""
	}
	firstSlash := strings.Index(b.path, "/")
	if firstSlash == -1 {
		return b.path
	}

	return b.path[:firstSlash]
}

func (m *pageMap) AddFilesBundle(header hugofs.FileMetaInfo, resources ...hugofs.FileMetaInfo) error {

	var (
		meta       = header.Meta()
		classifier = meta.Classifier()
		isBranch   = classifier == files.ContentClassBranch
		key        = cleanTreeKey(m.getBundleDir(meta))
		n          = m.newContentNodeFromFi(header)

		pageTree *contentBranchNode
	)

	if !isBranch && m.cfg.pageDisabled {
		return nil
	}

	if isBranch {
		// Either a section or a taxonomy node.
		if tc := m.cfg.getTaxonomyConfig(key); !tc.IsZero() {
			term := strings.TrimPrefix(strings.TrimPrefix(key, "/"+tc.plural), "/")
			n.viewInfo = &contentBundleViewInfo{
				name:       tc,
				termKey:    term,
				termOrigin: term,
			}

			n.viewInfo.ref = n
			pageTree = m.InsertSection(key, n)

		} else {
			key := cleanTreeKey(key)
			pageTree = m.InsertSection(key, n)
		}
	} else {

		// A regular page. Attach it to its section.
		_, pageTree = m.getOrCreateSection(n, key)
		if pageTree == nil {
			panic(fmt.Sprintf("NO section %s", key))
		}
		pageTree.InsertPage(key, n)
	}

	if m.cfg.isRebuild {
		// The resource owner will be either deleted or overwritten on rebuilds,
		// but make sure we handle deletion of resources (images etc.) as well.
		// TODO1 b.ForResource("").DeleteAll()
	}

	resourceTree := pageTree.pageResources
	if isBranch {
		resourceTree = pageTree.resources
	}

	for _, r := range resources {
		key := cleanTreeKey(r.Meta().Path())
		resourceTree.nodes.Insert(key, &contentNode{fi: r})
	}

	return nil
}

func (m *pageMap) getOrCreateSection(n *contentNode, s string) (string, *contentBranchNode) {
	level := strings.Count(s, "/")

	k, pageTree := m.LongestPrefix(path.Dir(s))

	mustCreate := false

	if pageTree == nil {
		mustCreate = true
	} else if level > 1 && k == "" {
		// We found the home section, but this page needs to be placed in
		// the root, e.g. "/blog", section.
		mustCreate = true
	} else {
		return k, pageTree
	}

	if !mustCreate {
		return k, pageTree
	}

	k = cleanTreeKey(s[:strings.Index(s[1:], "/")+1])

	n = &contentNode{
		path: n.rootSection(), // TODO1
	}

	if k != "" {
		// Make sure we always have the root/home node.
		if m.Get("") == nil {
			m.InsertSection("", &contentNode{})
		}
	}

	pageTree = m.InsertSection(k, n)
	return k, pageTree
}

type contentTreeNodeCallback func(s string, n *contentNode) bool
type contentTreeBranchNodeCallback func(s string, current *contentBranchNode) bool

type contentTreeOwnerNodeCallback func(
	// The branch in which n belongs.
	branch *contentBranchNode,

	// Owner of n.
	owner *contentNode,

	// The key
	key string,

	// The content node, either a Page or a Resource.
	n *contentNode,
) bool

type contentTreeOwnerBranchNodeCallback func(
	// The branch in which n belongs.
	branch *contentBranchNode,

	// Owner of n.
	owner *contentBranchNode,

	// The key
	key string,

	// The content node, either a Page or a Resource.
	n *contentNode,
) bool

func newcontentTreeNodeCallbackChain(callbacks ...contentTreeNodeCallback) contentTreeNodeCallback {
	return func(s string, n *contentNode) bool {
		for i, cb := range callbacks {
			// Allow the last callback to stop the walking.
			if i == len(callbacks)-1 {
				return cb(s, n)
			}

			if cb(s, n) {
				// Skip the rest of the callbacks, but continue walking.
				return false
			}
		}
		return false
	}
}

var (
	contentTreeNoListAlwaysFilter = func(s string, n *contentNode) bool {
		if n.p == nil {
			return true
		}
		return n.p.m.noListAlways()
	}

	contentTreeNoRenderFilter = func(s string, n *contentNode) bool {
		if n.p == nil {
			return true
		}
		return n.p.m.noRender()
	}

	contentTreeNoLinkFilter = func(s string, n *contentNode) bool {
		if n.p == nil {
			return true
		}
		return n.p.m.noLink()
	}

	contentTreeNoopFilter = func(s string, n *contentNode) bool {
		return false
	}
)

// contentTreeRef points to a branch node in the given map.
type contentTreeRef struct {
	m      *pageMap // TODO1 used?
	branch *contentBranchNode
	owner  *contentBranchNode
	key    string
	n      *contentNode // TODO1 used?
}

func (c *contentTreeRef) getCurrentSection() *contentBranchNode {
	if c.isSection() {
		return c.owner
	}
	return c.getSection()
}

func (c *contentTreeRef) isSection() bool {
	return c.branch != nil && c.branch != c.owner
}

func (c *contentTreeRef) getSection() *contentBranchNode {
	return c.branch
}

func (c *contentTreeRef) getRegularPagesRecursive() page.Pages {
	var pas page.Pages

	q := sectionMapQuery{
		Exclude: c.n.p.m.getListFilter(true),
		Branch: sectionMapQueryCallBacks{
			Key: newSectionMapQueryKey(c.key+"/", true),
		},
		Leaf: sectionMapQueryCallBacks{
			Page: func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
				pas = append(pas, n.p)
				return false
			},
		},
	}

	c.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}

func (c *contentTreeRef) getRegularPages() page.Pages {
	var pas page.Pages

	q := sectionMapQuery{
		Exclude: c.n.p.m.getListFilter(true),
		Branch: sectionMapQueryCallBacks{
			Key: newSectionMapQueryKey(c.key, false),
		},
		Leaf: sectionMapQueryCallBacks{
			Page: func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
				pas = append(pas, n.p)
				return false
			},
		},
	}

	c.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}

func (c *contentTreeRef) getPagesAndSections() page.Pages {
	var pas page.Pages

	c.m.WalkPagesPrefixSectionNoRecurse(
		c.key+"/",
		noTaxonomiesFilter,
		c.n.p.m.getListFilter(true),
		func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
			pas = append(pas, n.p)
			return false
		},
	)

	page.SortByDefault(pas)

	return pas
}

func (c *contentTreeRef) getSections() page.Pages {
	var pas page.Pages

	q := sectionMapQuery{
		NoRecurse:     true,
		Exclude:       c.n.p.m.getListFilter(true),
		BranchExclude: noTaxonomiesFilter,
		Branch: sectionMapQueryCallBacks{
			Key: newSectionMapQueryKey(c.key+"/", true),
			Page: func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
				pas = append(pas, n.p)
				return false
			},
		},
	}

	c.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}
