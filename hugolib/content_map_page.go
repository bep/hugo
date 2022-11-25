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
	"github.com/gohugoio/hugo/source"
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
	pageReverseIndex *contentTreeReverseIndex

	cachePages           *memcache.Partition[string, page.Pages]
	cacheResources       *memcache.Partition[string, resource.Resources]
	cacheContentRendered *memcache.Partition[string, *resources.StaleValue[contentTableOfContents]]
	cacheContentPlain    *memcache.Partition[string, *resources.StaleValue[plainPlainWords]]
	cacheContentSource   *memcache.Partition[string, *resources.StaleValue[[]byte]]

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

	// This tree contains Resoures bundled in pages.
	treeResources *doctree.Root[doctree.NodeGetter[resource.Resource]]

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
	// TODO1
	t.resourceTrees.DeletePrefix(helpers.AddTrailingSlash(key))

	/*
		/section/logo.png
		/section/page1
		/section/page1/logo.png
		/section/page2
		/section/subsection/page1
		/section/subsection/logo.png

		Delete: /section/page1 => prefix /section/page1/
		Delete: /section => exact /section/page1/logo.png
	*/

	t.treePages.Delete(key)
}

// Shape shapes all trees in t to the given dimension.
func (t pageTrees) Shape(d, v int) *pageTrees {
	t.treePages = t.treePages.Shape(d, v)
	t.treeResources = t.treeResources.Shape(d, v)
	t.treeTaxonomyEntries = t.treeTaxonomyEntries.Shape(d, v)

	return &t
}

func (t *pageTrees) debugPrint(prefix string, maxLevel int) {
	fmt.Println(prefix, ":")
	var prevKey string
	err := t.treePages.Walk(context.Background(), doctree.WalkConfig[contentNodeI]{
		Prefix: prefix,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
			level := strings.Count(key, "/")
			if level > maxLevel {
				return false, nil
			}
			p := n.(*pageState)
			s := strings.TrimPrefix(key, paths.CommonDir(prevKey, key))
			lenIndent := len(key) - len(s)
			fmt.Print(strings.Repeat("__", lenIndent))
			info := fmt.Sprintf("%s (%s)", s, p.Kind())
			fmt.Println(info)
			if p.Kind() == pagekinds.Taxonomy {

				w := doctree.WalkConfig[*weightedContentNode]{
					LockType: doctree.LockTypeWrite,
					Callback: func(ctx *doctree.WalkContext[*weightedContentNode], s string, n *weightedContentNode) (bool, error) {
						fmt.Print(strings.Repeat("__", lenIndent+2))
						fmt.Println(s)
						return false, nil
					},
				}
				t.treeTaxonomyEntries.Walk(context.Background(), w)

			}
			prevKey = key

			return false, nil

		},
	})
	if err != nil {
		panic(err)
	}

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

// This needs to be hashable.
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
			// TODO1 int.
			return strings.Contains(q.KindsInclude, p.Kind())
		}
		if q.KindsExclude != "" {
			return !strings.Contains(q.KindsExclude, p.Kind())
		}
		return true
	}
}

func (m *pageMap) getOrCreatePagesFromCache(key string, create func(string) (page.Pages, error)) (page.Pages, error) {
	return m.cachePages.GetOrCreate(context.TODO(), key, create)
}

func (m *pageMap) getPagesInSection(q pageMapQueryPagesInSection) page.Pages {
	cacheKey := q.Key()

	pages, err := m.getOrCreatePagesFromCache(cacheKey, func(string) (page.Pages, error) {
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

				// We store both leafs and branches in the same tree, so for non-recursive walks,
				// we need to walk until the end, but can skip
				// any not belonging to child branches.
				if otherBranch != "" && strings.HasPrefix(key, otherBranch) {
					return false, nil
				}

				if p, ok := n.(*pageState); ok && predicate(p) {
					pas = append(pas, p)
				}

				if n.isContentNodeBranch() {
					otherBranch = key + "/"
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

	v, err := m.cachePages.GetOrCreate(context.TODO(), key, func(string) (page.Pages, error) {
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

				pas = append(pas, pageWithWeight0{n.weight, p})
				return false, nil
			},
		})

		if err != nil {
			return nil, err
		}

		page.SortByDefault(pas)

		return pas, nil

	})

	if err != nil {
		panic(err)
	}

	return v
}

func (m *pageMap) getTermsForPageInTaxonomy(path, taxonomy string) page.Pages {
	prefix := "/" + taxonomy // TODO1
	if path == "/" {
		path = pageTreeHome
	}

	v, err := m.cachePages.GetOrCreate(context.TODO(), prefix+path, func(string) (page.Pages, error) {
		var pas page.Pages

		err := m.treeTaxonomyEntries.Walk(context.TODO(), doctree.WalkConfig[*weightedContentNode]{
			Prefix: prefix,
			Callback: func(ctx *doctree.WalkContext[*weightedContentNode], key string, n *weightedContentNode) (bool, error) {
				if strings.HasSuffix(key, path) {
					pas = append(pas, n.term)

				}
				return false, nil
			},
		})

		if err != nil {
			return nil, err
		}

		page.SortByDefault(pas)

		return pas, nil

	})

	if err != nil {
		panic(err)
	}

	return v
}

func (m *pageMap) getResourcesForPage(ps *pageState) resource.Resources {
	key := ps.Path() + "/get-resources-for-page"
	v, err := m.cacheResources.GetOrCreate(context.TODO(), key, func(string) (resource.Resources, error) {
		prefix := ps.Path()
		if prefix != "/" {
			prefix += "/"
		}
		tree := m.treeResources
		maxLevel := -1
		if ps.IsNode() {
			maxLevel = strings.Count(prefix, "/")
		}

		targetPaths := ps.targetPaths()
		dim := m.s.h.resolveDimension(pageTreeDimensionLanguage, ps)
		if dim.IsZero() {
			panic("failed to resolve page dimension")
		}

		var res resource.Resources

		// Then collect the other resources (text files, images etc.)
		// Here we fill inn missing resources for the given language.
		err := tree.Walk(context.TODO(), doctree.WalkConfig[doctree.NodeGetter[resource.Resource]]{
			Prefix:  prefix,
			NoShift: true,
			Callback: func(ctx *doctree.WalkContext[doctree.NodeGetter[resource.Resource]], key string, n doctree.NodeGetter[resource.Resource]) (bool, error) {
				if maxLevel >= 0 && strings.Count(key, "/") > maxLevel {
					return false, nil
				}
				switch nn := n.(type) {
				case *doctree.LazySlice[*resourceSource, resource.Resource]:
					sourceIdx := dim.Index
					if !nn.HasSource(sourceIdx) {
						// TODO1 default content language
						for i := 0; i < dim.Size; i++ {
							if source, found := nn.GetSource(i); found {
								if source.path.IsContent() {
									return false, nil
								}
								sourceIdx = i
								break
							}
						}
					}

					r, err := nn.GetOrCreate(sourceIdx, dim.Index, func(rsource *resourceSource) (resource.Resource, error) {
						relPath := rsource.path.BaseRel(ps.m.pathInfo)
						if rsource.path.IsContent() {
							f, err := source.NewFileInfo(rsource.fi)
							if err != nil {
								return nil, err
							}
							pageResource, err := m.s.h.newPage(
								&pageMeta{
									f:            f,
									pathInfo:     rsource.path, // TODO1 reuse the resourceSource object.
									resourcePath: relPath,
									bundled:      true,
								},
							)
							if err != nil {
								return nil, err
							}
							// No cascade for resources.
							if err := pageResource.setMetadatPost(nil); err != nil {
								return nil, err
							}
							if err = pageResource.initPage(); err != nil {
								return nil, err
							}

							// TODO1
							pageResource.pageOutput = pageResource.pageOutputs[ps.pageOutputIdx]

							return pageResource, nil
						}

						rd := resources.ResourceSourceDescriptor{
							OpenReadSeekCloser: rsource.opener,
							Path:               rsource.path,
							RelPermalink:       path.Join(targetPaths.SubResourceBaseLink, relPath),
							TargetPath:         path.Join(targetPaths.SubResourceBaseTarget, relPath),
							Name:               relPath,
							LazyPublish:        !ps.m.buildConfig.PublishResources,
						}
						return m.s.ResourceSpec.New(rd)
					})
					if err != nil {
						return false, err
					}

					if r := r.GetNode(); r != nil {
						res = append(res, r)
					}
				default:
					panic(fmt.Sprintf("unexpected type %T", n))
				}

				return false, nil
			},
		})

		if err != nil {
			return nil, err
		}

		lessFunc := func(i, j int) bool {
			ri, rj := res[i], res[j]
			if ri.ResourceType() < rj.ResourceType() {
				return true
			}

			p1, ok1 := ri.(page.Page)
			p2, ok2 := rj.(page.Page)

			if ok1 != ok2 {
				// Pull pages behind other resources.

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

		if len(ps.m.resourcesMetadata) > 0 {
			resources.AssignMetadata(ps.m.resourcesMetadata, res...)
			sort.SliceStable(res, lessFunc)

		}

		return res, nil

	})

	if err != nil {
		panic(err)
	}

	return v
}

type weightedContentNode struct {
	n      contentNodeI
	weight int
	term   *pageWithOrdinal
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

type resourceNode interface {
}

var _ resourceNode = (*resourceNodeIs)(nil)

type resourceNodeIs []resourceNode

type notSupportedShifter struct {
}

func (s *notSupportedShifter) Shift(n doctree.NodeGetter[resource.Resource], dimension []int) (doctree.NodeGetter[resource.Resource], bool) {
	panic("not supported")
}

func (s *notSupportedShifter) All(n doctree.NodeGetter[resource.Resource]) []doctree.NodeGetter[resource.Resource] {
	panic("not supported")
}

func (s *notSupportedShifter) Dimension(n doctree.NodeGetter[resource.Resource], d int) []doctree.NodeGetter[resource.Resource] {
	panic("not supported")
}

func (s *notSupportedShifter) Insert(old, new doctree.NodeGetter[resource.Resource]) (doctree.NodeGetter[resource.Resource], bool) {
	panic("not supported")
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
		pageTrees:            s.h.pageTrees.Shape(0, i),
		cachePages:           memcache.GetOrCreatePartition[string, page.Pages](s.MemCache, fmt.Sprintf("pages/%d", i), memcache.OptionsPartition{Weight: 10, ClearWhen: memcache.ClearOnRebuild}),
		cacheResources:       memcache.GetOrCreatePartition[string, resource.Resources](s.MemCache, fmt.Sprintf("resources/%d", i), memcache.OptionsPartition{Weight: 60, ClearWhen: memcache.ClearOnRebuild}),
		cacheContentRendered: memcache.GetOrCreatePartition[string, *resources.StaleValue[contentTableOfContents]](s.MemCache, fmt.Sprintf("content-rendered/%d", i), memcache.OptionsPartition{Weight: 70, ClearWhen: memcache.ClearOnChange}),
		cacheContentPlain:    memcache.GetOrCreatePartition[string, *resources.StaleValue[plainPlainWords]](s.MemCache, fmt.Sprintf("content-plain/%d", i), memcache.OptionsPartition{Weight: 70, ClearWhen: memcache.ClearOnChange}),
		cacheContentSource:   memcache.GetOrCreatePartition[string, *resources.StaleValue[[]byte]](s.MemCache, fmt.Sprintf("content-source/%d", i), memcache.OptionsPartition{Weight: 70, ClearWhen: memcache.ClearOnChange}),

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

	m.pageReverseIndex = &contentTreeReverseIndex{
		initFn: func(rm map[any]contentNodeI) {
			add := func(k string, n contentNodeI) {
				existing, found := rm[k]
				if found && existing != ambiguousContentNode {
					rm[k] = ambiguousContentNode
				} else if !found {
					rm[k] = n
				}
			}

			m.treePages.Walk(
				context.TODO(), doctree.WalkConfig[contentNodeI]{
					LockType: doctree.LockTypeRead,
					Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
						p := n.(*pageState)
						if p.File() != nil {
							add(p.File().FileInfo().Meta().PathInfo.BaseNameNoIdentifier(), p)
						}
						return false, nil
					},
				},
			)

		},
		contentTreeReverseIndexMap: &contentTreeReverseIndexMap{},
	}

	return m
}

type contentTreeReverseIndex struct {
	initFn func(rm map[any]contentNodeI)
	*contentTreeReverseIndexMap
}

func (c *contentTreeReverseIndex) Reset() {
	c.contentTreeReverseIndexMap = &contentTreeReverseIndexMap{
		m: make(map[any]contentNodeI),
	}
}

func (c *contentTreeReverseIndex) Get(key any) contentNodeI {
	c.init.Do(func() {
		c.m = make(map[any]contentNodeI)
		c.initFn(c.contentTreeReverseIndexMap.m)
	})
	return c.m[key]
}

type contentTreeReverseIndexMap struct {
	init sync.Once
	m    map[any]contentNodeI
}

type sitePagesAssembler struct {
	*Site
	changeTracker *whatChanged
	ctx           context.Context
}

// Calculate and apply aggregate values to the page tree (e.g. dates, cascades).
func (sa *sitePagesAssembler) applyAggregates() error {
	sectionPageCount := map[string]int{}

	aggregatesWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeRead,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			p := n.(*pageState)

			if p.Kind() == pagekinds.Term {
				// Delay this.
				return false, nil
			}

			if p.IsPage() {
				rootSection := p.Section()
				sectionPageCount[rootSection]++
			}

			// Handle cascades first to get any default dates set.
			var cascade map[page.PageMatcher]maps.Params
			if s == "" {
				// Home page gets it's cascade from the site config.
				cascade = sa.cascade

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

	err := sa.pageMap.treePages.Walk(sa.ctx, aggregatesWalker)

	const mainSectionsKey = "mainsections"
	if _, found := sa.pageMap.s.Info.Params()[mainSectionsKey]; !found {
		var mainSection string
		var maxcount int
		for section, counter := range sectionPageCount {
			if section != "" && counter > maxcount {
				mainSection = section
				maxcount = counter
			}
		}
		sa.pageMap.s.Info.Params()[mainSectionsKey] = []string{mainSection}
	}

	return err

}

func (sa *sitePagesAssembler) applyCascadesToTerms() error {
	aggregatesWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeRead,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			p := n.(*pageState)
			if p.Kind() != pagekinds.Term {
				// Already handled.
				return false, nil
			}
			var cascade map[page.PageMatcher]maps.Params
			_, data := ctx.Data().LongestPrefix(s)
			if data != nil {
				cascade = data.(map[page.PageMatcher]maps.Params)
			}
			p.setMetadatPost(cascade)
			return false, nil
		},
	}
	return sa.pageMap.treePages.Walk(sa.ctx, aggregatesWalker)
}

// If the Page kind is disabled, remove any Page related node from the tree.
func (sa *sitePagesAssembler) removeDisabledKinds() error {
	cfg := sa.pageMap.cfg
	if !cfg.pageDisabled {
		// Nothing to do.
		return nil
	}
	var keys []string
	sa.pageMap.treePages.Walk(
		sa.ctx, doctree.WalkConfig[contentNodeI]{
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
		sa.pageMap.DeletePage(k)
	}

	return nil
}

// Remove any leftover node that we should not build for some reason (draft, expired, scheduled in the future).
// Note that for the home and section kinds we just disable the nodes to preserve the structure.
func (sa *sitePagesAssembler) removeShouldNotBuild() error {
	s := sa.Site
	var keys []string
	sa.pageMap.treePages.Walk(
		sa.ctx, doctree.WalkConfig[contentNodeI]{
			LockType: doctree.LockTypeRead,
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
				p := n.(*pageState)
				if !s.shouldBuild(p) {
					switch p.Kind() {
					case pagekinds.Home, pagekinds.Section:
						// We need to keep these for the structure, but disable
						// them so they don't get listed/rendered.
						(&p.m.buildConfig).Disable()
					default:
						keys = append(keys, key)
					}
				}
				return false, nil
			},
		},
	)
	for _, k := range keys {
		sa.pageMap.DeletePage(k)
	}

	return nil
}

func (sa *sitePagesAssembler) assembleTaxonomies() error {
	if sa.pageMap.cfg.taxonomyDisabled || sa.pageMap.cfg.taxonomyTermDisabled {
		return nil
	}

	var (
		pages   = sa.pageMap.treePages
		entries = sa.pageMap.treeTaxonomyEntries
		views   = sa.pageMap.cfg.taxonomyConfig.views
	)

	w := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeWrite,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			p := n.(*pageState)
			for _, viewName := range views {
				vals := types.ToStringSlicePreserveString(getParam(p, viewName.plural, false))
				if vals == nil {
					continue
				}

				w := getParamToLower(p, viewName.plural+"_weight")
				weight, err := cast.ToIntE(w)
				if err != nil {
					sa.Log.Warnf("Unable to convert taxonomy weight %#v to int for %q", w, n.Path())
					// weight will equal zero, so let the flow continue
				}

				for i, v := range vals {
					termKey := sa.getTaxonomyKey(v)
					viewTermKey := "/" + viewName.plural + "/" + termKey
					term := pages.Get(viewTermKey)
					if term == nil {
						// TODO1 error handling.
						m := &pageMeta{
							title:    v, // helpers.FirstUpper(v),
							s:        sa.Site,
							pathInfo: paths.Parse(viewTermKey),
							kind:     pagekinds.Term,
						}
						n, _ := sa.h.newPage(m)
						pages.Insert(viewTermKey, n) // TODO1 insert vs shift
						term = pages.Get(viewTermKey)
					}

					if s == "" {
						// Consider making this the real value.
						s = pageTreeHome
					}

					key := viewTermKey + s

					entries.Insert(key, &weightedContentNode{
						weight: weight,
						n:      n,
						term:   &pageWithOrdinal{pageState: term.(*pageState), ordinal: i},
					})
				}
			}
			return false, nil
		},
	}

	return pages.Walk(sa.ctx, w)
}

// // Create the fixed output pages, e.g. sitemap.xml, if not already there.
func (sa *sitePagesAssembler) addStandalonePages() error {
	s := sa.Site
	m := s.pageMap
	tree := m.treePages

	commit := tree.Lock(true)
	defer commit()

	addStandalone := func(key, kind string, f output.Format) {
		if !sa.Site.isEnabled(kind) || tree.Has(key) {
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

func (sa *sitePagesAssembler) addMissingRootSections() error {
	isBranchPredicate := func(n contentNodeI) bool {
		return n.isContentNodeBranch()
	}

	var (
		tree    = sa.pageMap.treePages
		hasHome bool
	)

	// Add missing root sections.
	seen := map[string]bool{}
	missingRootSectionsWalker := doctree.WalkConfig[contentNodeI]{
		LockType: doctree.LockTypeWrite,
		Callback: func(ctx *doctree.WalkContext[contentNodeI], s string, n contentNodeI) (bool, error) {
			if n == nil {
				panic("n is nil")
			}

			if ps, ok := n.(*pageState); ok {
				if ps.Lang() != sa.Lang() {
					panic(fmt.Sprintf("lang mismatch: %q: %s != %s", s, ps.Lang(), sa.Lang()))
				}
			}

			if s == "" {
				hasHome = true
				sa.home = n.(*pageState)
				return false, nil
			}

			p := paths.Parse(s)
			section := p.Section()
			if seen[section] {
				return false, nil
			}
			seen[section] = true

			ss, n := tree.LongestPrefix(p.Dir(), isBranchPredicate)

			if n == nil || (ss == "" && p.Dir() != "/") {
				pth := paths.Parse("/" + p.Section())
				// TODO1 error handling.
				m := &pageMeta{
					s:        sa.Site,
					pathInfo: pth,
				}
				p, _ := sa.h.newPage(m)

				tree.Insert(pth.Path(), p)
			}

			// /a/b
			// TODO1
			if strings.Count(s, "/") > 1 {
				//return true, nil
			}
			return false, nil
		},
	}

	if err := tree.Walk(sa.ctx, missingRootSectionsWalker); err != nil {
		return err
	}

	if !hasHome {
		p := paths.Parse("")
		// TODO1 error handling.
		m := &pageMeta{
			s:        sa.Site,
			pathInfo: p,
			kind:     pagekinds.Home,
		}
		n, _ := sa.h.newPage(m)
		tree.InsertWithLock(p.Path(), n)
		sa.home = n
	}

	return nil
}

func (sa *sitePagesAssembler) addMissingTaxonomies() error {
	if sa.pageMap.cfg.taxonomyDisabled {
		return nil
	}

	var tree = sa.pageMap.treePages

	commit := tree.Lock(true)
	defer commit()

	for _, viewName := range sa.pageMap.cfg.taxonomyConfig.views {
		key := viewName.pluralTreeKey
		if v := tree.Get(key); v == nil {
			m := &pageMeta{
				s:        sa.Site,
				pathInfo: paths.Parse(key),
				kind:     pagekinds.Taxonomy,
			}
			p, _ := sa.h.newPage(m)
			tree.Insert(key, p)
		}
	}

	return nil

}

func (site *Site) AssemblePages(changeTracker *whatChanged) error {
	ctx := context.TODO()

	assembler := &sitePagesAssembler{
		Site:          site,
		changeTracker: changeTracker,
		ctx:           ctx,
	}

	if err := assembler.removeDisabledKinds(); err != nil {
		return err
	}

	if err := assembler.addMissingTaxonomies(); err != nil {
		return err
	}

	if err := assembler.addMissingRootSections(); err != nil {
		return err
	}

	if err := assembler.addStandalonePages(); err != nil {
		return err
	}

	if err := assembler.applyAggregates(); err != nil {
		return err
	}

	if err := assembler.removeShouldNotBuild(); err != nil {
		return err
	}

	if err := assembler.assembleTaxonomies(); err != nil {
		return err
	}

	if err := assembler.applyCascadesToTerms(); err != nil {
		return err
	}

	return nil

}

// TODO1 make this into a delimiter to be used by all.
const pageTreeHome = "/_h"

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
							taxonomy.add(p.m.pathInfo.NameNoIdentifier(), page.NewWeightedPage(wn.weight, wn.n.(page.Page), wn.term.Page()))
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
