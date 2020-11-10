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
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/parser/pageparser"

	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gobuffalo/flect"
	"github.com/gohugoio/hugo/output"

	"github.com/gohugoio/hugo/common/types"

	"github.com/gohugoio/hugo/helpers"

<<<<<<< HEAD
	"github.com/gohugoio/hugo/resources/page"

	"github.com/gohugoio/hugo/hugofs/files"

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"github.com/gohugoio/hugo/hugofs"
)

// Used to mark ambiguous keys in reverse index lookups.
var ambiguousContentNode = &contentNode{}

<<<<<<< HEAD
func newContentMap(cfg contentMapConfig) *contentMap {
	m := &contentMap{
		cfg:             &cfg,
		pages:           &contentTree{Name: "pages", Tree: radix.New()},
		sections:        &contentTree{Name: "sections", Tree: radix.New()},
		taxonomies:      &contentTree{Name: "taxonomies", Tree: radix.New()},
		taxonomyEntries: &contentTree{Name: "taxonomyEntries", Tree: radix.New()},
		resources:       &contentTree{Name: "resources", Tree: radix.New()},
	}

	m.pageTrees = []*contentTree{
		m.pages, m.sections, m.taxonomies,
	}

	m.bundleTrees = []*contentTree{
		m.pages, m.sections, m.taxonomies, m.resources,
	}

	m.branchTrees = []*contentTree{
		m.sections, m.taxonomies,
	}

	addToReverseMap := func(k string, n *contentNode, m map[any]*contentNode) {
		k = strings.ToLower(k)
		existing, found := m[k]
		if found && existing != ambiguousContentNode {
			m[k] = ambiguousContentNode
		} else if !found {
			m[k] = n
		}
	}

	m.pageReverseIndex = &contentTreeReverseIndex{
		t: []*contentTree{m.pages, m.sections, m.taxonomies},
		contentTreeReverseIndexMap: &contentTreeReverseIndexMap{
			initFn: func(t *contentTree, m map[any]*contentNode) {
				t.Walk(func(s string, v any) bool {
					n := v.(*contentNode)
					if n.p != nil && !n.p.File().IsZero() {
						meta := n.p.File().FileInfo().Meta()
						if meta.Path != meta.PathFile() {
							// Keep track of the original mount source.
							mountKey := filepath.ToSlash(filepath.Join(meta.Module, meta.PathFile()))
							addToReverseMap(mountKey, n, m)
						}
					}
					k := strings.TrimPrefix(strings.TrimSuffix(path.Base(s), cmLeafSeparator), cmBranchSeparator)
					addToReverseMap(k, n, m)
					return false
				})
			},
		},
	}

	return m
}

type cmInsertKeyBuilder struct {
	m *contentMap

	err error

	// Builder state
	tree    *contentTree
	baseKey string // Section or page key
	key     string
}

func (b cmInsertKeyBuilder) ForPage(s string) *cmInsertKeyBuilder {
	// fmt.Println("ForPage:", s, "baseKey:", b.baseKey, "key:", b.key)
	baseKey := b.baseKey
	b.baseKey = s

	if baseKey != "/" {
		// Don't repeat the section path in the key.
		s = strings.TrimPrefix(s, baseKey)
	}
	s = strings.TrimPrefix(s, "/")

	switch b.tree {
	case b.m.sections:
		b.tree = b.m.pages
		b.key = baseKey + cmBranchSeparator + s + cmLeafSeparator
	case b.m.taxonomies:
		b.key = path.Join(baseKey, s)
	default:
		panic("invalid state")
	}

	return &b
}

func (b cmInsertKeyBuilder) ForResource(s string) *cmInsertKeyBuilder {
	// fmt.Println("ForResource:", s, "baseKey:", b.baseKey, "key:", b.key)

	baseKey := helpers.AddTrailingSlash(b.baseKey)
	s = strings.TrimPrefix(s, baseKey)

	switch b.tree {
	case b.m.pages:
		b.key = b.key + s
	case b.m.sections, b.m.taxonomies:
		b.key = b.key + cmLeafSeparator + s
	default:
		panic(fmt.Sprintf("invalid state: %#v", b.tree))
	}
	b.tree = b.m.resources
	return &b
}

func (b *cmInsertKeyBuilder) Insert(n *contentNode) *cmInsertKeyBuilder {
	if b.err == nil {
		b.tree.Insert(b.Key(), n)
	}
	return b
}

func (b *cmInsertKeyBuilder) Key() string {
	switch b.tree {
	case b.m.sections, b.m.taxonomies:
		return cleanSectionTreeKey(b.key)
	default:
		return cleanTreeKey(b.key)
	}
}

func (b *cmInsertKeyBuilder) DeleteAll() *cmInsertKeyBuilder {
	if b.err == nil {
		b.tree.DeletePrefix(b.Key())
	}
	return b
}

func (b *cmInsertKeyBuilder) WithFile(fi hugofs.FileMetaInfo) *cmInsertKeyBuilder {
	b.newTopLevel()
	m := b.m
	meta := fi.Meta()
	p := cleanTreeKey(meta.Path)
	bundlePath := m.getBundleDir(meta)
	isBundle := meta.Classifier.IsBundle()
	if isBundle {
		panic("not implemented")
	}

	p, k := b.getBundle(p)
	if k == "" {
		b.err = fmt.Errorf("no bundle header found for %q", bundlePath)
		return b
	}

	id := k + m.reduceKeyPart(p, fi.Meta().Path)
	b.tree = b.m.resources
	b.key = id
	b.baseKey = p

	return b
}

func (b *cmInsertKeyBuilder) WithSection(s string) *cmInsertKeyBuilder {
	s = cleanSectionTreeKey(s)
	b.newTopLevel()
	b.tree = b.m.sections
	b.baseKey = s
	b.key = s
	return b
}

func (b *cmInsertKeyBuilder) WithTaxonomy(s string) *cmInsertKeyBuilder {
	s = cleanSectionTreeKey(s)
	b.newTopLevel()
	b.tree = b.m.taxonomies
	b.baseKey = s
	b.key = s
	return b
}

// getBundle gets both the key to the section and the prefix to where to store
// this page bundle and its resources.
func (b *cmInsertKeyBuilder) getBundle(s string) (string, string) {
	m := b.m
	section, _ := m.getSection(s)

	p := strings.TrimPrefix(s, section)

	bundlePathParts := strings.Split(p, "/")
	basePath := section + cmBranchSeparator

	// Put it into an existing bundle if found.
	for i := len(bundlePathParts) - 2; i >= 0; i-- {
		bundlePath := path.Join(bundlePathParts[:i]...)
		searchKey := basePath + bundlePath + cmLeafSeparator
		if _, found := m.pages.Get(searchKey); found {
			return section + bundlePath, searchKey
		}
	}

	// Put it into the section bundle.
	return section, section + cmLeafSeparator
}

func (b *cmInsertKeyBuilder) newTopLevel() {
	b.key = ""
}

type contentBundleViewInfo struct {
	ordinal    int
	name       viewName
	termKey    string
	termOrigin string
	weight     int
	ref        *contentNode
}

func (c *contentBundleViewInfo) kind() string {
	if c.termKey != "" {
		return page.KindTerm
	}
	return page.KindTaxonomy
}

func (c *contentBundleViewInfo) sections() []string {
	if c.kind() == page.KindTaxonomy {
		return []string{c.name.plural}
	}

	return []string{c.name.plural, c.termKey}
}

func (c *contentBundleViewInfo) term() string {
	if c.termOrigin != "" {
		return c.termOrigin
	}

	return c.termKey
}

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
	// Both of the above cases can  result in ambigous lookup errors.
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
}

type contentMapConfig struct {
	lang                 string
	taxonomyConfig       []viewName
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
	for _, n := range cfg.taxonomyConfig {
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
	path string
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

type contentTree struct {
	Name string
	*radix.Tree
}

type contentTrees []*contentTree

func (t contentTrees) DeletePrefix(prefix string) int {
	var count int
	for _, tree := range t {
		tree.Walk(func(s string, v any) bool {
			return false
		})
		count += tree.DeletePrefix(prefix)
	}
	return count
}

type contentTreeNodeCallback func(s string, n *contentNode) bool

func newContentTreeFilter(fn func(n *contentNode) bool) contentTreeNodeCallback {
	return func(s string, n *contentNode) bool {
		return fn(n)
	}
}

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
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
)

<<<<<<< HEAD
func (c *contentTree) WalkQuery(query pageMapQuery, walkFn contentTreeNodeCallback) {
	filter := query.Filter
	if filter == nil {
		filter = contentTreeNoListAlwaysFilter
	}
	if query.Prefix != "" {
		c.WalkBelow(query.Prefix, func(s string, v any) bool {
			n := v.(*contentNode)
			if filter != nil && filter(s, n) {
=======
var (
	_ contentKindProvider = (*contentBundleViewInfo)(nil)
	_ viewInfoTrait       = (*contentBundleViewInfo)(nil)
)

var trimCutsetDotSlashSpace = func(r rune) bool {
	return r == '.' || r == '/' || unicode.IsSpace(r)
}

func newcontentTreeNodeCallbackChain(callbacks ...contentTreeNodeCallback) contentTreeNodeCallback {
	return func(s string, n *contentNode) bool {
		for i, cb := range callbacks {
			// Allow the last callback to stop the walking.
			if i == len(callbacks)-1 {
				return cb(s, n)
			}

			if cb(s, n) {
				// Skip the rest of the callbacks, but continue walking.
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
				return false
			}
		}
		return false
	}
}

type contentBundleViewInfo struct {
	name viewName
	term string
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

type contentGetBranchProvider interface {
	// GetBranch returns the the current branch, which will be itself
	// for branch nodes (e.g. sections).
	// To always navigate upwards, use GetContainerBranch().
	GetBranch() *contentBranchNode
}

type contentGetContainerBranchProvider interface {
	// GetContainerBranch returns the container for pages and sections.
	GetContainerBranch() *contentBranchNode
}

type contentGetContainerNodeProvider interface {
	// GetContainerNode returns the container for resources.
	GetContainerNode() *contentNode
}

type contentGetNodeProvider interface {
	GetNode() *contentNode
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

func (cfg contentMapConfig) getTaxonomyConfig(s string) (v viewName) {
	s = strings.TrimPrefix(s, "/")
	if s == "" {
		return
	}
<<<<<<< HEAD

	c.Walk(func(s string, v any) bool {
		n := v.(*contentNode)
		if filter != nil && filter(s, n) {
			return false
=======
	for _, n := range cfg.taxonomyConfig.views {
		if strings.HasPrefix(s, n.plural) {
			return n
		}
	}

	return
}

var (
	_ identity.IdentityProvider          = (*contentNode)(nil)
	_ identity.DependencyManagerProvider = (*contentNode)(nil)
)

type contentNode struct {
	key string

	keyPartsInit sync.Once
	keyParts     []string

	p           *pageState
	pageContent pageparser.Result

	running bool

	// Additional traits for this node.
	traits interface{}

	// Tracks dependencies in server mode.
	idmInit sync.Once
	idm     identity.Manager
}

func (n *contentNode) IdentifierBase() interface{} {
	return n.key
}

func (b *contentNode) GetIdentity() identity.Identity {
	return b
}

func (b *contentNode) GetDependencyManager() identity.Manager {
	b.idmInit.Do(func() {
		// TODO1
		if true || b.running {
			b.idm = identity.NewManager(b)
		} else {
			b.idm = identity.NopManager
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}
	})
	return b.idm
}

func (b *contentNode) GetContainerNode() *contentNode {
	return b
}

func (b *contentNode) HasFi() bool {
	_, ok := b.traits.(hugofs.FileInfoProvider)
	return ok
}

func (b *contentNode) FileInfo() hugofs.FileMetaInfo {
	fip, ok := b.traits.(hugofs.FileInfoProvider)
	if !ok {
		return nil
	}
	return fip.FileInfo()
}

func (b *contentNode) Key() string {
	return b.key
}

<<<<<<< HEAD
func (c contentTrees) Walk(fn contentTreeNodeCallback) {
	for _, tree := range c {
		tree.Walk(func(s string, v any) bool {
			n := v.(*contentNode)
			return fn(s, n)
		})
	}
}

func (c contentTrees) WalkPrefix(prefix string, fn contentTreeNodeCallback) {
	for _, tree := range c {
		tree.WalkPrefix(prefix, func(s string, v any) bool {
			n := v.(*contentNode)
			return fn(s, n)
		})
	}
}

// WalkBelow walks the tree below the given prefix, i.e. it skips the
// node with the given prefix as key.
func (c *contentTree) WalkBelow(prefix string, fn radix.WalkFn) {
	c.Tree.WalkPrefix(prefix, func(s string, v any) bool {
		if s == prefix {
			return false
=======
func (b *contentNode) KeyParts() []string {
	b.keyPartsInit.Do(func() {
		if b.key != "" {
			b.keyParts = paths.FieldsSlash(b.key)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}
	})
	return b.keyParts
}

<<<<<<< HEAD
func (c *contentTree) getMatch(matches func(b *contentNode) bool) string {
	var match string
	c.Walk(func(s string, v any) bool {
		n, ok := v.(*contentNode)
		if !ok {
			return false
		}
=======
func (b *contentNode) GetNode() *contentNode {
	return b
}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

func (b *contentNode) IsStandalone() bool {
	_, ok := b.traits.(kindOutputFormat)
	return ok
}

// IsView returns whether this is a view node (a taxonomy or a term).
func (b *contentNode) IsView() bool {
	_, ok := b.traits.(viewInfoTrait)
	return ok
}

// isCascadingEdit parses any front matter and returns whether it has a cascade section and
// if that has changed.
func (n *contentNode) isCascadingEdit() bool {
	if n.p == nil {
		return false
	}
	fi := n.FileInfo()
	if fi == nil {
		return false
	}
	f, err := fi.Meta().Open()
	if err != nil {
		// File may have been removed, assume a cascading edit.
		// Some false positives are OK.
		return true
	}

	pf, err := pageparser.ParseFrontMatterAndContent(f)
	f.Close()
	if err != nil {
		return true
	}

	if n.p == nil || n.p.bucket == nil {
		return false
	}

	maps.PrepareParams(pf.FrontMatter)
	cascade1, ok := pf.FrontMatter["cascade"]
	hasCascade := n.p.bucket.cascade != nil && len(n.p.bucket.cascade) > 0
	if !ok {
		return hasCascade
	}

	if !hasCascade {
		return true
	}

	for _, v := range n.p.bucket.cascade {
		if !reflect.DeepEqual(cascade1, v) {
			return true
		}
<<<<<<< HEAD

		return false
	})

	return match
}

func (c *contentTree) hasBelow(s1 string) bool {
	var t bool
	c.WalkBelow(s1, func(s2 string, v any) bool {
		t = true
		return true
	})
	return t
}

func (c *contentTree) printKeys() {
	c.Walk(func(s string, v any) bool {
		fmt.Println(s)
		return false
	})
}

func (c *contentTree) printKeysPrefix(prefix string) {
	c.WalkPrefix(prefix, func(s string, v any) bool {
		fmt.Println(s)
		return false
	})
}

// contentTreeRef points to a node in the given tree.
type contentTreeRef struct {
	m   *pageMap
	t   *contentTree
	n   *contentNode
	key string
}

func (c *contentTreeRef) getCurrentSection() (string, *contentNode) {
	if c.isSection() {
		return c.key, c.n
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	}

	return false
}

type contentNodeInfo struct {
	branch     *contentBranchNode
	isBranch   bool
	isResource bool
}

func (info *contentNodeInfo) SectionsEntries() []string {
	return info.branch.n.KeyParts()
}

// TDOO1 somehow document that this will now return a leading slash, "" for home page.
func (info *contentNodeInfo) SectionsPath() string {
	k := info.branch.n.Key()
	if k == "" {
		// TODO1 consider this.
		return "/"
	}
	return k
}

type contentNodeInfoProvider interface {
	SectionsEntries() []string
	SectionsPath() string
}

type contentNodeProvider interface {
	contentGetNodeProvider
	types.Identifier
}

type contentTreeNodeCallback func(s string, n *contentNode) bool

type contentTreeNodeCallbackNew func(node contentNodeProvider) bool

type contentTreeRefProvider interface {
	contentGetBranchProvider
	contentGetContainerNodeProvider
	contentNodeInfoProvider
	contentNodeProvider
}

type fileInfoHolder struct {
	fi hugofs.FileMetaInfo
}

func (f fileInfoHolder) FileInfo() hugofs.FileMetaInfo {
	return f.fi
}

type kindOutputFormat struct {
	kind   string
	output output.Format
}

func (k kindOutputFormat) Kind() string {
	return k.kind
}

func (k kindOutputFormat) OutputFormat() output.Format {
	return k.output
}

type kindOutputFormatTrait interface {
	Kind() string
	OutputFormat() output.Format
}

// bookmark3
func (m *pageMap) AddFilesBundle(header hugofs.FileMetaInfo, resources ...hugofs.FileMetaInfo) error {
	var (
		n        *contentNode
		err      error
		pageTree *contentBranchNode
		pathInfo = header.Meta().PathInfo
	)

	if !pathInfo.IsBranchBundle() && m.cfg.pageDisabled {
		return nil
	}

	if pathInfo.IsBranchBundle() {
		// Apply some metadata if it's a taxonomy node.
		if tc := m.cfg.getTaxonomyConfig(pathInfo.Base()); !tc.IsZero() {
			term := strings.TrimPrefix(strings.TrimPrefix(pathInfo.Base(), "/"+tc.plural), "/")

			n, err = m.NewContentNode(
				viewInfoFileInfoHolder{
					&contentBundleViewInfo{
						name: tc,
						term: term,
					},
					fileInfoHolder{fi: header},
				},
				pathInfo.Base(),
			)
			if err != nil {
				return err
			}

<<<<<<< HEAD
	return pas
}

func (c *contentTreeRef) getPagesAndSections() page.Pages {
	var pas page.Pages

	query := pageMapQuery{
		Filter: c.n.p.m.getListFilter(true),
		Prefix: c.key,
	}

	c.m.collectPagesAndSections(query, func(c *contentNode) {
		pas = append(pas, c.p)
	})

	page.SortByDefault(pas)

	return pas
}

func (c *contentTreeRef) getSections() page.Pages {
	var pas page.Pages

	query := pageMapQuery{
		Filter: c.n.p.m.getListFilter(true),
		Prefix: c.key,
	}

	c.m.collectSections(query, func(c *contentNode) {
		pas = append(pas, c.p)
	})

	page.SortByDefault(pas)

	return pas
}

type contentTreeReverseIndex struct {
	t []*contentTree
	*contentTreeReverseIndexMap
}

type contentTreeReverseIndexMap struct {
	m      map[any]*contentNode
	init   sync.Once
	initFn func(*contentTree, map[any]*contentNode)
}

func (c *contentTreeReverseIndex) Reset() {
	c.contentTreeReverseIndexMap = &contentTreeReverseIndexMap{
		initFn: c.initFn,
	}
}

func (c *contentTreeReverseIndex) Get(key any) *contentNode {
	c.init.Do(func() {
		c.m = make(map[any]*contentNode)
		for _, tree := range c.t {
			c.initFn(tree, c.m)
=======
		} else {
			n, err = m.NewContentNode(
				fileInfoHolder{fi: header},
				pathInfo.Base(),
			)
			if err != nil {
				return err
			}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}

		pageTree = m.InsertBranch(n)

		for _, r := range resources {
			n, err := m.NewContentNode(
				fileInfoHolder{fi: r},
				r.Meta().PathInfo.Base(),
			)
			if err != nil {
				return err
			}
			pageTree.resources.nodes.Insert(n.key, n)
		}

		return nil
	}

	n, err = m.NewContentNode(
		fileInfoHolder{fi: header},
		pathInfo.Base(),
	)
	if err != nil {
		return err
	}

	// A regular page. Attach it to its section.
	var created bool
	_, pageTree, created, err = m.getOrCreateSection(n)
	if err != nil {
		return err
	}

	if created {
		// This means there are most likely no content file for this
		// section.
		// Apply some default metadata to the node.
		sectionName := helpers.FirstUpper(pathInfo.Section())
		var title string
		if m.s.Cfg.GetBool("pluralizeListTitles") {
			title = flect.Pluralize(sectionName)
		} else {
			title = sectionName
		}
		pageTree.defaultTitle = title
	}

	pageTree.InsertPage(n.key, n)

	for _, r := range resources {
		n, err := m.NewContentNode(
			fileInfoHolder{fi: r},
			r.Meta().PathInfo.Base(),
		)
		if err != nil {
			return err
		}
		pageTree.pageResources.nodes.Insert(n.key, n)
	}

	return nil
}

func (m *pageMap) getOrCreateSection(n *contentNode) (string, *contentBranchNode, bool, error) {
	level := strings.Count(n.key, "/")

	k, pageTree := m.GetBranchContainer(n.key)

	mustCreate := false

	if pageTree == nil {
		mustCreate = true
	} else if level > 1 && k == "" {
		// We found the home section, but this page needs to be placed in
		// the root, e.g. "/blog", section.
		mustCreate = true
	} else {
		return k, pageTree, false, nil
	}

	if !mustCreate {
		return k, pageTree, false, nil
	}

	var keyParts []string
	if level > 1 {
		keyParts = n.KeyParts()[:1]
	}
	var err error
	n, err = m.NewContentNode(nil, keyParts...)
	if err != nil {
		return k, pageTree, false, err
	}

	if k != "" {
		// Make sure we always have the root/home node.
		if m.GetBranch("") == nil {
			m.InsertBranch(&contentNode{})
		}
	}

	pageTree = m.InsertBranch(n)
	return k, pageTree, true, nil
}

type stringKindProvider string

func (k stringKindProvider) Kind() string {
	return string(k)
}

type viewInfoFileInfoHolder struct {
	viewInfoTrait
	hugofs.FileInfoProvider
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
