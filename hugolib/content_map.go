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

<<<<<<< HEAD
type contentMap struct {
	cfg *contentMapConfig

	// View of regular pages, sections, and taxonomies.
	pageTrees contentTrees

	// View of pages, sections, taxonomies, and resources.
	bundleTrees contentTrees

	// View of sections and taxonomies.
	branchTrees contentTrees

	// Stores page bundles keyed by its path's directory or the base filename,
	// e.g. "blog/post.md" => "/blog/post", "blog/post/index.md" => "/blog/post"
	// These are the "regular pages" and all of them are bundles.
	pages *contentTree

	// A reverse index used as a fallback in GetPage.
	// There are currently two cases where this is used:
	// 1. Short name lookups in ref/relRef, e.g. using only "mypage.md" without a path.
	// 2. Links resolved from a remounted content directory. These are restricted to the same module.
	// Both of the above cases can  result in ambiguous lookup errors.
	pageReverseIndex *contentTreeReverseIndex

	// Section nodes.
	sections *contentTree

	// Taxonomy nodes.
	taxonomies *contentTree

	// Pages in a taxonomy.
	taxonomyEntries *contentTree

	// Resources stored per bundle below a common prefix, e.g. "/blog/post__hb_".
	resources *contentTree
}

func (m *contentMap) AddFiles(fis ...hugofs.FileMetaInfo) error {
	for _, fi := range fis {
		if err := m.addFile(fi); err != nil {
			return err
		}
	}

	return nil
}

func (m *contentMap) AddFilesBundle(header hugofs.FileMetaInfo, resources ...hugofs.FileMetaInfo) error {
	var (
		meta       = header.Meta()
		classifier = meta.Classifier
		isBranch   = classifier == files.ContentClassBranch
		bundlePath = m.getBundleDir(meta)

		n = m.newContentNodeFromFi(header)
		b = m.newKeyBuilder()

		section string
	)

	if isBranch {
		// Either a section or a taxonomy node.
		section = bundlePath
		if tc := m.cfg.getTaxonomyConfig(section); !tc.IsZero() {
			term := strings.TrimPrefix(strings.TrimPrefix(section, "/"+tc.plural), "/")

			n.viewInfo = &contentBundleViewInfo{
				name:       tc,
				termKey:    term,
				termOrigin: term,
			}

			n.viewInfo.ref = n
			b.WithTaxonomy(section).Insert(n)
		} else {
			b.WithSection(section).Insert(n)
		}
	} else {
		// A regular page. Attach it to its section.
		section, _ = m.getOrCreateSection(n, bundlePath)
		b = b.WithSection(section).ForPage(bundlePath).Insert(n)
	}

	if m.cfg.isRebuild {
		// The resource owner will be either deleted or overwritten on rebuilds,
		// but make sure we handle deletion of resources (images etc.) as well.
		b.ForResource("").DeleteAll()
	}

	for _, r := range resources {
		rb := b.ForResource(cleanTreeKey(r.Meta().Path))
		rb.Insert(&contentNode{fi: r})
	}

	return nil
}

func (m *contentMap) CreateMissingNodes() error {
	// Create missing home and root sections
	rootSections := make(map[string]any)
	trackRootSection := func(s string, b *contentNode) {
		parts := strings.Split(s, "/")
		if len(parts) > 2 {
			root := strings.TrimSuffix(parts[1], cmBranchSeparator)
			if root != "" {
				if _, found := rootSections[root]; !found {
					rootSections[root] = b
				}
			}
		}
	}

	m.sections.Walk(func(s string, v any) bool {
		n := v.(*contentNode)

		if s == "/" {
			return false
		}

		trackRootSection(s, n)
		return false
	})

	m.pages.Walk(func(s string, v any) bool {
		trackRootSection(s, v.(*contentNode))
		return false
	})

	if _, found := rootSections["/"]; !found {
		rootSections["/"] = true
	}

	for sect, v := range rootSections {
		var sectionPath string
		if n, ok := v.(*contentNode); ok && n.path != "" {
			sectionPath = n.path
			firstSlash := strings.Index(sectionPath, "/")
			if firstSlash != -1 {
				sectionPath = sectionPath[:firstSlash]
			}
		}
		sect = cleanSectionTreeKey(sect)
		_, found := m.sections.Get(sect)
		if !found {
			m.sections.Insert(sect, &contentNode{path: sectionPath})
		}
	}

	for _, view := range m.cfg.taxonomyConfig {
		s := cleanSectionTreeKey(view.plural)
		_, found := m.taxonomies.Get(s)
		if !found {
			b := &contentNode{
				viewInfo: &contentBundleViewInfo{
					name: view,
				},
			}
			b.viewInfo.ref = b
			m.taxonomies.Insert(s, b)
		}
	}

	return nil
}

func (m *contentMap) getBundleDir(meta *hugofs.FileMeta) string {
	dir := cleanTreeKey(filepath.Dir(meta.Path))

	switch meta.Classifier {
	case files.ContentClassContent:
		return path.Join(dir, meta.TranslationBaseName)
	default:
		return dir
	}
}

func (m *contentMap) newContentNodeFromFi(fi hugofs.FileMetaInfo) *contentNode {
	return &contentNode{
		fi:   fi,
		path: strings.TrimPrefix(filepath.ToSlash(fi.Meta().Path), "/"),
	}
}

func (m *contentMap) getFirstSection(s string) (string, *contentNode) {
	s = helpers.AddTrailingSlash(s)
	for {
		k, v, found := m.sections.LongestPrefix(s)

		if !found {
			return "", nil
		}

		if strings.Count(k, "/") <= 2 {
			return k, v.(*contentNode)
		}

		s = helpers.AddTrailingSlash(path.Dir(strings.TrimSuffix(s, "/")))

	}
}

func (m *contentMap) newKeyBuilder() *cmInsertKeyBuilder {
	return &cmInsertKeyBuilder{m: m}
}

func (m *contentMap) getOrCreateSection(n *contentNode, s string) (string, *contentNode) {
	level := strings.Count(s, "/")
	k, b := m.getSection(s)

	mustCreate := false

	if k == "" {
		mustCreate = true
	} else if level > 1 && k == "/" {
		// We found the home section, but this page needs to be placed in
		// the root, e.g. "/blog", section.
		mustCreate = true
	}

	if mustCreate {
		k = cleanSectionTreeKey(s[:strings.Index(s[1:], "/")+1])

		b = &contentNode{
			path: n.rootSection(),
		}

		m.sections.Insert(k, b)
	}

	return k, b
}

func (m *contentMap) getPage(section, name string) *contentNode {
	section = helpers.AddTrailingSlash(section)
	key := section + cmBranchSeparator + name + cmLeafSeparator

	v, found := m.pages.Get(key)
	if found {
		return v.(*contentNode)
	}
	return nil
}

func (m *contentMap) getSection(s string) (string, *contentNode) {
	s = helpers.AddTrailingSlash(path.Dir(strings.TrimSuffix(s, "/")))

	k, v, found := m.sections.LongestPrefix(s)

	if found {
		return k, v.(*contentNode)
	}
	return "", nil
}

func (m *contentMap) getTaxonomyParent(s string) (string, *contentNode) {
	s = helpers.AddTrailingSlash(path.Dir(strings.TrimSuffix(s, "/")))
	k, v, found := m.taxonomies.LongestPrefix(s)

	if found {
		return k, v.(*contentNode)
	}

	v, found = m.sections.Get("/")
	if found {
		return s, v.(*contentNode)
	}

	return "", nil
}

func (m *contentMap) addFile(fi hugofs.FileMetaInfo) error {
	b := m.newKeyBuilder()
	return b.WithFile(fi).Insert(m.newContentNodeFromFi(fi)).err
}

func cleanTreeKey(k string) string {
	k = "/" + strings.ToLower(strings.Trim(path.Clean(filepath.ToSlash(k)), "./"))
	return k
}

func cleanSectionTreeKey(k string) string {
	k = cleanTreeKey(k)
	if k != "/" {
		k += "/"
	}

	return k
}

func (m *contentMap) onSameLevel(s1, s2 string) bool {
	return strings.Count(s1, "/") == strings.Count(s2, "/")
}

func (m *contentMap) deleteBundleMatching(matches func(b *contentNode) bool) {
	// Check sections first
	s := m.sections.getMatch(matches)
	if s != "" {
		m.deleteSectionByPath(s)
		return
	}

	s = m.pages.getMatch(matches)
	if s != "" {
		m.deletePage(s)
		return
	}

	s = m.resources.getMatch(matches)
	if s != "" {
		m.resources.Delete(s)
	}
}

// Deletes any empty root section that's not backed by a content file.
func (m *contentMap) deleteOrphanSections() {
	var sectionsToDelete []string

	m.sections.Walk(func(s string, v any) bool {
		n := v.(*contentNode)

		if n.fi != nil {
			// Section may be empty, but is backed by a content file.
			return false
		}

		if s == "/" || strings.Count(s, "/") > 2 {
			return false
		}

		prefixBundle := s + cmBranchSeparator

		if !(m.sections.hasBelow(s) || m.pages.hasBelow(prefixBundle) || m.resources.hasBelow(prefixBundle)) {
			sectionsToDelete = append(sectionsToDelete, s)
		}

		return false
	})

	for _, s := range sectionsToDelete {
		m.sections.Delete(s)
	}
}

func (m *contentMap) deletePage(s string) {
	m.pages.DeletePrefix(s)
	m.resources.DeletePrefix(s)
}

func (m *contentMap) deleteSectionByPath(s string) {
	if !strings.HasSuffix(s, "/") {
		panic("section must end with a slash")
	}
	if !strings.HasPrefix(s, "/") {
		panic("section must start with a slash")
	}
	m.sections.DeletePrefix(s)
	m.pages.DeletePrefix(s)
	m.resources.DeletePrefix(s)
}

func (m *contentMap) deletePageByPath(s string) {
	m.pages.Walk(func(s string, v any) bool {
		fmt.Println("S", s)

		return false
	})
}

func (m *contentMap) deleteTaxonomy(s string) {
	m.taxonomies.DeletePrefix(s)
}

func (m *contentMap) reduceKeyPart(dir, filename string) string {
	dir, filename = filepath.ToSlash(dir), filepath.ToSlash(filename)
	dir, filename = strings.TrimPrefix(dir, "/"), strings.TrimPrefix(filename, "/")

	return strings.TrimPrefix(strings.TrimPrefix(filename, dir), "/")
}

func (m *contentMap) splitKey(k string) []string {
	if k == "" || k == "/" {
		return nil
	}

	return strings.Split(k, "/")[1:]
}

func (m *contentMap) testDump() string {
	var sb strings.Builder

	for i, r := range []*contentTree{m.pages, m.sections, m.resources} {
		sb.WriteString(fmt.Sprintf("Tree %d:\n", i))
		r.Walk(func(s string, v any) bool {
			sb.WriteString("\t" + s + "\n")
			return false
		})
	}

	for i, r := range []*contentTree{m.pages, m.sections} {
		r.Walk(func(s string, v any) bool {
			c := v.(*contentNode)
			cpToString := func(c *contentNode) string {
				var sb strings.Builder
				if c.p != nil {
					sb.WriteString("|p:" + c.p.Title())
				}
				if c.fi != nil {
					sb.WriteString("|f:" + filepath.ToSlash(c.fi.Meta().Path))
				}
				return sb.String()
			}
			sb.WriteString(path.Join(m.cfg.lang, r.Name) + s + cpToString(c) + "\n")

			resourcesPrefix := s

			if i == 1 {
				resourcesPrefix += cmLeafSeparator

				m.pages.WalkPrefix(s+cmBranchSeparator, func(s string, v any) bool {
					sb.WriteString("\t - P: " + filepath.ToSlash((v.(*contentNode).fi.(hugofs.FileMetaInfo)).Meta().Filename) + "\n")
					return false
				})
			}

			m.resources.WalkPrefix(resourcesPrefix, func(s string, v any) bool {
				sb.WriteString("\t - R: " + filepath.ToSlash((v.(*contentNode).fi.(hugofs.FileMetaInfo)).Meta().Filename) + "\n")
				return false
			})

			return false
		})
	}

	return sb.String()
=======
type contentKindProvider interface {
	Kind() string
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
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
