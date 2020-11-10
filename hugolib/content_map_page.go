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
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/parser/pageparser"

	"github.com/gohugoio/hugo/resources/page/pagekinds"
	"github.com/gohugoio/hugo/resources/page/siteidentities"

	"github.com/gohugoio/hugo/output"

	"github.com/gohugoio/hugo/common/types"
	"github.com/spf13/cast"

	"github.com/gohugoio/hugo/common/maps"

	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"

	"github.com/gohugoio/hugo/common/para"
)

func newPageMap(i int, s *Site) *pageMap {
	var m *pageMap

	taxonomiesConfig := s.siteCfg.taxonomiesConfig.Values()
	createBranchNode := func(elem ...string) (*contentNode, error) {
		var traits interface{}
		key := cleanTreeKey(path.Join(elem...))
		if view, found := taxonomiesConfig.viewsByTreeKey[key]; found {
			traits = &contentBundleViewInfo{
				name: view,
			}
		}
		return m.NewContentNode(traits, key)
	}

	m = &pageMap{
		cfg: contentMapConfig{
			lang:                 s.Lang(),
			taxonomyConfig:       taxonomiesConfig,
			taxonomyDisabled:     !s.isEnabled(pagekinds.Taxonomy),
			taxonomyTermDisabled: !s.isEnabled(pagekinds.Term),
			pageDisabled:         !s.isEnabled(pagekinds.Page),
		},
		i:         i,
		s:         s,
		branchMap: newBranchMap(createBranchNode),
	}

	m.nav = pageMapNavigation{m: m}

	m.pageReverseIndex = &contentTreeReverseIndex{
		initFn: func(rm map[interface{}]*contentNode) {
			m.WalkPagesAllPrefixSection("", nil, contentTreeNoListAlwaysFilter, func(np contentNodeProvider) bool {
				n := np.GetNode()
				fi := n.FileInfo()

				addKey := func(k string) {
					existing, found := rm[k]
					if found && existing != ambiguousContentNode {
						rm[k] = ambiguousContentNode
					} else if !found {
						rm[k] = n
					}
				}
				if fi != nil {
					addKey(fi.Meta().PathInfo.BaseNameNoIdentifier())
				} else {
					// TODO1 needed?
					addKey(path.Base(n.Key()))
				}

				return false
			})
		},
		contentTreeReverseIndexMap: &contentTreeReverseIndexMap{},
	}

	return m
}

func newPageMaps(h *HugoSites) *pageMaps {
	mps := make([]*pageMap, len(h.Sites))
	for i, s := range h.Sites {
		mps[i] = s.pageMap
	}
	return &pageMaps{
		workers: para.New(1), // TODO1 h.numWorkers),
		pmaps:   mps,
	}
}

type contentTreeReverseIndex struct {
	initFn func(rm map[interface{}]*contentNode)
	*contentTreeReverseIndexMap
}

func (c *contentTreeReverseIndex) Reset() {
	c.contentTreeReverseIndexMap = &contentTreeReverseIndexMap{
		m: make(map[interface{}]*contentNode),
	}
}

<<<<<<< HEAD
func (m *pageMap) createMissingTaxonomyNodes() error {
	if m.cfg.taxonomyDisabled {
		return nil
	}
	m.taxonomyEntries.Walk(func(s string, v any) bool {
		n := v.(*contentNode)
		vi := n.viewInfo
		k := cleanSectionTreeKey(vi.name.plural + "/" + vi.termKey)

		if _, found := m.taxonomies.Get(k); !found {
			vic := &contentBundleViewInfo{
				name:       vi.name,
				termKey:    vi.termKey,
				termOrigin: vi.termOrigin,
			}
			m.taxonomies.Insert(k, &contentNode{viewInfo: vic})
		}
		return false
=======
func (c *contentTreeReverseIndex) Get(key interface{}) *contentNode {
	c.init.Do(func() {
		c.m = make(map[interface{}]*contentNode)
		c.initFn(c.contentTreeReverseIndexMap.m)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	})
	return c.m[key]
}

type contentTreeReverseIndexMap struct {
	init sync.Once
	m    map[interface{}]*contentNode
}

type ordinalWeight struct {
	ordinal int
	weight  int
}

type pageMap struct {
	cfg contentMapConfig
	i   int
	s   *Site

	nav pageMapNavigation

	*branchMap

	// A reverse index used as a fallback in GetPage for short references.
	pageReverseIndex *contentTreeReverseIndex
}

// TODO1 use this
func (m *pageMap) MustContentNode(traits interface{}, elem ...string) *contentNode {
	n, err := m.NewContentNode(traits, elem...)
	if err != nil {
		panic(err)
	}
	return n
}

func (m *pageMap) NewContentNode(traits interface{}, elem ...string) (*contentNode, error) {
	switch v := traits.(type) {
	case string:
		panic("traits can not be a string")
	case *contentBundleViewInfo:
		if v == nil {
			panic("traits can not be nil")
		}
	}

	var pth string
	if len(elem) > 0 {
		pth = elem[0]
		if len(elem) > 1 {
			pth = path.Join(elem...)
		}
	}

	key := cleanTreeKey(pth)

	n := &contentNode{
		key:     key,
		traits:  traits,
		running: m.s.running(),
	}

	if fi := n.FileInfo(); fi != nil {
		r, err := fi.Meta().Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()

		n.pageContent, err = pageparser.Parse(
			r,
			pageparser.Config{EnableEmoji: m.s.siteCfg.enableEmoji},
		)

<<<<<<< HEAD
	if n.fi.Meta().IsRootFile {
		// Make sure that the bundle/section we start walking from is always
		// rendered.
		// This is only relevant in server fast render mode.
		ps.forceRender = true
	}

	n.p = ps
	if ps.IsNode() {
		ps.bucket = newPageBucket(ps)
	}

	gi, err := s.h.gitInfoForPage(ps)
	if err != nil {
		return nil, fmt.Errorf("failed to load Git data: %w", err)
	}
	ps.gitInfo = gi

	owners, err := s.h.codeownersForPage(ps)
	if err != nil {
		return nil, fmt.Errorf("failed to load CODEOWNERS: %w", err)
	}
	ps.codeowners = owners

	r, err := content()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	parseResult, err := pageparser.Parse(
		r,
		pageparser.Config{EnableEmoji: s.siteCfg.enableEmoji},
	)
	if err != nil {
		return nil, err
	}

	ps.pageContent = pageContent{
		source: rawPageContent{
			parsed:         parseResult,
			posMainContent: -1,
			posSummaryEnd:  -1,
			posBodyStart:   -1,
		},
	}

	if err := ps.mapContent(parentBucket, metaProvider); err != nil {
		return nil, ps.wrapError(err)
	}

	if err := metaProvider.applyDefaultValues(n); err != nil {
		return nil, err
	}

	ps.init.Add(func() (any, error) {
		pp, err := newPagePaths(s, ps, metaProvider)
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		if err != nil {
			return nil, err
		}

	}

	return n, nil
}

func (m *pageMap) AssemblePages(changeTracker *whatChanged) error {
	isRebuild := m.cfg.isRebuild
	if isRebuild {
		siteLastMod := m.s.lastmod
		defer func() {
			if siteLastMod != m.s.lastmod {
				changeTracker.Add(siteidentities.Stats)
			}
		}()
	}

	var theErr error

	if isRebuild {
		m.WalkTaxonomyTerms(func(s string, b *contentBranchNode) bool {
			b.refs = make(map[interface{}]ordinalWeight)
			return false
		})
	}

<<<<<<< HEAD
func (m *pageMap) createSiteTaxonomies() error {
	m.s.taxonomies = make(TaxonomyList)
	var walkErr error
	m.taxonomies.Walk(func(s string, v any) bool {
		n := v.(*contentNode)
		t := n.viewInfo
=======
	// Holds references to sections or pages to exlude from the build
	// because front matter dictated it (e.g. a draft).
	var (
		sectionsToDelete = make(map[string]bool)
		pagesToDelete    []contentTreeRefProvider
	)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

	// handleBranch creates the Page in np if not already set.
	handleBranch := func(np contentNodeProvider) bool {
		n := np.GetNode()
		s := np.Key()
		tref := np.(contentTreeRefProvider)
		branch := tref.GetBranch()
		var err error

		if n.p != nil {
			if n.p.buildState > 0 {
				n.p, err = m.s.newPageFromTreeRef(tref, n.p.pageContent)
				if err != nil {
					theErr = err
					return true
				}
			}
			// Page already set, nothing more to do.
			if n.p.IsHome() {
				m.s.home = n.p
			}
			return false
		} else {
<<<<<<< HEAD
			taxonomy := m.s.taxonomies[viewName.plural]
			if taxonomy == nil {
				walkErr = fmt.Errorf("missing taxonomy: %s", viewName.plural)
				return true
			}
			m.taxonomyEntries.WalkPrefix(s, func(ss string, v any) bool {
				b2 := v.(*contentNode)
				info := b2.viewInfo
				taxonomy.add(info.termKey, page.NewWeightedPage(info.weight, info.ref.p, n.p))
=======
			n.p, err = m.s.newPageFromTreeRef(tref, zeroContent)
			if err != nil {
				theErr = err
				return true
			}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

		}

		if n.p.IsHome() {
			m.s.home = n.p
		}

		if !m.s.shouldBuild(n.p) {
			sectionsToDelete[s] = true
			if s == "" {
				// Home page, abort.
				return true
			}
		}

		branch.n.p.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.userProvided)

		return false
	}

	// handlePage creates the page in np.
	handlePage := func(np contentNodeProvider) bool {
		n := np.GetNode()
		tref2 := np.(contentTreeRefProvider)
		branch := np.(contentGetBranchProvider).GetBranch()

		if n.p == nil {
			var err error
			n.p, err = m.s.newPageFromTreeRef(tref2, zeroContent)
			if err != nil {
				theErr = err
				return true
			}

		} else if n.p.buildState > 0 {
			var err error
			n.p, err = m.s.newPageFromTreeRef(tref2, n.p.pageContent)
			if err != nil {
				theErr = err
				return true
			}
		} else {
			return false
		}

		if !m.s.shouldBuild(n.p) {
			pagesToDelete = append(pagesToDelete, tref2)
			return false
		}

		branch.n.p.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.userProvided)

		return false
	}

	// handleResource creates the resources in np.
	handleResource := func(np contentNodeProvider) bool {
		n := np.GetNode()

		if n.p != nil {
			return false
		}

		branch := np.(contentGetBranchProvider).GetBranch()
		owner := np.(contentGetContainerNodeProvider).GetContainerNode()
		tref2 := np.(contentTreeRefProvider)

		if owner.p == nil {
			panic("invalid state, page not set on resource owner")
		}

		p := owner.p
		meta := n.FileInfo().Meta()
		classifier := meta.PathInfo.BundleType()
		var r resource.Resource
		switch classifier {
		case paths.PathTypeContentResource:
			var rp *pageState
			var err error
			rp, err = m.s.newPageFromTreeRef(tref2, zeroContent)
			if err != nil {
				theErr = err
				return true
			}

			rp.m.resourcePath = strings.TrimPrefix(rp.Path(), p.Path())[1:]
			r = rp
		case paths.PathTypeFile:
			var err error
			r, err = branch.newResource(n, p)
			if err != nil {
				theErr = err
				return true
			}
		default:
			panic(fmt.Sprintf("invalid classifier: %d", classifier))
		}

		p.resources = append(p.resources, r)

		return false
	}

	// Create home page if it does not exist.
	hn := m.GetBranch("")
	if hn == nil {
		hn = m.InsertBranch(&contentNode{})
	}

	// Create the fixed output pages if not already there.
	addStandalone := func(s, kind string, f output.Format) {
		if !m.s.isEnabled(kind) {
			return
		}

		if !hn.pages.Has(s) {
			hn.InsertPage(s, &contentNode{key: s, traits: kindOutputFormat{kind: kind, output: f}})
		}
	}

	addStandalone("/404", pagekinds.Status404, output.HTTPStatusHTMLFormat)

	if m.i == 0 || m.s.h.IsMultihost() {
		addStandalone("/robots", pagekinds.RobotsTXT, output.RobotsTxtFormat)
	}

	// TODO1 coordinate
	addStandalone("/sitemap", pagekinds.Sitemap, output.SitemapFormat)

	if !m.cfg.taxonomyDisabled {
		// Create the top level taxonomy nodes if they don't exist.
		for _, viewName := range m.cfg.taxonomyConfig.views {
			key := viewName.pluralTreeKey
			if sectionsToDelete[key] {
				continue
			}
			taxonomy := m.GetBranch(key)
			if taxonomy == nil {
				n, err := m.NewContentNode(
					&contentBundleViewInfo{
						name: viewName,
					},
					viewName.plural,
				)
				if err != nil {
					return err
				}
				m.InsertRootAndBranch(n)
			}
		}
	}

	// First pass: Create Pages and Resources.
	m.Walk(
		branchMapQuery{
			Deep:    true, // Need the branch tree
			Exclude: func(s string, n *contentNode) bool { return false },
			Branch: branchMapQueryCallBacks{
				Key:      newBranchMapQueryKey("", true),
				Page:     handleBranch,
				Resource: handleResource,
			},
			Leaf: branchMapQueryCallBacks{
				Page:     handlePage,
				Resource: handleResource,
			},
		})

	if theErr != nil {
		return theErr
	}

	// Delete pages and sections marked for deletion.
	for _, p := range pagesToDelete {
		p.GetBranch().pages.nodes.Delete(p.Key())
		p.GetBranch().pageResources.nodes.Delete(p.Key() + "/")
		if !p.GetBranch().n.HasFi() && p.GetBranch().pages.nodes.Len() == 0 {
			// Delete orphan section.
			sectionsToDelete[p.GetBranch().n.key] = true
		}
	}

	for s := range sectionsToDelete {
		m.branches.Delete(s)
		m.branches.DeletePrefix(s + "/")
	}

	// Attach pages to views.
	if !m.cfg.taxonomyDisabled {
		handleTaxonomyEntries := func(np contentNodeProvider) bool {
			if m.cfg.taxonomyTermDisabled {
				return false
			}

			for _, viewName := range m.cfg.taxonomyConfig.views {
				if sectionsToDelete[viewName.pluralTreeKey] {
					continue
				}

				taxonomy := m.GetBranch(viewName.pluralTreeKey)

				n := np.GetNode()
				s := np.Key()

				if n.p == nil {
					panic("page is nil: " + s)
				}
				vals := types.ToStringSlicePreserveString(getParam(n.p, viewName.plural, false))
				if vals == nil {
					continue
				}

				w := getParamToLower(n.p, viewName.plural+"_weight")
				weight, err := cast.ToIntE(w)
				if err != nil {
					m.s.Log.Errorf("Unable to convert taxonomy weight %#v to int for %q", w, n.p.Path())
					// weight will equal zero, so let the flow continue
				}

				for i, v := range vals {
					keyParts := append(viewName.pluralParts(), v)
					key := cleanTreeKey(keyParts...)

					// It may have been added with the content files
					termBranch := m.GetBranch(key)

					if termBranch == nil {
						vic := &contentBundleViewInfo{
							name: viewName,
							term: v,
						}

						n, err := m.NewContentNode(vic, key)
						if err != nil {
							panic(err)
						}

						_, termBranch, err = m.InsertRootAndBranch(n)
						if err != nil {
							panic(err)
						}

						treeRef := m.newNodeProviderPage(key, n, taxonomy, termBranch, true).(contentTreeRefProvider)
						n.p, err = m.s.newPageFromTreeRef(treeRef, zeroContent)
						if err != nil {
							return true
						}
					}

					termBranch.refs[n.p] = ordinalWeight{ordinal: i, weight: weight}
					termBranch.n.p.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.userProvided)
				}

			}
			return false
		}

		m.Walk(
			branchMapQuery{
				Branch: branchMapQueryCallBacks{
					Key:  newBranchMapQueryKey("", true),
					Page: handleTaxonomyEntries,
				},
				Leaf: branchMapQueryCallBacks{
					Page: handleTaxonomyEntries,
				},
			},
		)

	}

	// Finally, collect aggregate values from the content tree.
	var (
		siteLastChanged     time.Time
		rootSectionCounters map[string]int
	)

	_, mainSectionsSet := m.s.s.Info.Params()["mainsections"]
	if !mainSectionsSet {
		rootSectionCounters = make(map[string]int)
	}

	handleAggregatedValues := func(np contentNodeProvider) bool {
		n := np.GetNode()
		s := np.Key()
		branch := np.(contentGetBranchProvider).GetBranch()
		owner := np.(contentGetContainerBranchProvider).GetContainerBranch()

		if s == "" {
			if n.p.m.calculated.Lastmod().After(siteLastChanged) {
				siteLastChanged = n.p.m.calculated.Lastmod()
			}
			return false
		}

		if rootSectionCounters != nil {
			// Keep track of the page count per root section
			rootSection := s[1:]
			firstSlash := strings.Index(rootSection, "/")
			if firstSlash != -1 {
				rootSection = rootSection[:firstSlash]
			}
			rootSectionCounters[rootSection] += branch.pages.nodes.Len()
		}

		parent := owner.n.p
		for parent != nil {
			parent.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.calculated)

			if n.p.m.calculated.Lastmod().After(siteLastChanged) {
				siteLastChanged = n.p.m.calculated.Lastmod()
			}

			if parent.bucket == nil {
				panic("bucket not set")
			}

			if parent.bucket.parent == nil {
				break
			}

			parent = parent.bucket.parent.self
		}

		return false
	}

	m.Walk(
		branchMapQuery{
			Deep:         true, // Need the branch relations
			OnlyBranches: true,
			Branch: branchMapQueryCallBacks{
				Key:  newBranchMapQueryKey("", true),
				Page: handleAggregatedValues,
			},
		},
	)

	m.s.lastmod = siteLastChanged
	if rootSectionCounters != nil {
		var mainSection string
		var mainSectionCount int

		for k, v := range rootSectionCounters {
			if v > mainSectionCount {
				mainSection = k
				mainSectionCount = v
			}
		}

		mainSections := []string{mainSection}
		m.s.s.Info.Params()["mainSections"] = mainSections
		m.s.s.Info.Params()["mainsections"] = mainSections

	}

	return nil
}

func (m *pageMap) CreateListAllPages() page.Pages {
	pages := make(page.Pages, 0)

	m.WalkPagesAllPrefixSection("", nil, contentTreeNoListAlwaysFilter, func(np contentNodeProvider) bool {
		n := np.GetNode()
		if n.p == nil {
			panic(fmt.Sprintf("BUG: page not set for %q", np.Key()))
		}
		pages = append(pages, n.p)
		return false
	})

	page.SortByDefault(pages)
	return pages
}

<<<<<<< HEAD
func (m *pageMap) assemblePages() error {
	m.taxonomyEntries.DeletePrefix("/")

	if err := m.assembleSections(); err != nil {
		return err
	}

	var err error

	if err != nil {
		return err
	}

	m.pages.Walk(func(s string, v any) bool {
		n := v.(*contentNode)

		var shouldBuild bool

		defer func() {
			// Make sure we always rebuild the view cache.
			if shouldBuild && err == nil && n.p != nil {
				m.attachPageToViews(s, n)
=======
func (m *pageMap) CreateSiteTaxonomies() error {
	m.s.taxonomies = make(TaxonomyList)
	for _, viewName := range m.cfg.taxonomyConfig.views {
		taxonomy := make(Taxonomy)
		m.s.taxonomies[viewName.plural] = taxonomy
		prefix := viewName.pluralTreeKey + "/"
		m.WalkBranchesPrefix(prefix, func(s string, b *contentBranchNode) bool {
			termKey := strings.TrimPrefix(s, prefix)
			for k, v := range b.refs {
				taxonomy.add(termKey, page.NewWeightedPage(v.weight, k.(*pageState), b.n.p))
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
			}

			return false
<<<<<<< HEAD
		}

		var parent *contentNode
		var parentBucket *pagesMapBucket

		_, parent = m.getSection(s)
		if parent == nil {
			panic(fmt.Sprintf("BUG: parent not set for %q", s))
		}
		parentBucket = parent.p.bucket

		n.p, err = m.newPageFromContentNode(n, parentBucket, nil)
		if err != nil {
			return true
		}

		shouldBuild = !(n.p.Kind() == page.KindPage && m.cfg.pageDisabled) && m.s.shouldBuild(n.p)
		if !shouldBuild {
			m.deletePage(s)
			return false
		}

		n.p.treeRef = &contentTreeRef{
			m:   m,
			t:   m.pages,
			n:   n,
			key: s,
		}

		if err = m.assembleResources(s, n.p, parentBucket); err != nil {
			return true
		}

		return false
	})

	m.deleteOrphanSections()

	return err
}

func (m *pageMap) assembleResources(s string, p *pageState, parentBucket *pagesMapBucket) error {
	var err error

	m.resources.WalkPrefix(s, func(s string, v any) bool {
		n := v.(*contentNode)
		meta := n.fi.Meta()
		classifier := meta.Classifier
		var r resource.Resource
		switch classifier {
		case files.ContentClassContent:
			var rp *pageState
			rp, err = m.newPageFromContentNode(n, parentBucket, p)
			if err != nil {
				return true
			}
			rp.m.resourcePath = filepath.ToSlash(strings.TrimPrefix(rp.File().Path(), p.File().Dir()))
			r = rp

		case files.ContentClassFile:
			r, err = m.newResource(n.fi, p)
			if err != nil {
				return true
			}
		default:
			panic(fmt.Sprintf("invalid classifier: %q", classifier))
		}

		p.resources = append(p.resources, r)
		return false
	})

	return err
}

func (m *pageMap) assembleSections() error {
	var sectionsToDelete []string
	var err error

	m.sections.Walk(func(s string, v any) bool {
		n := v.(*contentNode)
		var shouldBuild bool

		defer func() {
			// Make sure we always rebuild the view cache.
			if shouldBuild && err == nil && n.p != nil {
				m.attachPageToViews(s, n)
				if n.p.IsHome() {
					m.s.home = n.p
				}
			}
		}()

		sections := m.splitKey(s)

		if n.p != nil {
			if n.p.IsHome() {
				m.s.home = n.p
			}
			shouldBuild = true
			return false
		}

		var parent *contentNode
		var parentBucket *pagesMapBucket

		if s != "/" {
			_, parent = m.getSection(s)
			if parent == nil || parent.p == nil {
				panic(fmt.Sprintf("BUG: parent not set for %q", s))
			}
		}

		if parent != nil {
			parentBucket = parent.p.bucket
		} else if s == "/" {
			parentBucket = m.s.siteBucket
		}

		kind := page.KindSection
		if s == "/" {
			kind = page.KindHome
		}

		if n.fi != nil {
			n.p, err = m.newPageFromContentNode(n, parentBucket, nil)
			if err != nil {
				return true
			}
		} else {
			n.p = m.s.newPage(n, parentBucket, kind, "", sections...)
		}

		shouldBuild = m.s.shouldBuild(n.p)
		if !shouldBuild {
			sectionsToDelete = append(sectionsToDelete, s)
			return false
		}

		n.p.treeRef = &contentTreeRef{
			m:   m,
			t:   m.sections,
			n:   n,
			key: s,
		}

		if err = m.assembleResources(s+cmLeafSeparator, n.p, parentBucket); err != nil {
			return true
		}

		return false
	})

	for _, s := range sectionsToDelete {
		m.deleteSectionByPath(s)
	}

	return err
}

func (m *pageMap) assembleTaxonomies() error {
	var taxonomiesToDelete []string
	var err error

	m.taxonomies.Walk(func(s string, v any) bool {
		n := v.(*contentNode)

		if n.p != nil {
			return false
=======
		})
	}

	for _, taxonomy := range m.s.taxonomies {
		for _, v := range taxonomy {
			v.Sort()
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}
	}

	return nil
}

func (m *pageMap) WalkTaxonomyTerms(fn func(s string, b *contentBranchNode) bool) {
	for _, viewName := range m.cfg.taxonomyConfig.views {
		m.WalkBranchesPrefix(viewName.pluralTreeKey+"/", func(s string, b *contentBranchNode) bool {
			return fn(s, b)
		})
	}
}

func (m *pageMap) WithEveryBundleNode(fn func(n *contentNode) bool) error {
	callbackPage := func(np contentNodeProvider) bool {
		return fn(np.GetNode())
	}

	callbackResource := func(np contentNodeProvider) bool {
		return fn(np.GetNode())
	}

	q := branchMapQuery{
		Exclude: func(s string, n *contentNode) bool { return n.p == nil },
		Branch: branchMapQueryCallBacks{
			Key:      newBranchMapQueryKey("", true),
			Page:     callbackPage,
			Resource: callbackResource,
		},
		Leaf: branchMapQueryCallBacks{
			Page:     callbackPage,
			Resource: callbackResource,
		},
	}

	return m.Walk(q)
}

// WithEveryBundlePage applies fn to every Page, including those bundled inside
// leaf bundles.
func (m *pageMap) WithEveryBundlePage(fn func(p *pageState) bool) error {
	return m.WithEveryBundleNode(func(n *contentNode) bool {
		if n.p != nil {
			return fn(n.p)
		}
		return false
	})
}

func (m *pageMap) Debug(prefix string, w io.Writer) {
	m.branchMap.debug(prefix, w)

	/*fmt.Fprintln(w)
	for k := range m.pageReverseIndex.m {
		fmt.Fprintln(w, k)
	}*/
}

type pageMapNavigation struct {
	m *pageMap
}

func (nav pageMapNavigation) getPagesAndSections(in contentNodeProvider) page.Pages {
	if in == nil {
		return nil
	}

	var pas page.Pages

	nav.m.WalkPagesPrefixSectionNoRecurse(
		in.Key()+"/",
		noTaxonomiesFilter,
		in.GetNode().p.m.getListFilter(true),
		func(n contentNodeProvider) bool {
			pas = append(pas, n.GetNode().p)
			return false
		},
	)

	page.SortByDefault(pas)

	return pas
}

func (nav pageMapNavigation) getRegularPages(in contentNodeProvider) page.Pages {
	if in == nil {
		return nil
	}

	var pas page.Pages

	q := branchMapQuery{
		Exclude: in.GetNode().p.m.getListFilter(true),
		Branch: branchMapQueryCallBacks{
			Key: newBranchMapQueryKey(in.Key(), false),
		},
		Leaf: branchMapQueryCallBacks{
			Page: func(n contentNodeProvider) bool {
				pas = append(pas, n.GetNode().p)
				return false
			},
		},
	}

	nav.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}

func (nav pageMapNavigation) getRegularPagesRecursive(in contentNodeProvider) page.Pages {
	if in == nil {
		return nil
	}

	var pas page.Pages

	q := branchMapQuery{
		Exclude: in.GetNode().p.m.getListFilter(true),
		Branch: branchMapQueryCallBacks{
			Key: newBranchMapQueryKey(in.Key()+"/", true),
		},
		Leaf: branchMapQueryCallBacks{
			Page: func(n contentNodeProvider) bool {
				pas = append(pas, n.GetNode().p)
				return false
			},
		},
	}

	nav.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}

func (nav pageMapNavigation) getSections(in contentNodeProvider) page.Pages {
	if in == nil {
		return nil
	}
	var pas page.Pages

	q := branchMapQuery{
		NoRecurse:     true,
		Exclude:       in.GetNode().p.m.getListFilter(true),
		BranchExclude: noTaxonomiesFilter,
		Branch: branchMapQueryCallBacks{
			Key: newBranchMapQueryKey(in.Key()+"/", true),
			Page: func(n contentNodeProvider) bool {
				pas = append(pas, n.GetNode().p)
				return false
			},
		},
	}

	nav.m.Walk(q)

	page.SortByDefault(pas)

	return pas
}

type pageMaps struct {
	workers *para.Workers
	pmaps   []*pageMap
}

func (m *pageMaps) AssemblePages(changeTracker *whatChanged) error {
	return m.withMaps(func(runner para.Runner, pm *pageMap) error {
		if err := pm.AssemblePages(changeTracker); err != nil {
			return err
		}
		return nil
	})
}

// deleteSection deletes the entire section from s.
func (m *pageMaps) deleteSection(s string) {
	m.withMaps(func(runner para.Runner, pm *pageMap) error {
		pm.branches.Delete(s)
		pm.branches.DeletePrefix(s + "/")
		return nil
	})
}

func (m *pageMaps) walkBranchesPrefix(prefix string, fn func(s string, n *contentNode) bool) error {
	return m.withMaps(func(runner para.Runner, pm *pageMap) error {
		callbackPage := func(np contentNodeProvider) bool {
			return fn(np.Key(), np.GetNode())
		}

		q := branchMapQuery{
			OnlyBranches: true,
			Branch: branchMapQueryCallBacks{
				Key:  newBranchMapQueryKey(prefix, true),
				Page: callbackPage,
			},
		}

		return pm.Walk(q)
	})
}

func (m *pageMaps) walkBundles(fn func(n *contentNode) bool) error {
	return m.withMaps(func(runner para.Runner, pm *pageMap) error {
		return pm.WithEveryBundleNode(fn)
	})
}

func (m *pageMaps) withMaps(fn func(runner para.Runner, pm *pageMap) error) error {
	for _, pm := range m.pmaps {
		pm := pm
		if err := fn(nil, pm); err != nil {
			return err
		}
	}
	return nil
}

func (m *pageMaps) withMapsPara(fn func(runner para.Runner, pm *pageMap) error) error {
	g, _ := m.workers.Start(context.Background())
	for _, pm := range m.pmaps {
		pm := pm
		g.Run(func() error {
			return fn(g, pm)
		})
	}
	return g.Wait()
}

type pagesMapBucket struct {
	// Cascading front matter.
	cascade map[page.PageMatcher]maps.Params

	parent *pagesMapBucket // The parent bucket, nil if the home page.
	self   *pageState      // The branch node.

	*pagesMapBucketPages
}

func (b *pagesMapBucket) getPagesAndSections() page.Pages {
	if b == nil {
		return nil
	}

	b.pagesAndSectionsInit.Do(func() {
		b.pagesAndSections = b.self.s.pageMap.nav.getPagesAndSections(b.self.m.treeRef)
	})

	return b.pagesAndSections
}

func (b *pagesMapBucket) getPagesInTerm() page.Pages {
	if b == nil {
		return nil
	}

	b.pagesInTermInit.Do(func() {
		branch := b.self.m.treeRef.(contentGetBranchProvider).GetBranch()
		for k := range branch.refs {
			b.pagesInTerm = append(b.pagesInTerm, k.(*pageState))
		}

		page.SortByDefault(b.pagesInTerm)
	})

	return b.pagesInTerm
}

func (b *pagesMapBucket) getRegularPages() page.Pages {
	if b == nil {
		return nil
	}

	b.regularPagesInit.Do(func() {
		b.regularPages = b.self.s.pageMap.nav.getRegularPages(b.self.m.treeRef)
	})

	return b.regularPages
}

func (b *pagesMapBucket) getRegularPagesInTerm() page.Pages {
	if b == nil {
		return nil
	}

	b.regularPagesInTermInit.Do(func() {
		all := b.getPagesInTerm()

		for _, p := range all {
			if p.IsPage() {
				b.regularPagesInTerm = append(b.regularPagesInTerm, p)
			}
		}
	})

	return b.regularPagesInTerm
}

func (b *pagesMapBucket) getRegularPagesRecursive() page.Pages {
	if b == nil {
		return nil
	}

	b.regularPagesRecursiveInit.Do(func() {
		b.regularPagesRecursive = b.self.s.pageMap.nav.getRegularPagesRecursive(b.self.m.treeRef)
	})

	return b.regularPagesRecursive
}

func (b *pagesMapBucket) getSections() page.Pages {
	if b == nil {
		return nil
	}

	b.sectionsInit.Do(func() {
		b.sections = b.self.s.pageMap.nav.getSections(b.self.m.treeRef)
	})

	return b.sections
}

func (b *pagesMapBucket) getTaxonomies() page.Pages {
<<<<<<< HEAD
	b.sectionsInit.Do(func() {
		var pas page.Pages
		ref := b.owner.treeRef
		ref.m.collectTaxonomies(ref.key, func(c *contentNode) {
			pas = append(pas, c.p)
		})
		page.SortByDefault(pas)
		b.sections = pas
	})

	return b.sections
}

func (b *pagesMapBucket) getTaxonomyEntries() page.Pages {
	var pas page.Pages
	ref := b.owner.treeRef
	viewInfo := ref.n.viewInfo
	prefix := strings.ToLower("/" + viewInfo.name.plural + "/" + viewInfo.termKey + "/")
	ref.m.taxonomyEntries.WalkPrefix(prefix, func(s string, v any) bool {
		n := v.(*contentNode)
		pas = append(pas, n.viewInfo.ref.p)
		return false
	})
	page.SortByDefault(pas)
	return pas
}

type sectionAggregate struct {
	datesAll             resource.Dates
	datesSection         resource.Dates
	pageCount            int
	mainSection          string
	mainSectionPageCount int
}

type sectionAggregateHandler struct {
	sectionAggregate
	sectionPageCount int

	// Section
	b *contentNode
	s string
}

func (h *sectionAggregateHandler) String() string {
	return fmt.Sprintf("%s/%s - %d - %s", h.sectionAggregate.datesAll, h.sectionAggregate.datesSection, h.sectionPageCount, h.s)
}

func (h *sectionAggregateHandler) isRootSection() bool {
	return h.s != "/" && strings.Count(h.s, "/") == 2
}

func (h *sectionAggregateHandler) handleNested(v sectionWalkHandler) error {
	nested := v.(*sectionAggregateHandler)
	h.sectionPageCount += nested.pageCount
	h.pageCount += h.sectionPageCount
	h.datesAll.UpdateDateAndLastmodIfAfter(nested.datesAll)
	h.datesSection.UpdateDateAndLastmodIfAfter(nested.datesAll)
	return nil
}

func (h *sectionAggregateHandler) handlePage(s string, n *contentNode) error {
	h.sectionPageCount++

	var d resource.Dated
	if n.p != nil {
		d = n.p
	} else if n.viewInfo != nil && n.viewInfo.ref != nil {
		d = n.viewInfo.ref.p
	} else {
=======
	if b == nil {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		return nil
	}

	b.taxonomiesInit.Do(func() {
		ref := b.self.m.treeRef

<<<<<<< HEAD
func (h *sectionAggregateHandler) handleSectionPost() error {
	if h.sectionPageCount > h.mainSectionPageCount && h.isRootSection() {
		h.mainSectionPageCount = h.sectionPageCount
		h.mainSection = strings.TrimPrefix(h.s, "/")
	}

	if resource.IsZeroDates(h.b.p) {
		h.b.p.m.Dates = h.datesSection
	}

	h.datesSection = resource.Dates{}

	return nil
}

func (h *sectionAggregateHandler) handleSectionPre(s string, b *contentNode) error {
	h.s = s
	h.b = b
	h.sectionPageCount = 0
	h.datesAll.UpdateDateAndLastmodIfAfter(b.p)
	return nil
}

type sectionWalkHandler interface {
	handleNested(v sectionWalkHandler) error
	handlePage(s string, b *contentNode) error
	handleSectionPost() error
	handleSectionPre(s string, b *contentNode) error
}

type sectionWalker struct {
	err error
	m   *contentMap
}

func (w *sectionWalker) applyAggregates() *sectionAggregateHandler {
	return w.walkLevel("/", func() sectionWalkHandler {
		return &sectionAggregateHandler{}
	}).(*sectionAggregateHandler)
}

func (w *sectionWalker) walkLevel(prefix string, createVisitor func() sectionWalkHandler) sectionWalkHandler {
	level := strings.Count(prefix, "/")

	visitor := createVisitor()

	w.m.taxonomies.WalkBelow(prefix, func(s string, v any) bool {
		currentLevel := strings.Count(s, "/")

		if currentLevel > level+1 {
			return false
		}

		n := v.(*contentNode)

		if w.err = visitor.handleSectionPre(s, n); w.err != nil {
			return true
		}

		if currentLevel == 2 {
			nested := w.walkLevel(s, createVisitor)
			if w.err = visitor.handleNested(nested); w.err != nil {
				return true
			}
		} else {
			w.m.taxonomyEntries.WalkPrefix(s, func(ss string, v any) bool {
				n := v.(*contentNode)
				w.err = visitor.handlePage(ss, n)
				return w.err != nil
			})
		}

		w.err = visitor.handleSectionPost()

		return w.err != nil
	})

	w.m.sections.WalkBelow(prefix, func(s string, v any) bool {
		currentLevel := strings.Count(s, "/")
		if currentLevel > level+1 {
			return false
		}

		n := v.(*contentNode)

		if w.err = visitor.handleSectionPre(s, n); w.err != nil {
			return true
		}

		w.m.pages.WalkPrefix(s+cmBranchSeparator, func(s string, v any) bool {
			w.err = visitor.handlePage(s, v.(*contentNode))
			return w.err != nil
=======
		b.self.s.pageMap.WalkBranchesPrefix(ref.Key()+"/", func(s string, branch *contentBranchNode) bool {
			b.taxonomies = append(b.taxonomies, branch.n.p)
			return false
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		})
		page.SortByDefault(b.taxonomies)
	})

	return b.taxonomies
}

type pagesMapBucketPages struct {
	pagesAndSectionsInit sync.Once
	pagesAndSections     page.Pages

	regularPagesInit sync.Once
	regularPages     page.Pages

	regularPagesRecursiveInit sync.Once
	regularPagesRecursive     page.Pages

	sectionsInit sync.Once
	sections     page.Pages

	taxonomiesInit sync.Once
	taxonomies     page.Pages

	pagesInTermInit sync.Once
	pagesInTerm     page.Pages

	regularPagesInTermInit sync.Once
	regularPagesInTerm     page.Pages
}

type viewName struct {
	singular      string // e.g. "category"
	plural        string // e.g. "categories"
	pluralTreeKey string
}

func (v viewName) IsZero() bool {
	return v.singular == ""
}

func (v viewName) pluralParts() []string {
	return paths.FieldsSlash(v.plural)
}
