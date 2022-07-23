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
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources"
	"github.com/spf13/cast"

	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/common/maps"

	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
)

type pageMap struct {
	i int
	s *Site

	*pageTrees

	cachePages   memcache.Getter
	cacheContent memcache.Getter

	cfg contentMapConfig
}

const (
	pageTreeDimensionLanguage = iota
)

// pageTrees holds pages and resources in a tree structure for all sites/languages.
// Eeach site gets its own tree set via the Shape method.
type pageTrees struct {
	// This tree contains all Pages.
	// This include regular pages, sections, taxonimies and so on.
	// Note that all of these trees share the same key structure,
	// so you can take a leaf Page key and to a prefix search
	// treeLeafResources with key + "/" to get all of its resources.
	treePages *doctree.Root[contentNodeI]

	// This tree contains Resoures bundled in regular pages.
	treeLeafResources *doctree.Root[resource.Resource]

	// This tree contains Resources bundled in branch pages (e.g. sections).
	treeBranchResources *doctree.Root[resource.Resource]

	// This tree contains all taxonomy entries, e.g "/tags/blue/page1"
	treeTaxonomyEntries *doctree.Root[*weightedContentNode]

	// A slice of the resource trees.
	resourceTrees doctree.MutableTrees
}

// GetIdentities collects all identities from in all trees matching the given key.
// This will at most match in one tree, but may give identies from multiple dimensions (e.g. language).
func (t *pageTrees) GetIdentities(key string) []identity.Identity {
	var ids []identity.Identity

	// TODO1 others
	for _, n := range t.treePages.GetAll(key) {
		ids = append(ids, n)
	}

	return ids
}

func (t *pageTrees) DeletePage(key string) {
	commit1 := t.resourceTrees.Lock(true)
	defer commit1()
	commit2 := t.treePages.Lock(true)
	defer commit2()
	t.resourceTrees.DeletePrefix(helpers.AddLeadingSlash(key))
	t.treePages.Delete(key)
}

// Shape shapes all trees in t to the given dimension.
func (t pageTrees) Shape(d, v int) *pageTrees {
	t.treePages = t.treePages.Shape(d, v)
	t.treeLeafResources = t.treeLeafResources.Shape(d, v)
	t.treeBranchResources = t.treeBranchResources.Shape(d, v)
	t.treeTaxonomyEntries = t.treeTaxonomyEntries.Shape(d, v)
	return &t
}

var (
	_ types.Identifier = pageMapQueryPagesInSection{}
	_ types.Identifier = pageMapQueryPagesBelowPath{}
)

type pageMapQueryPagesInSection struct {
	pageMapQueryPagesBelowPath

	Recursive   bool
	IncludeSelf bool
}

func (q pageMapQueryPagesInSection) Key() string {
	return q.pageMapQueryPagesBelowPath.Key() + "/" + strconv.FormatBool(q.Recursive) + "/" + strconv.FormatBool(q.IncludeSelf)
}

//  This needs to be hashable.
type pageMapQueryPagesBelowPath struct {
	Path string

	// Set to true if this is to construct one of the site collections.
	ListFilterGlobal bool

	// Bar separated list of page kinds to include.
	KindsInclude string

	// Bar separated list of page kinds to exclude.
	// Will be ignored if KindsInclude is set.
	KindsExclude string
}

func (q pageMapQueryPagesBelowPath) Key() string {
	return q.Path + "/" + strconv.FormatBool(q.ListFilterGlobal) + "/" + q.KindsInclude + "/" + q.KindsExclude
}

// predicatePage returns whether to include a given Page.
func (q pageMapQueryPagesBelowPath) predicatePage() func(p *pageState) bool {
	return func(p *pageState) bool {
		if !p.m.shouldList(q.ListFilterGlobal) {
			return false
		}
		if q.KindsInclude != "" {
			return strings.Contains(q.KindsInclude, p.Kind())
		}
		if q.KindsExclude != "" {
			return !strings.Contains(q.KindsExclude, p.Kind())
		}
		return true
	}
}

func (m *pageMap) getOrCreatePagesFromCache(key string, create func() (page.Pages, error)) (page.Pages, error) {
	v, err := m.cachePages.GetOrCreate(context.TODO(), key, func() *memcache.Entry {

		pages, err := create()

		return &memcache.Entry{
			Value:     pages,
			Err:       err,
			ClearWhen: memcache.ClearOnRebuild,
		}
	})

	if err != nil {
		return nil, err
	}

	return v.(page.Pages), nil
}

func (m *pageMap) getPagesInSection(q pageMapQueryPagesInSection) page.Pages {
	cacheKey := q.Key()

	pages, err := m.getOrCreatePagesFromCache(cacheKey, func() (page.Pages, error) {
		prefix := helpers.AddTrailingSlash(q.Path)

		var (
			pas         page.Pages
			otherBranch string
			predicate   = q.predicatePage()
		)

		err := m.treePages.Walk(context.TODO(), doctree.WalkConfig[contentNodeI]{
			Prefix: prefix,
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
				if q.Recursive {

					if p, ok := n.(*pageState); ok && predicate(p) {
						pas = append(pas, p)
					}
					return false, nil
				}
				if otherBranch == "" || !strings.HasPrefix(key, otherBranch) {
					if p, ok := n.(*pageState); ok && predicate(p) {
						pas = append(pas, p)
					}
				}
				if n.isContentNodeBranch() {
					otherBranch = key
				}
				return false, nil
			},
		})

		if err == nil {
			if q.IncludeSelf {
				pas = append(pas, m.treePages.Get(q.Path).(page.Page))
			}
			page.SortByDefault(pas)
		}

		return pas, err

	})

	if err != nil {
		panic(err)
	}

	return pages

}

func (m *pageMap) getPagesWithTerm(q pageMapQueryPagesBelowPath) page.Pages {
	key := q.Key()
	v, err := m.cachePages.GetOrCreate(context.TODO(), key, func() *memcache.Entry {
		var (
			pas       page.Pages
			predicate = q.predicatePage()
		)
		err := m.treeTaxonomyEntries.Walk(context.TODO(), doctree.WalkConfig[*weightedContentNode]{
			Prefix: helpers.AddTrailingSlash(q.Path),
			Callback: func(ctx *doctree.WalkContext[*weightedContentNode], key string, n *weightedContentNode) (bool, error) {
				p := n.n.(*pageState)
				if !predicate(p) {
					return false, nil
				}
				pas = append(pas, p)
				return false, nil
			},
		})

		page.SortByDefault(pas)

		return &memcache.Entry{
			Value:     pas,
			Err:       err,
			ClearWhen: memcache.ClearOnRebuild,
		}
	})

	if err != nil {
		panic(err)
	}

	return v.(page.Pages)
}

func (m *pageMap) getTermsForPageInTaxonomy(path, taxonomy string) page.Pages {
	prefix := "/" + taxonomy // TODO1

	v, err := m.cachePages.GetOrCreate(context.TODO(), prefix+path, func() *memcache.Entry {
		var pas page.Pages

		err := m.treeTaxonomyEntries.Walk(context.TODO(), doctree.WalkConfig[*weightedContentNode]{
			Prefix: prefix,
			Callback: func(ctx *doctree.WalkContext[*weightedContentNode], key string, n *weightedContentNode) (bool, error) {
				if strings.HasSuffix(key, path) {
					pas = append(pas, n.term.(page.Page))
				}
				return false, nil
			},
		})

		page.SortByDefault(pas)

		return &memcache.Entry{
			Value:     pas,
			Err:       err,
			ClearWhen: memcache.ClearOnRebuild,
		}
	})

	if err != nil {
		panic(err)
	}

	return v.(page.Pages)
}

func (m *pageMap) getResourcesForPage(p *pageState) resource.Resources {
	key := p.Path() + "/get-resources-for-page"
	v, err := m.cachePages.GetOrCreate(context.TODO(), key, func() *memcache.Entry {
		prefix := p.Path()
		if prefix != "/" {
			prefix += "/"
		}
		tree := m.treeLeafResources
		if p.IsNode() {
			tree = m.treeBranchResources
		}

		targetPaths := p.targetPaths()
		pagePath := p.Path()

		if targetPaths.Link == "/mybundle/mybundle/" {

		}

		var res resource.Resources
		err := tree.Walk(context.TODO(), doctree.WalkConfig[resource.Resource]{
			Prefix: prefix,
			Callback: func(ctx *doctree.WalkContext[resource.Resource], key string, n resource.Resource) (bool, error) {
				// bookmark
				if init, ok := n.(*resources.ResourceLazyInit); ok {
					var err error
					n, err = init.Init(func(p *paths.Path, openContent resource.OpenReadSeekCloser) (resource.Resource, error) {
						relPath := strings.TrimPrefix(p.Path(), pagePath)
						var rd resources.ResourceSourceDescriptor
						rd.OpenReadSeekCloser = openContent
						rd.Path = p
						rd.RelPermalink = path.Join(targetPaths.SubResourceBaseLink, relPath)
						rd.TargetPath = path.Join(targetPaths.SubResourceBaseTarget, relPath)

						return m.s.ResourceSpec.New(rd)

					})

					if err != nil {
						return false, err
					}
				}

				res = append(res, n)
				return false, nil
			},
		})

		if err == nil {
			lessFunc := func(i, j int) bool {
				ri, rj := res[i], res[j]
				if ri.ResourceType() < rj.ResourceType() {
					return true
				}

				p1, ok1 := ri.(page.Page)
				p2, ok2 := rj.(page.Page)

				if ok1 != ok2 {
					return ok2
				}

				if ok1 {
					return page.DefaultPageSort(p1, p2)
				}

				// Make sure not to use RelPermalink or any of the other methods that
				// trigger lazy publishing.
				return ri.Name() < rj.Name()
			}
			sort.SliceStable(res, lessFunc)

			if len(p.m.resourcesMetadata) > 0 {
				resources.AssignMetadata(p.m.resourcesMetadata, res...)
				sort.SliceStable(res, lessFunc)
			}
		}

		return &memcache.Entry{
			Value:     res,
			Err:       err,
			ClearWhen: memcache.ClearOnRebuild,
		}
	})

	if err != nil {
		panic(err)
	}

	return v.(resource.Resources)
}

type weightedContentNode struct {
	n       contentNodeI
	weight  int
	ordinal int
	term    contentNodeI
}

type contentNodeI interface {
	identity.Identity
	Path() string
	isContentNodeBranch() bool
	isContentNodeResource() bool
}

var _ contentNodeI = (*contentNodeIs)(nil)

type contentNodeIs []contentNodeI

func (n contentNodeIs) Path() string {
	return n[0].Path()
}

func (n contentNodeIs) isContentNodeBranch() bool {
	return n[0].isContentNodeBranch()
}

func (n contentNodeIs) isContentNodeResource() bool {
	return n[0].isContentNodeResource()
}

func (n contentNodeIs) IdentifierBase() any {
	return n[0].IdentifierBase()
}

type contentNodeShifter struct {
	langIntToLang map[int]string
	langLangToInt map[string]int
}

func (s *contentNodeShifter) Shift(n contentNodeI, dimension []int) (contentNodeI, bool) {
	switch v := n.(type) {
	case contentNodeIs:
		if len(v) == 0 {
			panic("empty contentNodeIs")
		}
		vv := v[dimension[0]]
		return vv, vv != nil
	case page.Page:
		if v.Lang() == s.langIntToLang[dimension[0]] {
			return n, true
		}
	case resource.Resource:
		panic("TODO1: not implemented")
		//return n, true
	}
	return nil, false
}

func (s *contentNodeShifter) All(n contentNodeI) []contentNodeI {
	switch vv := n.(type) {
	case contentNodeIs:
		return vv
	default:
		return contentNodeIs{n}
	}
}

func (s *contentNodeShifter) Dimension(n contentNodeI, d int) []contentNodeI {
	// We currently have only one dimension.
	if d != 0 {
		panic("dimension out of range")
	}
	return s.All(n)
}

func (s *contentNodeShifter) Insert(old, new contentNodeI) (contentNodeI, bool) {

	if newp, ok := new.(*pageState); ok {
		switch vv := old.(type) {
		case *pageState:
			if vv.Lang() == newp.Lang() {
				return new, true
			}
			is := make(contentNodeIs, len(s.langIntToLang))
			is[s.langLangToInt[newp.Lang()]] = new
			is[s.langLangToInt[vv.Lang()]] = old
			return is, true
		case contentNodeIs:
			vv[s.langLangToInt[newp.Lang()]] = new
			return vv, true
		default:
			panic("TODO1: not implemented")

		}
	} else {
		panic("TODO1: not implemented")
	}

}

type resourceShifter struct {
}

func (s *resourceShifter) Shift(n resource.Resource, dimension []int) (resource.Resource, bool) {
	return n, true
}

func (s *resourceShifter) All(n resource.Resource) []resource.Resource {
	return []resource.Resource{n}
}

func (s *resourceShifter) Dimension(n resource.Resource, d int) []resource.Resource {
	// We currently have only one dimension.
	if d != 0 {
		panic("dimension out of range")
	}
	return s.All(n)
}

func (s *resourceShifter) Insert(old, new resource.Resource) (resource.Resource, bool) {
	return new, true
}

type weightedContentNodeShifter struct {
}

func (s *weightedContentNodeShifter) Shift(n *weightedContentNode, dimension []int) (*weightedContentNode, bool) {
	return n, true
}

func (s *weightedContentNodeShifter) All(n *weightedContentNode) []*weightedContentNode {
	return []*weightedContentNode{n}
}

func (s *weightedContentNodeShifter) Dimension(n *weightedContentNode, d int) []*weightedContentNode {
	// We currently have only one dimension.
	if d != 0 {
		panic("dimension out of range")
	}
	return s.All(n)
}

func (s *weightedContentNodeShifter) Insert(old, new *weightedContentNode) (*weightedContentNode, bool) {
	return new, true
}

func newPageMap(i int, s *Site) *pageMap {
	var m *pageMap

	taxonomiesConfig := s.siteCfg.taxonomiesConfig.Values()

	m = &pageMap{
		pageTrees:    s.h.pageTrees.Shape(0, i),
		cachePages:   s.MemCache.GetOrCreatePartition(fmt.Sprintf("pages/%d", i), memcache.ClearOnRebuild),
		cacheContent: s.MemCache.GetOrCreatePartition(fmt.Sprintf("content/%d", i), memcache.ClearOnRebuild),

		// Old

		cfg: contentMapConfig{
			lang:                 s.Lang(),
			taxonomyConfig:       taxonomiesConfig,
			taxonomyDisabled:     !s.isEnabled(pagekinds.Taxonomy),
			taxonomyTermDisabled: !s.isEnabled(pagekinds.Term),
			pageDisabled:         !s.isEnabled(pagekinds.Page),
		},
		i: i,
		s: s,
	}

	// TODO1
	/*
		m.pageReverseIndex = &contentTreeReverseIndex{
			initFn: func(rm map[any]*contentNode) {
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
	*/

	return m
}

type contentTreeReverseIndex struct {
	initFn func(rm map[any]page.Page)
	*contentTreeReverseIndexMap
}

func (c *contentTreeReverseIndex) Reset() {
	c.contentTreeReverseIndexMap = &contentTreeReverseIndexMap{
		m: make(map[any]page.Page),
	}
}

func (c *contentTreeReverseIndex) Get(key any) page.Page {
	c.init.Do(func() {
		c.m = make(map[any]page.Page)
		c.initFn(c.contentTreeReverseIndexMap.m)
	})
	return c.m[key]
}

type contentTreeReverseIndexMap struct {
	init sync.Once
	m    map[any]page.Page
}

type sitePagesAssembler struct {
	*Site
	changeTracker *whatChanged
}

// Calculate and apply aggregate values to the page tree (e.g. dates, cascades).
func (site *sitePagesAssembler) applyAggregates() error {
	aggregatesWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeRead,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			p := n.(*pageState)

			// Handle cascades first to get any default dates set.
			var cascade map[page.PageMatcher]maps.Params
			if s == "" {
				// Home page gets it's cascade from the site config.
				cascade = site.cascade

				if p.m.cascade == nil {
					// Pass the site cascade downwards.
					ctx.Data().Insert(s, cascade)
				}
			} else {
				_, data := ctx.Data().LongestPrefix(s)
				if data != nil {
					cascade = data.(map[page.PageMatcher]maps.Params)
				}
			}

			p.setMetadatPost(cascade)

			const eventName = "dates"
			if n.isContentNodeBranch() {
				p := n.(*pageState)
				if p.m.cascade != nil {
					// Pass it down.
					ctx.Data().Insert(s, p.m.cascade)
				}
				ctx.AddEventListener(eventName, s, func(e *doctree.Event[contentNodeI]) {
					sp, ok1 := e.Source.(*pageState)
					tp, ok2 := n.(*pageState)
					if ok1 && ok2 {
						if !sp.m.dates.IsDateOrLastModAfter(tp.m.dates) {
							// Prevent unnecessary bubbling of events.
							e.StopPropagation()
						}
						tp.m.dates.UpdateDateAndLastmodIfAfter(sp.m.dates)

						if tp.IsHome() {
							if tp.m.dates.Lastmod().After(tp.s.lastmod) {
								tp.s.lastmod = tp.m.dates.Lastmod()
							}
							if sp.m.dates.Lastmod().After(tp.s.lastmod) {
								tp.s.lastmod = sp.m.dates.Lastmod()
							}
						}
					}
				})
			}

			ctx.SendEvent(&doctree.Event[contentNodeI]{Source: n, Path: s, Name: eventName})

			return false, nil
		},
	}

	return site.pageMap.treePages.Walk(context.TODO(), aggregatesWalker)

}

func (site *sitePagesAssembler) removeDisabledKinds() error {
	cfg := site.pageMap.cfg

	if cfg.pageDisabled {
		var keys []string
		site.pageMap.treePages.Walk(
			context.TODO(), doctree.WalkConfig[contentNodeI]{
				LockType: doctree.LockTypeRead,
				Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
					p := n.(*pageState)
					switch p.Kind() {
					case pagekinds.Page, pagekinds.Taxonomy, pagekinds.Term:
						keys = append(keys, s)
					case pagekinds.Home, pagekinds.Section:

					}

					return false, nil
				},
			},
		)

		for _, k := range keys {
			site.pageMap.DeletePage(k)
		}

	}

	return nil
}

func (site *sitePagesAssembler) removeShouldNotBuild() error {
	s := site.Site
	var keys []string
	site.pageMap.treePages.Walk(
		context.TODO(), doctree.WalkConfig[contentNodeI]{
			LockType: doctree.LockTypeRead,
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
				if !s.shouldBuild(n.(*pageState)) {
					keys = append(keys, key)
					if key == "" {
						return true, nil
					}
				}
				return false, nil
			},
		},
	)
	for _, k := range keys {
		site.pageMap.DeletePage(k)
	}

	return nil
}

func (site *sitePagesAssembler) assembleTaxonomies() error {
	if site.pageMap.cfg.taxonomyDisabled || site.pageMap.cfg.taxonomyTermDisabled {
		return nil
	}

	var (
		tree                = site.pageMap.treePages
		treeTaxonomyEntries = site.pageMap.treeTaxonomyEntries
	)

	taxonomiesWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeWrite,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			p := n.(*pageState)
			for _, viewName := range site.pageMap.cfg.taxonomyConfig.views {
				vals := types.ToStringSlicePreserveString(getParam(p, viewName.plural, false))
				if vals == nil {
					continue
				}
				w := getParamToLower(p, viewName.plural+"_weight")
				weight, err := cast.ToIntE(w)
				if err != nil {
					site.Log.Errorf("Unable to convert taxonomy weight %#v to int for %q", w, n.Path())
					// weight will equal zero, so let the flow continue
				}

				for i, v := range vals {
					termKey := site.getTaxonomyKey(v)
					viewTermKey := "/" + viewName.plural + "/" + termKey
					term := tree.Get(viewTermKey)
					if term == nil {
						// TODO1 error handling.
						m := &pageMeta{
							s:        site.Site,
							pathInfo: paths.Parse(viewTermKey),
							kind:     pagekinds.Term,
						}
						n, _ := site.h.newPage(m)
						tree.Insert(viewTermKey, n) // TODO1 insert vs shift
						term = tree.Get(viewTermKey)
					}

					key := viewTermKey + s
					treeTaxonomyEntries.Insert(key, &weightedContentNode{
						ordinal: i,
						weight:  weight,
						n:       n,
						term:    term,
					})
				}
			}
			return false, nil
		},
	}

	return tree.Walk(context.TODO(), taxonomiesWalker)
}

// // Create the fixed output pages, e.g. sitemap.xml, if not already there.
func (site *sitePagesAssembler) addStandalonePages() error {
	s := site.Site
	m := s.pageMap
	tree := m.treePages

	commit := tree.Lock(true)
	defer commit()

	addStandalone := func(key, kind string, f output.Format) {
		if !site.Site.isEnabled(kind) || tree.Has(key) {
			return
		}

		m := &pageMeta{
			s:                      s,
			pathInfo:               paths.Parse(key),
			kind:                   kind,
			standaloneOutputFormat: f,
		}

		p, _ := s.h.newPage(m)

		tree.Insert(key, p)

	}

	addStandalone("/404", pagekinds.Status404, output.HTTPStatusHTMLFormat)
	if m.i == 0 || m.s.h.IsMultihost() {
		addStandalone("/robots", pagekinds.RobotsTXT, output.RobotsTxtFormat)
	}

	// TODO1 coordinate
	addStandalone("/sitemap", pagekinds.Sitemap, output.SitemapFormat)

	return nil
}

func (site *sitePagesAssembler) addMissingRootSections() error {
	isBranchPredicate := func(n contentNodeI) bool {
		return n.isContentNodeBranch()
	}

	var (
		tree    = site.pageMap.treePages
		hasHome bool
	)

	// Add missing root sections.
	missingRootSectionsWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeWrite,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			if n == nil {
				panic("n is nil")
			}
			if ps, ok := n.(*pageState); ok {
				if ps.Lang() != site.Lang() {
					panic(fmt.Sprintf("lang mismatch: %q: %s != %s", s, ps.Lang(), site.Lang()))
				}
			}

			if s == "" {
				hasHome = true
				site.home = n.(*pageState)
			}

			if !n.isContentNodeBranch() {
				p := paths.Parse(s)
				_, n := tree.LongestPrefix(p.Dir(), isBranchPredicate)

				if n == nil {
					p := paths.Parse("/" + p.Section())
					// TODO1 error handling.
					m := &pageMeta{
						s:        site.Site,
						pathInfo: p,
						kind:     pagekinds.Section,
					}
					n, _ := site.h.newPage(m)
					tree.Insert(p.Path(), n)
				}
			}

			// /a/b
			// TODO1
			if strings.Count(s, "/") > 1 {
				//return true, nil
			}
			return false, nil
		},
	}

	if err := tree.Walk(context.TODO(), missingRootSectionsWalker); err != nil {
		return err
	}

	if !hasHome {
		p := paths.Parse("")
		// TODO1 error handling.
		m := &pageMeta{
			s:        site.Site,
			pathInfo: p,
			kind:     pagekinds.Home,
		}
		n, _ := site.h.newPage(m)
		tree.InsertWithLock(p.Path(), n)
		site.home = n
	}

	return nil
}

func (site *sitePagesAssembler) addMissingTaxonomies() error {
	if site.pageMap.cfg.taxonomyDisabled {
		return nil
	}

	var tree = site.pageMap.treePages

	commit := tree.Lock(true)
	defer commit()

	for _, viewName := range site.pageMap.cfg.taxonomyConfig.views {
		key := viewName.pluralTreeKey
		if v := tree.Get(key); v == nil {
			m := &pageMeta{
				s:        site.Site,
				pathInfo: paths.Parse(key),
				kind:     pagekinds.Taxonomy,
			}
			p, _ := site.h.newPage(m)
			tree.Insert(key, p)
		}
	}

	return nil

}

func (site *Site) AssemblePages(changeTracker *whatChanged) error {

	assembler := &sitePagesAssembler{
		Site:          site,
		changeTracker: changeTracker,
	}

	if err := assembler.addMissingRootSections(); err != nil {
		return err
	}

	if err := assembler.addMissingTaxonomies(); err != nil {
		return err
	}

	if err := assembler.addStandalonePages(); err != nil {
		return err
	}

	if err := assembler.removeDisabledKinds(); err != nil {
		return err
	}

	if err := assembler.assembleTaxonomies(); err != nil {
		return err
	}

	if err := assembler.applyAggregates(); err != nil {
		return err
	}

	// This needs to be done after we have applied the cascades.
	// TODO1 check if we need to split the above.
	if err := assembler.removeShouldNotBuild(); err != nil {
		return err
	}

	return nil

}

func (m *pageMap) CreateSiteTaxonomies() error {
	m.s.taxonomies = make(TaxonomyList)

	if m.cfg.taxonomyDisabled {
		return nil
	}

	for _, viewName := range m.cfg.taxonomyConfig.views {
		key := viewName.pluralTreeKey
		m.s.taxonomies[viewName.plural] = make(Taxonomy)
		taxonomyWalker := doctree.WalkConfig[contentNodeI]{
			Prefix:   helpers.AddTrailingSlash(key),
			LockType: doctree.LockTypeRead,
			Callback: func(ctx *doctree.WalkContext[contentNodeI], k1 string, n contentNodeI) (bool, error) {
				p := n.(*pageState)
				plural := p.Section()

				switch p.Kind() {
				case pagekinds.Term:
					taxonomy := m.s.taxonomies[plural]
					if taxonomy == nil {
						return true, fmt.Errorf("missing taxonomy: %s", plural)
					}
					entryWalker := doctree.WalkConfig[*weightedContentNode]{
						Prefix:   helpers.AddTrailingSlash(k1),
						LockType: doctree.LockTypeRead,
						Callback: func(ctx *doctree.WalkContext[*weightedContentNode], k2 string, wn *weightedContentNode) (bool, error) {
							taxonomy.add(p.m.pathInfo.NameNoIdentifier(), page.NewWeightedPage(wn.weight, wn.n.(page.Page), wn.term.(page.Page)))
							return false, nil
						},
					}
					if err := m.treeTaxonomyEntries.Walk(context.TODO(), entryWalker); err != nil {
						return true, err
					}
				default:
					return false, nil
				}

				return false, nil
			},
		}
		if err := m.treePages.Walk(context.TODO(), taxonomyWalker); err != nil {
			return err
		}
	}

	for _, taxonomy := range m.s.taxonomies {
		for _, v := range taxonomy {
			v.Sort()
		}
	}

	return nil
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
