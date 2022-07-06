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
	"io"
	"sort"
	"strings"
	"sync"

	"go.uber.org/atomic"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/hugofs/glob"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/resources"
	"github.com/gohugoio/hugo/resources/page/pagekinds"
	"github.com/gohugoio/hugo/resources/resource"

	"github.com/fsnotify/fsnotify"

	"github.com/gohugoio/hugo/identity"

	radix "github.com/armon/go-radix"

	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/parser/metadecoders"

	"errors"

	"github.com/gohugoio/hugo/common/para"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/hugofs"

	"github.com/gohugoio/hugo/source"

	"github.com/bep/gitmap"
	"github.com/gohugoio/hugo/config"

	"github.com/gohugoio/hugo/publisher"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/langs"
	"github.com/gohugoio/hugo/lazy"

	"github.com/gohugoio/hugo/langs/i18n"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/tpl"
	"github.com/gohugoio/hugo/tpl/tplimpl"
)

// HugoSites represents the sites to build. Each site represents a language.
type HugoSites struct {
	Sites []*Site

	multilingual *Multilingual

	// Multihost is set if multilingual and baseURL set on the language level.
	multihost bool

	// If this is running in the dev server.
	running bool

	// Render output formats for all sites.
	renderFormats output.Formats

	// The currently rendered Site.
	currentSite *Site

	*deps.Deps

	gitInfo       *gitInfo
	codeownerInfo *codeownerInfo

	// As loaded from the /data dirs
	data map[string]any

	// Cache for page listings.
	cachePages *memcache.Partition[string, page.Pages]

	pageTrees *pageTrees

	// Keeps track of bundle directories and symlinks to enable partial rebuilding.
	ContentChanges *contentChangeMap

	// File change events with filename stored in this map will be skipped.
	skipRebuildForFilenamesMu sync.Mutex
	skipRebuildForFilenames   map[string]bool

	init *hugoSitesInit

	workers    *para.Workers
	numWorkers int

	*fatalErrorHandler
	buildCounters *buildCounters
}

// ShouldSkipFileChangeEvent allows skipping filesystem event early before
// the build is started.
func (h *HugoSites) ShouldSkipFileChangeEvent(ev fsnotify.Event) bool {
	h.skipRebuildForFilenamesMu.Lock()
	defer h.skipRebuildForFilenamesMu.Unlock()
	return h.skipRebuildForFilenames[ev.Name]
}

type buildCounters struct {
	contentRender atomic.Uint64
	pageRender    atomic.Uint64
}

type fatalErrorHandler struct {
	mu sync.Mutex

	h *HugoSites

	err error

	done  bool
	donec chan bool // will be closed when done
}

// FatalError error is used in some rare situations where it does not make sense to
// continue processing, to abort as soon as possible and log the error.
func (f *fatalErrorHandler) FatalError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.done {
		f.done = true
		close(f.donec)
	}
	f.err = err
}

func (f *fatalErrorHandler) getErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fatalErrorHandler) Done() <-chan bool {
	return f.donec
}

type hugoSitesInit struct {
	// Loads the data from all of the /data folders.
	data *lazy.Init

	// Performs late initialization (before render) of the templates.
	layouts *lazy.Init

	// Loads the Git info and CODEOWNERS for all the pages if enabled.
	gitInfo *lazy.Init
}

func (h *hugoSitesInit) Reset() {
	h.data.Reset()
	h.layouts.Reset()
	h.gitInfo.Reset()
}

func (h *HugoSites) Data() map[string]any {
	if _, err := h.init.data.Do(); err != nil {
		h.SendError(fmt.Errorf("failed to load data: %w", err))
		return nil
	}
	return h.data
}

func (h *HugoSites) gitInfoForPage(p page.Page) (*gitmap.GitInfo, error) {
	if _, err := h.init.gitInfo.Do(); err != nil {
		return nil, err
	}

	if h.gitInfo == nil {
		return nil, nil
	}

	return h.gitInfo.forPage(p), nil
}

func (h *HugoSites) codeownersForPage(p page.Page) ([]string, error) {
	if _, err := h.init.gitInfo.Do(); err != nil {
		return nil, err
	}

	if h.codeownerInfo == nil {
		return nil, nil
	}

	return h.codeownerInfo.forPage(p), nil
}

func (h *HugoSites) siteInfos() page.Sites {
	infos := make(page.Sites, len(h.Sites))
	for i, site := range h.Sites {
		infos[i] = site.Info
	}
	return infos
}

func (h *HugoSites) pickOneAndLogTheRest(errors []error) error {
	if len(errors) == 0 {
		return nil
	}

	var i int

	for j, err := range errors {
		// If this is in server mode, we want to return an error to the client
		// with a file context, if possible.
		if herrors.UnwrapFileError(err) != nil {
			i = j
			break
		}
	}

	// Log the rest, but add a threshold to avoid flooding the log.
	const errLogThreshold = 5

	for j, err := range errors {
		if j == i || err == nil {
			continue
		}

		if j >= errLogThreshold {
			break
		}

		h.Log.Errorln(err)
	}

	return errors[i]
}

func (h *HugoSites) IsMultihost() bool {
	return h != nil && h.multihost
}

// TODO(bep) consolidate
func (h *HugoSites) LanguageSet() map[string]int {
	set := make(map[string]int)
	for i, s := range h.Sites {
		set[s.language.Lang] = i
	}
	return set
}

func (h *HugoSites) NumLogErrors() int {
	if h == nil {
		return 0
	}
	return int(h.Log.LogCounters().ErrorCounter.Count())
}

func (h *HugoSites) PrintProcessingStats(w io.Writer) {
	stats := make([]*helpers.ProcessingStats, len(h.Sites))
	for i := 0; i < len(h.Sites); i++ {
		stats[i] = h.Sites[i].PathSpec.ProcessingStats
	}
	helpers.ProcessingStatsTable(w, stats...)
}

// GetContentPage finds a Page with content given the absolute filename.
// Returns nil if none found.
func (h *HugoSites) GetContentPage(filename string) page.Page {
	var p page.Page

	h.withPage(func(s string, p2 *pageState) bool {
		if p2.File() == nil {
			return false
		}
		if p2.File().FileInfo().Meta().Filename == filename {
			p = p2
			return true
		}

		for _, r := range p2.Resources().ByType(pageResourceType) {
			p3 := r.(page.Page)
			if p3.File() != nil && p3.File().FileInfo().Meta().Filename == filename {
				p = p3
				return true
			}
		}

		return false
	})

	return p
}

// NewHugoSites creates a new collection of sites given the input sites, building
// a language configuration based on those.
func newHugoSites(cfg deps.DepsCfg, sites ...*Site) (*HugoSites, error) {
	if cfg.Language != nil {
		return nil, errors.New("Cannot provide Language in Cfg when sites are provided")
	}

	// Return error at the end. Make the caller decide if it's fatal or not.
	var initErr error

	langConfig, err := newMultiLingualFromSites(cfg.Cfg, sites...)
	if err != nil {
		return nil, fmt.Errorf("failed to create language config: %w", err)
	}

	var contentChangeTracker *contentChangeMap

	numWorkers := config.GetNumWorkerMultiplier()
	if numWorkers > len(sites) {
		numWorkers = len(sites)
	}
	var workers *para.Workers
	if numWorkers > 1 {
		workers = para.New(numWorkers)
	}

	h := &HugoSites{
		running:                 cfg.Running,
		multilingual:            langConfig,
		multihost:               cfg.Cfg.GetBool("multihost"),
		Sites:                   sites,
		workers:                 workers,
		numWorkers:              numWorkers,
		skipRebuildForFilenames: make(map[string]bool),
		init: &hugoSitesInit{
			data:    lazy.New(),
			layouts: lazy.New(),
			gitInfo: lazy.New(),
		},
	}

	h.fatalErrorHandler = &fatalErrorHandler{
		h:     h,
		donec: make(chan bool),
	}

	h.init.data.Add(func() (any, error) {
		err := h.loadData(h.PathSpec.BaseFs.Data.Dirs)
		if err != nil {
			return nil, fmt.Errorf("failed to load data: %w", err)
		}
		return nil, nil
	})

	h.init.layouts.Add(func() (any, error) {
		for _, s := range h.Sites {
			if err := s.Tmpl().(tpl.TemplateManager).MarkReady(); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})

	h.init.gitInfo.Add(func() (any, error) {
		err := h.loadGitInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to load Git info: %w", err)
		}
		return nil, nil
	})

	for _, s := range sites {
		s.h = h
	}

	var l configLoader
	if err := l.applyDeps(cfg, sites...); err != nil {
		initErr = fmt.Errorf("add site dependencies: %w", err)
	}

	h.Deps = sites[0].Deps
	if h.Deps == nil {
		return nil, initErr
	}

	h.cachePages = memcache.GetOrCreatePartition[string, page.Pages](
		h.Deps.MemCache,
		"hugo-sites-pages",
		memcache.OptionsPartition{Weight: 10, ClearWhen: memcache.ClearOnRebuild},
	)

	// Only needed in server mode.
	// TODO(bep) clean up the running vs watching terms
	if cfg.Running {
		contentChangeTracker = &contentChangeMap{
			pathSpec:      h.PathSpec,
			symContent:    make(map[string]map[string]bool),
			leafBundles:   radix.New(),
			branchBundles: make(map[string]bool),
		}
		h.ContentChanges = contentChangeTracker
	}

	return h, initErr
}

func (h *HugoSites) loadGitInfo() error {
	if h.Cfg.GetBool("enableGitInfo") {
		gi, err := newGitInfo(h.Cfg)
		if err != nil {
			h.Log.Errorln("Failed to read Git log:", err)
		} else {
			h.gitInfo = gi
		}

		co, err := newCodeOwners(h.Cfg)
		if err != nil {
			h.Log.Errorln("Failed to read CODEOWNERS:", err)
		} else {
			h.codeownerInfo = co
		}
	}
	return nil
}

func (l configLoader) applyDeps(cfg deps.DepsCfg, sites ...*Site) error {
	if cfg.TemplateProvider == nil {
		cfg.TemplateProvider = tplimpl.DefaultTemplateProvider
	}

	if cfg.TranslationProvider == nil {
		cfg.TranslationProvider = i18n.NewTranslationProvider()
	}

	var (
		d   *deps.Deps
		err error
	)

	for i, s := range sites {
		if s.Deps != nil {
			continue
		}

		onCreated := func(d *deps.Deps) error {
			s.Deps = d

			// Set up the main publishing chain.
			pub, err := publisher.NewDestinationPublisher(
				d.ResourceSpec,
				s.outputFormatsConfig,
				s.mediaTypesConfig,
			)
			if err != nil {
				return err
			}
			s.publisher = pub

			if err := s.initializeSiteInfo(); err != nil {
				return err
			}

			d.Site = s.Info

			siteConfig, err := l.loadSiteConfig(s.language)
			if err != nil {
				return fmt.Errorf("load site config: %w", err)
			}
			s.siteConfigConfig = siteConfig

			if s.h.pageTrees == nil {
				langIntToLang := map[int]string{}
				langLangToInt := map[string]int{}

				for i, s := range sites {
					langIntToLang[i] = s.language.Lang
					langLangToInt[s.language.Lang] = i
				}

				dimensions := []int{0} // language

				pageTreeConfig := doctree.Config[contentNodeI]{
					Dimensions: dimensions,
					Shifter:    &contentNodeShifter{langIntToLang: langIntToLang, langLangToInt: langLangToInt},
				}

				resourceTreeConfig := doctree.Config[doctree.NodeGetter[resource.Resource]]{
					Dimensions: dimensions,
					Shifter:    &notSupportedShifter{},
				}

				taxonomyEntriesTreeConfig := doctree.Config[*weightedContentNode]{
					Dimensions: []int{0}, // Language
					Shifter:    &weightedContentNodeShifter{},
				}

				s.h.pageTrees = &pageTrees{
					treePages: doctree.New(
						pageTreeConfig,
					),
					treeResources: doctree.New(
						resourceTreeConfig,
					),
					treeTaxonomyEntries: doctree.New(
						taxonomyEntriesTreeConfig,
					),
				}

				s.h.pageTrees.resourceTrees = doctree.MutableTrees{
					s.h.pageTrees.treeResources,
					s.h.pageTrees.treeTaxonomyEntries,
				}
			}

			pm := newPageMap(i, s)

			s.pageFinder = newPageFinder(pm)
			s.siteRefLinker, err = newSiteRefLinker(s.language, s)
			return err
		}

		cfg.Language = s.language
		cfg.MediaTypes = s.mediaTypesConfig
		cfg.OutputFormats = s.outputFormatsConfig

		if d == nil {
			cfg.WithTemplate = s.withSiteTemplates(cfg.WithTemplate)

			var err error
			d, err = deps.New(cfg)
			if err != nil {
				return fmt.Errorf("create deps: %w", err)
			}

			d.OutputFormatsConfig = s.outputFormatsConfig

			if err := onCreated(d); err != nil {
				return fmt.Errorf("on created: %w", err)
			}

			if err = d.LoadResources(); err != nil {
				return fmt.Errorf("load resources: %w", err)
			}

		} else {
			d, err = d.ForLanguage(cfg, onCreated)
			if err != nil {
				return err
			}
			d.OutputFormatsConfig = s.outputFormatsConfig
		}
	}

	return nil
}

// NewHugoSites creates HugoSites from the given config.
func NewHugoSites(cfg deps.DepsCfg) (*HugoSites, error) {
	if cfg.Logger == nil {
		cfg.Logger = loggers.NewErrorLogger()
	}
	if cfg.Fs == nil {
		return nil, errors.New("no filesystem given")
	}
	sites, err := createSitesFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("from config: %w", err)
	}

	return newHugoSites(cfg, sites...)
}

func (s *Site) withSiteTemplates(withTemplates ...func(templ tpl.TemplateManager) error) func(templ tpl.TemplateManager) error {
	return func(templ tpl.TemplateManager) error {
		for _, wt := range withTemplates {
			if wt == nil {
				continue
			}
			if err := wt(templ); err != nil {
				return err
			}
		}

		return nil
	}
}

func createSitesFromConfig(cfg deps.DepsCfg) ([]*Site, error) {
	var sites []*Site

	languages := getLanguages(cfg.Cfg)

	for i, lang := range languages {
		if lang.Disabled {
			continue
		}
		var s *Site
		var err error
		cfg.Language = lang
		s, err = newSite(i, cfg)

		if err != nil {
			return nil, err
		}

		sites = append(sites, s)
	}

	return sites, nil
}

// Reset resets the sites and template caches etc., making it ready for a full rebuild.
// TODO1
func (h *HugoSites) reset(config *BuildCfg) {
	if config.ResetState {
		for i, s := range h.Sites {
			h.Sites[i] = s.reset()
			if r, ok := s.Fs.PublishDir.(hugofs.Reseter); ok {
				r.Reset()
			}
		}
	}

	h.fatalErrorHandler = &fatalErrorHandler{
		h:     h,
		donec: make(chan bool),
	}

	h.init.Reset()
}

// resetLogs resets the log counters etc. Used to do a new build on the same sites.
func (h *HugoSites) resetLogs() {
	h.Log.Reset()
	loggers.GlobalErrorCounter.Reset()
	for _, s := range h.Sites {
		s.Deps.Log.Reset()
		s.Deps.LogDistinct.Reset()
	}
}

func (h *HugoSites) withSite(para bool, fn func(s *Site) error) error {
	if !para || h.workers == nil {
		for _, s := range h.Sites {
			if err := fn(s); err != nil {
				return err
			}
		}
		return nil
	}

	g, _ := h.workers.Start(context.Background())
	for _, s := range h.Sites {
		s := s
		g.Run(func() error {
			return fn(s)
		})
	}
	return g.Wait()
}

func (h *HugoSites) withPage(fn func(s string, p *pageState) bool) {
	h.withSite(true, func(s *Site) error {
		s.pageMap.treePages.Walk(context.TODO(), doctree.WalkConfig[contentNodeI]{
			LockType: doctree.LockTypeRead,
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
				return fn(key, n.(*pageState)), nil
			},
		})

		return nil
	})
}

// getPageFirstDimension returns the first dimension of the page.
// Use this if you don't really care about the dimension.
func (h *HugoSites) getPageFirstDimension(s string) *pageState {
	all := h.Sites[0].pageMap.treePages.GetAll(s)
	if len(all) == 0 {
		return nil
	}
	return all[0].(*pageState)
}

func (h *HugoSites) createSitesFromConfig(cfg config.Provider) error {
	oldLangs, _ := h.Cfg.Get("languagesSorted").(langs.Languages)

	l := configLoader{cfg: h.Cfg}
	if err := l.loadLanguageSettings(oldLangs); err != nil {
		return err
	}

	depsCfg := deps.DepsCfg{Fs: h.Fs, Cfg: l.cfg}

	sites, err := createSitesFromConfig(depsCfg)
	if err != nil {
		return err
	}

	langConfig, err := newMultiLingualFromSites(depsCfg.Cfg, sites...)
	if err != nil {
		return err
	}

	h.Sites = sites

	for _, s := range sites {
		s.h = h
	}

	var cl configLoader
	if err := cl.applyDeps(depsCfg, sites...); err != nil {
		return err
	}

	h.Deps = sites[0].Deps

	h.multilingual = langConfig
	h.multihost = h.Deps.Cfg.GetBool("multihost")

	return nil
}

func (h *HugoSites) GetDependencyManager() identity.Manager {
	// TODO1 consider this
	return identity.NopManager
}

type siteInfos []*SiteInfo

func (s siteInfos) GetDependencyManager() identity.Manager {
	return s[0].s.h.GetDependencyManager()
}

func (h *HugoSites) toSiteInfos() siteInfos {
	infos := make(siteInfos, len(h.Sites))
	for i, s := range h.Sites {
		infos[i] = s.Info
	}
	return infos
}

// BuildCfg holds build options used to, as an example, skip the render step.
type BuildCfg struct {
	// Reset site state before build. Use to force full rebuilds.
	ResetState bool
	// If set, we re-create the sites from the given configuration before a build.
	// This is needed if new languages are added.
	NewConfig config.Provider
	// Skip rendering. Useful for testing.
	SkipRender bool
	// Use this to indicate what changed (for rebuilds).
	whatChanged *whatChanged

	// This is a partial re-render of some selected pages. This means
	// we should skip most of the processing.
	PartialReRender bool

	// Set in server mode when the last build failed for some reason.
	ErrRecovery bool

	// Recently visited URLs. This is used for partial re-rendering.
	RecentlyVisited map[string]bool

	// Can be set to build only with a sub set of the content source.
	ContentInclusionFilter *glob.FilenameFilter

	// Set when the buildlock is already acquired (e.g. the archetype content builder).
	NoBuildLock bool
}

// shouldRender is used in the Fast Render Mode to determine if we need to re-render
// a Page: If it is recently visited (the home pages will always be in this set) or changed.
// Note that a page does not have to have a content page / file.
// For regular builds, this will allways return true.
// TODO(bep) rename/work this.
func (cfg *BuildCfg) shouldRender(p *pageState) bool {
	return p.renderState == 0
	/*
		if p.forceRender {
			//panic("TODO1")
		}

		if len(cfg.RecentlyVisited) == 0 {
			return true
		}

		if cfg.RecentlyVisited[p.RelPermalink()] {
			return true
		}

		// TODO1 stale?

		return false*/
}

func (h *HugoSites) resolveDimension(d int, v any) doctree.Dimension {
	if d != pageTreeDimensionLanguage {
		panic("dimension not supported")
	}
	switch vv := v.(type) {
	case *pageState:
		return h.resolveDimension(d, vv.s)
	case *Site:
		return doctree.Dimension{
			Name:      vv.Language().Lang,
			Dimension: d,
			Index:     vv.languageIndex,
			Size:      len(h.Sites),
		}
	case resources.PathInfoProvder:
		return h.resolveDimension(d, vv.PathInfo())
	case *paths.Path:
		lang := vv.Lang()
		languageIndex := -1

		for _, s := range h.Sites {
			if s.Language().Lang == lang {
				languageIndex = s.languageIndex
				break
			}
		}

		// TODO defaultContentLanguage.
		if languageIndex == -1 {
			lang = h.Sites[0].Lang()
			languageIndex = h.Sites[0].languageIndex
		}

		return doctree.Dimension{
			Name:      lang,
			Dimension: d,
			Index:     languageIndex,
			Size:      len(h.Sites),
		}
	default:
		panic(fmt.Sprintf("unsupported type %T", v))

	}

}

// TODO(bep) improve this.
func (h *HugoSites) renderCrossSitesSitemap() error {
	if !h.multilingual.enabled() || h.IsMultihost() {
		return nil
	}

	sitemapEnabled := false
	for _, s := range h.Sites {
		if s.isEnabled(pagekinds.Sitemap) {
			sitemapEnabled = true
			break
		}
	}

	if !sitemapEnabled {
		return nil
	}

	s := h.Sites[0]

	templ := s.lookupLayouts("sitemapindex.xml", "_default/sitemapindex.xml", "_internal/_default/sitemapindex.xml")

	return s.renderAndWriteXML(&s.PathSpec.ProcessingStats.Sitemaps, "sitemapindex",
		s.siteCfg.sitemap.Filename, h.toSiteInfos(), templ)
}

func (h *HugoSites) removePageByFilename(filename string) error {
	// TODO1
	/*exclude := func(s string, n *contentNode) bool {
		if n.p == nil {
			return true
		}

		fi := n.FileInfo()
		if fi == nil {
			return true
		}

		return fi.Meta().Filename != filename
	}*/

	return nil

	/*
		return h.getContentMaps().withMaps(func(runner para.Runner, m *pageMapOld) error {
			var sectionsToDelete []string
			var pagesToDelete []contentTreeRefProvider

			q := branchMapQuery{
				Exclude: exclude,
				Branch: branchMapQueryCallBacks{
					Key: newBranchMapQueryKey("", true),
					Page: func(np contentNodeProvider) bool {
						sectionsToDelete = append(sectionsToDelete, np.Key())
						return false
					},
				},
				Leaf: branchMapQueryCallBacks{
					Page: func(np contentNodeProvider) bool {
						n := np.GetNode()
						pagesToDelete = append(pagesToDelete, n.p.m.treeRef)
						return false
					},
				},
			}

			if err := m.Walk(q); err != nil {
				return err
			}

			// Delete pages and sections marked for deletion.
			for _, p := range pagesToDelete {
				p.GetBranch().pages.nodes.Delete(p.Key())
				p.GetBranch().pageResources.nodes.Delete(p.Key() + "/")
				if !p.GetBranch().n.HasFi() && p.GetBranch().pages.nodes.Len() == 0 {
					// Delete orphan section.
					sectionsToDelete = append(sectionsToDelete, p.GetBranch().n.key)
				}
			}

			for _, s := range sectionsToDelete {
				m.branches.Delete(s)
				m.branches.DeletePrefix(s + "/")
			}

			return nil
		})
	*/
}

func (s *Site) preparePagesForRender(isRenderingSite bool, idx int) error {
	var err error

	initPage := func(p *pageState) error {
		// TODO1 err handling this and all walks
		if err := p.initPage(); err != nil {
			return err
		}
		if err = p.initOutputFormat(isRenderingSite, idx); err != nil {
			return err
		}

		return nil

	}

	err = s.pageMap.treePages.Walk(
		context.TODO(),
		doctree.WalkConfig[contentNodeI]{
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {
				if p, ok := n.(*pageState); ok {
					if p == nil {
						panic("nil page")
					}
					// TODO1 page resources?
					if err := initPage(p); err != nil {
						return true, err
					}
				}
				return false, nil
			},
		},
	)

	if err != nil {
		return err
	}

	return nil

}

// Pages returns all pages for all sites.
func (h *HugoSites) Pages() page.Pages {
	key := "pages"
	v, err := h.cachePages.GetOrCreate(context.TODO(), key, func(string) (page.Pages, error) {
		var pages page.Pages
		for _, s := range h.Sites {
			pages = append(pages, s.Pages()...)
		}
		page.SortByDefault(pages)
		return pages, nil
	})
	if err != nil {
		panic(err)
	}
	return v
}

// Pages returns all regularpages for all sites.
func (h *HugoSites) RegularPages() page.Pages {
	key := "regular-pages"
	v, err := h.cachePages.GetOrCreate(context.TODO(), key, func(string) (page.Pages, error) {
		var pages page.Pages
		for _, s := range h.Sites {
			pages = append(pages, s.RegularPages()...)
		}
		page.SortByDefault(pages)

		return pages, nil
	})

	if err != nil {
		panic(err)
	}
	return v
}

func (h *HugoSites) loadData(fis []hugofs.FileMetaDirEntry) (err error) {
	spec := source.NewSourceSpec(h.PathSpec, nil, nil)

	h.data = make(map[string]any)
	for _, fi := range fis {
		src := spec.NewFilesystemFromFileMetaDirEntry(fi)
		err := src.Walk(func(file *source.File) error {
			return h.handleDataFile(file)
		})
		if err != nil {
			return err
		}
	}

	return
}

func (h *HugoSites) handleDataFile(r *source.File) error {
	var current map[string]any

	f, err := r.FileInfo().Meta().Open()
	if err != nil {
		return fmt.Errorf("data: failed to open %q: %w", r.LogicalName(), err)
	}
	defer f.Close()

	// Crawl in data tree to insert data
	current = h.data
	keyParts := strings.Split(r.Dir(), helpers.FilePathSeparator)

	for _, key := range keyParts {
		if key != "" {
			if _, ok := current[key]; !ok {
				current[key] = make(map[string]any)
			}
			current = current[key].(map[string]any)
		}
	}

	data, err := h.readData(r)
	if err != nil {
		return h.errWithFileContext(err, r)
	}

	if data == nil {
		return nil
	}

	// filepath.Walk walks the files in lexical order, '/' comes before '.'
	higherPrecedentData := current[r.BaseFileName()]

	switch data.(type) {
	case nil:
	case map[string]any:

		switch higherPrecedentData.(type) {
		case nil:
			current[r.BaseFileName()] = data
		case map[string]any:
			// merge maps: insert entries from data for keys that
			// don't already exist in higherPrecedentData
			higherPrecedentMap := higherPrecedentData.(map[string]any)
			for key, value := range data.(map[string]any) {
				if _, exists := higherPrecedentMap[key]; exists {
					// this warning could happen if
					// 1. A theme uses the same key; the main data folder wins
					// 2. A sub folder uses the same key: the sub folder wins
					// TODO(bep) figure out a way to detect 2) above and make that a WARN
					h.Log.Infof("Data for key '%s' in path '%s' is overridden by higher precedence data already in the data tree", key, r.Path())
				} else {
					higherPrecedentMap[key] = value
				}
			}
		default:
			// can't merge: higherPrecedentData is not a map
			h.Log.Warnf("The %T data from '%s' overridden by "+
				"higher precedence %T data already in the data tree", data, r.Path(), higherPrecedentData)
		}

	case []any:
		if higherPrecedentData == nil {
			current[r.BaseFileName()] = data
		} else {
			// we don't merge array data
			h.Log.Warnf("The %T data from '%s' overridden by "+
				"higher precedence %T data already in the data tree", data, r.Path(), higherPrecedentData)
		}

	default:
		h.Log.Errorf("unexpected data type %T in file %s", data, r.LogicalName())
	}

	return nil
}

func (h *HugoSites) errWithFileContext(err error, f *source.File) error {
	fim := f.FileInfo()
	realFilename := fim.Meta().Filename

	return herrors.NewFileErrorFromFile(err, realFilename, h.SourceSpec.Fs.Source, nil)

}

func (h *HugoSites) readData(f *source.File) (any, error) {
	file, err := f.FileInfo().Meta().Open()
	if err != nil {
		return nil, fmt.Errorf("readData: failed to open data file: %w", err)
	}
	defer file.Close()
	content := helpers.ReaderToBytes(file)

	format := metadecoders.FormatFromString(f.Ext())
	return metadecoders.Default.Unmarshal(content, format)
}

func (h *HugoSites) resetPageRenderStateForIdentities(ids ...identity.Identity) {
	if ids == nil {
		return
	}

	h.withPage(func(s string, p *pageState) bool {
		var probablyDependent bool
		for _, id := range ids {
			if !identity.IsNotDependent(p, id) {
				probablyDependent = true
				break
			}
		}

		if probablyDependent {
			// This will re-render the top level Page.
			for _, po := range p.pageOutputs {
				po.renderState = 0
			}
		}

		// We may also need to re-render one or more .Content
		// for this Page's output formats (e.g. when a shortcode template changes).
	OUTPUTS:
		for _, po := range p.pageOutputs {
			if po.cp == nil {
				continue
			}
			for _, id := range ids {
				if !identity.IsNotDependent(po.GetDependencyManager(), id) {
					po.cp.Reset()
					po.renderState = 0
					continue OUTPUTS
				}
			}
		}

		return false
	})

}

// Used in partial reloading to determine if the change is in a bundle.
type contentChangeMap struct {
	// Holds directories with leaf bundles.
	leafBundles *radix.Tree

	// Holds directories with branch bundles.
	branchBundles map[string]bool

	pathSpec *helpers.PathSpec

	// Hugo supports symlinked content (both directories and files). This
	// can lead to situations where the same file can be referenced from several
	// locations in /content -- which is really cool, but also means we have to
	// go an extra mile to handle changes.
	// This map is only used in watch mode.
	// It maps either file to files or the real dir to a set of content directories
	// where it is in use.
	// TODO1 replace all of this with DependencyManager
	symContentMu sync.Mutex
	symContent   map[string]map[string]bool
}

// TODO1 remove
/*
func (m *contentChangeMap) add(dirname string, tp bundleDirType) {
	m.mu.Lock()
	if !strings.HasSuffix(dirname, helpers.FilePathSeparator) {
		dirname += helpers.FilePathSeparator
	}
	switch tp {
	case bundleBranch:
		m.branchBundles[dirname] = true
	case bundleLeaf:
		m.leafBundles.Insert(dirname, true)
	default:
		m.mu.Unlock()
		panic("invalid bundle type")
	}
	m.mu.Unlock()
}
*/

// TODO1 add test for this and replace this. Also re remove.
func (m *contentChangeMap) addSymbolicLinkMapping(fim hugofs.FileMetaDirEntry) {
	meta := fim.Meta()
	if !meta.IsSymlink {
		return
	}
	m.symContentMu.Lock()

	from, to := meta.Filename, meta.OriginalFilename
	if fim.IsDir() {
		if !strings.HasSuffix(from, helpers.FilePathSeparator) {
			from += helpers.FilePathSeparator
		}
	}

	mm, found := m.symContent[from]

	if !found {
		mm = make(map[string]bool)
		m.symContent[from] = mm
	}
	mm[to] = true
	m.symContentMu.Unlock()
}

func (m *contentChangeMap) GetSymbolicLinkMappings(dir string) []string {
	mm, found := m.symContent[dir]
	if !found {
		return nil
	}
	dirs := make([]string, len(mm))
	i := 0
	for dir := range mm {
		dirs[i] = dir
		i++
	}

	sort.Strings(dirs)

	return dirs
}

// Debug helper.
func printInfoAboutHugoSites(h *HugoSites) {
	fmt.Println("Num sites:", len(h.Sites))

	for i, s := range h.Sites {
		fmt.Printf("Site %d: %s\n", i, s.Lang())
		s.pageMap.treePages.Walk(context.TODO(), doctree.WalkConfig[contentNodeI]{
			Callback: func(ctx *doctree.WalkContext[contentNodeI], key string, n contentNodeI) (bool, error) {

				p := n.(*pageState)
				kind := p.Kind()
				var filename string
				if p.File() != nil {
					filename = p.File().Path()
				}

				fmt.Printf("\t%q [%s] %s\n", key, kind, filename)

				/*resourceTree := s.pageMap.treeLeafResources
				if n.isContentNodeBranch() {
					resourceTree = s.pageMap.treeBranchResources
				}
				resourceTree.Walk(context.TODO(), doctree.WalkConfig[doctree.Getter[resource.Resource]]{
					Prefix: key + "/",
					Callback: func(ctx *doctree.WalkContext[doctree.Getter[resource.Resource]], key string, n doctree.Getter[resource.Resource]) (bool, error) {
						fmt.Println("\t\t", key, n.(resource.Resource).RelPermalink())
						return false, nil
					},
				})*/

				return false, nil

			},
		})

	}

}
