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
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gohugoio/hugo/common/types"
	"github.com/spf13/cast"

	"github.com/gohugoio/hugo/common/maps"

	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"

	"github.com/gohugoio/hugo/common/para"
)

func newPageMap(s *Site) *pageMap {
	taxonomiesConfig := s.siteCfg.taxonomiesConfig.Values()
	createBranchNode := func(key string) *contentNode {
		n := &contentNode{}
		if view, found := taxonomiesConfig.viewsByTreeKey[key]; found {
			n.viewInfo = &contentBundleViewInfo{
				name:       view,
				termKey:    view.plural,
				termOrigin: view.plural,
			}
			n.viewInfo.ref = n
		}
		return n
	}

	m := &pageMap{
		cfg: contentMapConfig{
			lang:                 s.Lang(),
			taxonomyConfig:       taxonomiesConfig,
			taxonomyDisabled:     !s.isEnabled(page.KindTaxonomy),
			taxonomyTermDisabled: !s.isEnabled(page.KindTerm),
			pageDisabled:         !s.isEnabled(page.KindPage),
		},
		s:         s,
		branchMap: newBranchMap(createBranchNode),
	}

	m.pageReverseIndex = &contentTreeReverseIndex{
		initFn: func(rm map[interface{}]*contentNode) {
			m.WalkPagesAllPrefixSection("", nil, contentTreeNoListAlwaysFilter, func(n contentNodeProvider) bool {
				k := cleanTreeKey(path.Base(n.Key()))
				existing, found := rm[k]
				if found && existing != ambiguousContentNode {
					rm[k] = ambiguousContentNode
				} else if !found {
					rm[k] = n.GetNode()
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
		workers: para.New(h.numWorkers),
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

func (c *contentTreeReverseIndex) Get(key interface{}) *contentNode {
	c.init.Do(func() {
		c.m = make(map[interface{}]*contentNode)
		c.initFn(c.contentTreeReverseIndexMap.m)
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
	s   *Site

	*branchMap

	// A reverse index used as a fallback in GetPage for short references.
	pageReverseIndex *contentTreeReverseIndex
}

func (m *pageMap) WalkTaxonomyTerms(fn func(s string, b *contentBranchNode) bool) {
	for _, viewName := range m.cfg.taxonomyConfig.views {
		m.WalkBranchesPrefix(viewName.pluralTreeKey+"/", func(s string, b *contentBranchNode) bool {
			return fn(s, b)
		})
	}
}

func (m *pageMap) createListAllPages() page.Pages {
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

	return nil
}

func (m *pageMap) createSiteTaxonomies() error {
	m.s.taxonomies = make(TaxonomyList)
	for _, viewName := range m.cfg.taxonomyConfig.views {
		taxonomy := make(Taxonomy)
		m.s.taxonomies[viewName.plural] = taxonomy
		m.WalkBranchesPrefix(viewName.pluralTreeKey+"/", func(s string, b *contentBranchNode) bool {
			info := b.n.viewInfo
			for k, v := range b.refs {
				taxonomy.add(info.termKey, page.NewWeightedPage(v.weight, k.(*pageState), b.n.p))
			}

			return false
		})
	}

	for _, taxonomy := range m.s.taxonomies {
		for _, v := range taxonomy {
			v.Sort()
		}
	}

	return nil
}

type pageDescriptor struct {
	treeRef *contentTreeRef
}

func (m *pageMap) assemblePages() error {
	isRebuild := m.cfg.isRebuild
	var err error

	if isRebuild {
		m.WalkTaxonomyTerms(func(s string, b *contentBranchNode) bool {
			b.refs = make(map[interface{}]ordinalWeight)
			return false
		})
	}

	// Holds references to sections or pages to exlude from the build
	// because front matter dictated it (e.g. a draft).
	var (
		sectionsToDelete = make(map[string]bool)
		pagesToDelete    []*contentTreeRef
	)

	handleBranch := func(np contentNodeProvider) bool {
		n := np.GetNode()
		s := np.Key()
		branch := np.(contentGetBranchProvider).GetBranch()
		owner := np.(contentGetOwnerBranchProvider).GetOwnerBranch()

		if n.p != nil {
			// Page already set, nothing more to do.
			if n.p.IsHome() {
				m.s.home = n.p
			}
			return false
		}

		// Determine Page Kind.
		var kind string
		if s == "" {
			kind = page.KindHome
		} else {
			// It's either a view (taxonomy, term) or a section.
			kind = m.cfg.taxonomyConfig.getPageKind(s)
			if kind == "" {
				kind = page.KindSection
			}
		}

		if n.fi != nil {
			n.p, err = m.s.newPageFromContentNode(n, owner.n.p.bucket, nil)
			if err != nil {
				return true
			}
		} else {
			n.p = m.s.newPage(n, owner.n.p.bucket, kind, "", m.splitKey(s)...)
		}

		n.p.treeRef = &contentTreeRef{
			m:      m,
			branch: branch,
			owner:  owner,
			key:    s,
			n:      n,
		}
		n.p.treeRef2 = np

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

	handlePage := func(np contentNodeProvider) bool {
		n := np.GetNode()
		s := np.Key()
		branch := np.(contentGetBranchProvider).GetBranch()
		owner := np.(contentGetOwnerBranchProvider).GetOwnerBranch()

		if n.fi != nil {
			n.p, err = m.s.newPageFromContentNode(n, branch.n.p.bucket, nil)
			if err != nil {
				return true
			}
		} else {
			n.p = m.s.newPage(n, owner.n.p.bucket, page.KindPage, "", m.splitKey(s)...)
		}

		tref := &contentTreeRef{
			m:      m,
			branch: branch,
			owner:  owner,
			key:    s,
			n:      n,
		}
		n.p.treeRef = tref

		if !m.s.shouldBuild(n.p) {
			pagesToDelete = append(pagesToDelete, tref)
			return false
		}

		branch.n.p.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.userProvided)

		return false
	}

	handleResource := func(np contentNodeProvider) bool {
		n := np.GetNode()
		branch := np.(contentGetBranchProvider).GetBranch()
		owner := np.(contentGetOwnerNodeProvider).GetOwnerNode()

		if owner.p == nil {
			panic("invalid state, page not set on resource owner")
		}

		p := owner.p
		meta := n.fi.Meta()
		classifier := meta.Classifier()
		var r resource.Resource
		switch classifier {
		case files.ContentClassContent:
			var rp *pageState
			rp, err = m.s.newPageFromContentNode(n, branch.n.p.bucket, p)
			if err != nil {
				return true
			}
			rp.m.resourcePath = filepath.ToSlash(strings.TrimPrefix(rp.Path(), p.File().Dir()))
			r = rp

		case files.ContentClassFile:
			r, err = branch.newResource(n.fi, p)
			if err != nil {
				return true
			}
		default:
			panic(fmt.Sprintf("invalid classifier: %q", classifier))
		}

		p.resources = append(p.resources, r)

		return false
	}

	// Create home page if it does not exist.

	hn := m.Get("")
	if hn == nil {
		hn = m.InsertBranch("", &contentNode{})
	}

	if hn.n.p == nil {
		if hn.n.fi != nil {
			hn.n.p, err = m.s.newPageFromContentNode(hn.n, nil, nil)
			if err != nil {
				return err
			}
		} else {
			hn.n.p = m.s.newPage(hn.n, nil, page.KindHome, "")
		}

		if !m.s.shouldBuild(hn.n.p) {
			m.branches.DeletePrefix("")
			return nil
		}

		hn.n.p.treeRef = &contentTreeRef{
			m:      m,
			branch: nil,
			owner:  nil,
			key:    "",
			n:      hn.n,
		}
		hn.n.p.treeRef2 = m.newNodeProviderPage("", hn.n, nil, nil, false)

	}

	m.s.home = hn.n.p

	// First pass.
	m.Walk(
		branchMapQuery{
			Deep:    true, // Need the branch tree
			Exclude: func(s string, n *contentNode) bool { return n.p != nil },
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

	if err != nil {
		return err
	}

	// Delete pages and sections marked for deletion.
	for _, p := range pagesToDelete {
		p.branch.pages.nodes.Delete(p.key)
		p.branch.pageResources.nodes.Delete(p.key + "/")
		if p.branch.n.fi == nil && p.branch.pages.nodes.Len() == 0 {
			// Delete orphan section.
			sectionsToDelete[p.branch.key] = true
		}
	}

	for s := range sectionsToDelete {
		m.branches.Delete(s)
		m.branches.DeletePrefix(s + "/")
	}

	// Attach pages to views.
	if !m.cfg.taxonomyDisabled {
		for _, viewName := range m.cfg.taxonomyConfig.views {

			key := cleanTreeKey(viewName.plural)
			if sectionsToDelete[key] {
				continue
			}

			taxonomy := m.Get(key)
			if taxonomy == nil {
				n := &contentNode{
					viewInfo: &contentBundleViewInfo{
						name: viewName,
					},
				}

				taxonomy = m.InsertBranch(key, n)
				n.p = m.s.newPage(n, m.s.home.bucket, page.KindTaxonomy, "", viewName.plural)

				n.p.treeRef = &contentTreeRef{
					m:      m,
					owner:  hn,
					branch: taxonomy,
					key:    key,
					n:      n,
				}

				n.p.treeRef2 = m.newNodeProviderPage(key, n, hn, taxonomy, true)

			}

			handleTaxonomyEntries := func(np contentNodeProvider) bool {
				n := np.GetNode()
				s := np.Key()

				if m.cfg.taxonomyTermDisabled {
					return false
				}
				if n.p == nil {
					panic("page is nil: " + s)
				}
				vals := types.ToStringSlicePreserveString(getParam(n.p, viewName.plural, false))
				if vals == nil {
					return false
				}
				w := getParamToLower(n.p, viewName.plural+"_weight")
				weight, err := cast.ToIntE(w)
				if err != nil {
					m.s.Log.Errorf("Unable to convert taxonomy weight %#v to int for %q", w, n.p.Path())
					// weight will equal zero, so let the flow continue
				}

				for i, v := range vals {
					term := m.s.getTaxonomyKey(v)

					termKey := cleanTreeKey(term)
					taxonomyTermKey := taxonomy.key + termKey

					// It may have been added with the content files
					termBranch := m.Get(taxonomyTermKey)

					if termBranch == nil {

						vic := &contentBundleViewInfo{
							name:       viewName,
							termKey:    term,
							termOrigin: v,
						}

						n := &contentNode{viewInfo: vic}
						n.p = m.s.newPage(n, taxonomy.n.p.bucket, page.KindTerm, vic.term(), viewName.plural, term)

						termBranch = m.InsertBranch(taxonomyTermKey, n)

						n.p.treeRef = &contentTreeRef{
							m:      m,
							owner:  taxonomy,
							branch: termBranch,
							key:    taxonomyTermKey,
							n:      n,
						}
						n.p.treeRef2 = np

					}

					termBranch.refs[n.p] = ordinalWeight{ordinal: i, weight: weight}
					termBranch.n.p.m.calculated.UpdateDateAndLastmodIfAfter(n.p.m.userProvided)

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
			owner := np.(contentGetOwnerBranchProvider).GetOwnerBranch()

			if s == "" {
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
	}

	return nil
}

func (m *pageMap) withEveryBundleNode(fn func(n *contentNode) bool) error {
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

// withEveryBundlePage applies fn to every Page, including those bundled inside
// leaf bundles.
func (m *pageMap) withEveryBundlePage(fn func(p *pageState) bool) error {
	return m.withEveryBundleNode(func(n *contentNode) bool {
		if n.p != nil {
			return fn(n.p)
		}
		return false
	})
}

type pageMaps struct {
	workers *para.Workers
	pmaps   []*pageMap
}

func (m *pageMaps) AssemblePages() error {
	return m.withMaps(func(runner para.Runner, pm *pageMap) error {
		if err := pm.assemblePages(); err != nil {
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
		return pm.withEveryBundleNode(fn)
	})
}

func (m *pageMaps) withMaps(fn func(runner para.Runner, pm *pageMap) error) error {
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
		b.pagesAndSections = b.self.treeRef.getPagesAndSections()
	})

	return b.pagesAndSections
}

func (b *pagesMapBucket) getPagesInTerm() page.Pages {
	if b == nil {
		return nil
	}

	b.pagesInTermInit.Do(func() {
		ref := b.self.treeRef
		if ref == nil {
			return
		}

		for k := range ref.branch.refs {
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
		b.regularPages = b.self.treeRef.getRegularPages()
		page.SortByDefault(b.regularPages)
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
		b.regularPagesRecursive = b.self.treeRef.getRegularPagesRecursive()
		page.SortByDefault(b.regularPagesRecursive)
	})

	return b.regularPagesRecursive
}

func (b *pagesMapBucket) getSections() page.Pages {
	if b == nil {
		return nil
	}

	b.sectionsInit.Do(func() {
		if b.self.treeRef == nil {
			return
		}
		b.sections = b.self.treeRef.getSections()
	})

	return b.sections
}

func (b *pagesMapBucket) getTaxonomies() page.Pages {
	if b == nil {
		return nil
	}

	b.taxonomiesInit.Do(func() {
		ref := b.self.treeRef
		if ref == nil {
			return
		}

		b.self.s.pageMap.WalkBranchesPrefix(ref.key+"/", func(s string, branch *contentBranchNode) bool {
			b.taxonomies = append(b.taxonomies, branch.n.p)
			return false
		})
		page.SortByDefault(b.taxonomies)
	})

	return b.taxonomies
}

type pagesMapBucketPages struct {
	pagesInit sync.Once
	pages     page.Pages

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
