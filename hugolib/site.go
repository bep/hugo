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
	"html/template"
	"io"
	"log"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

<<<<<<< HEAD
	"github.com/gohugoio/hugo/common/htime"
	"github.com/gohugoio/hugo/common/hugio"
=======
	"github.com/gohugoio/hugo/cache/memcache"

>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/modules"
	"golang.org/x/text/unicode/norm"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/common/constants"

	"github.com/gohugoio/hugo/common/loggers"

	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/markup/converter/hooks"

	"github.com/gohugoio/hugo/resources/resource"

	"github.com/gohugoio/hugo/markup/converter"

	"github.com/gohugoio/hugo/hugofs/files"

	"github.com/gohugoio/hugo/common/maps"

	"github.com/gohugoio/hugo/common/text"

	"github.com/gohugoio/hugo/common/hugo"
	"github.com/gohugoio/hugo/publisher"
<<<<<<< HEAD
=======
	"github.com/gohugoio/hugo/resources/page/pagekinds"
	"github.com/gohugoio/hugo/resources/page/siteidentities"
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

	"github.com/gohugoio/hugo/langs"

	"github.com/gohugoio/hugo/resources/page"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/lazy"

	"github.com/gohugoio/hugo/media"

	"github.com/fsnotify/fsnotify"
	bp "github.com/gohugoio/hugo/bufferpool"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/navigation"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/related"
	"github.com/gohugoio/hugo/resources/page/pagemeta"
	"github.com/gohugoio/hugo/source"
	"github.com/gohugoio/hugo/tpl"

	"github.com/spf13/afero"
	"github.com/spf13/cast"
)

// Site contains all the information relevant for constructing a static
// site.  The basic flow of information is as follows:
//
// 1. A list of Files is parsed and then converted into Pages.
//
// 2. Pages contain sections (based on the file they were generated from),
//    aliases and slugs (included in a pages frontmatter) which are the
//    various targets that will get generated.  There will be canonical
//    listing.  The canonical path can be overruled based on a pattern.
//
// 3. Taxonomies are created via configuration and will present some aspect of
//    the final page and typically a perm url.
//
// 4. All Pages are passed through a template based on their desired
//    layout based on numerous different elements.
//
// 5. The entire collection of files is written to disk.
type Site struct {

	// The owning container. When multiple languages, there will be multiple
	// sites .
	h *HugoSites

	*PageCollections

	taxonomies TaxonomyList

	Info *SiteInfo

	language   *langs.Language
	siteBucket *pagesMapBucket

	siteCfg siteConfigHolder

	disabledKinds map[string]bool

	// Output formats defined in site config per Page Kind, or some defaults
	// if not set.
	// Output formats defined in Page front matter will override these.
	outputFormats map[string]output.Formats

	// All the output formats and media types available for this site.
	// These values will be merged from the Hugo defaults, the site config and,
	// finally, the language settings.
	outputFormatsConfig output.Formats
	mediaTypesConfig    media.Types

	siteConfigConfig SiteConfig

	// How to handle page front matter.
	frontmatterHandler pagemeta.FrontMatterHandler

	// We render each site for all the relevant output formats in serial with
	// this rendering context pointing to the current one.
	rc *siteRenderingContext

	// The output formats that we need to render this site in. This slice
	// will be fixed once set.
	// This will be the union of Site.Pages' outputFormats.
	// This slice will be sorted.
	renderFormats output.Formats

	// Logger etc.
	*deps.Deps `json:"-"`

	// The func used to title case titles.
	titleFunc func(s string) string

	relatedDocsHandler *page.RelatedDocsHandler
	siteRefLinker

	publisher publisher.Publisher

	menus navigation.Menus

	// Shortcut to the home page. Note that this may be nil if
	// home page, for some odd reason, is disabled.
	home *pageState

	// The last modification date of this site.
	lastmod time.Time

	// Lazily loaded site dependencies
	init *siteInit
}

func (s *Site) Taxonomies() TaxonomyList {
	s.init.taxonomies.Do()
	return s.taxonomies
}

type (
	taxonomiesConfig       map[string]string
	taxonomiesConfigValues struct {
		views          []viewName
		viewsByTreeKey map[string]viewName
	}
)

func (t taxonomiesConfig) Values() taxonomiesConfigValues {
	var views []viewName
	for k, v := range t {
		views = append(views, viewName{singular: k, plural: v, pluralTreeKey: cleanTreeKey(v)})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].plural < views[j].plural
	})

	viewsByTreeKey := make(map[string]viewName)
	for _, v := range views {
		viewsByTreeKey[v.pluralTreeKey] = v
	}

	return taxonomiesConfigValues{
		views:          views,
		viewsByTreeKey: viewsByTreeKey,
	}
}

type siteConfigHolder struct {
	sitemap          config.Sitemap
	taxonomiesConfig taxonomiesConfig
	timeout          time.Duration
	hasCJKLanguage   bool
	enableEmoji      bool
}

// Lazily loaded site dependencies.
type siteInit struct {
	prevNext          *lazy.Init
	prevNextInSection *lazy.Init
	menus             *lazy.Init
	taxonomies        *lazy.Init
}

func (init *siteInit) Reset() {
	init.prevNext.Reset()
	init.prevNextInSection.Reset()
	init.menus.Reset()
	init.taxonomies.Reset()
}

func (s *Site) initInit(init *lazy.Init, pctx pageContext) bool {
	_, err := init.Do()
	if err != nil {
		s.h.FatalError(pctx.wrapError(err))
	}
	return err == nil
}

func (s *Site) prepareInits() {
	s.init = &siteInit{}

	var init lazy.Init

	s.init.prevNext = init.Branch(func() (any, error) {
		regularPages := s.RegularPages()
		for i, p := range regularPages {
			np, ok := p.(nextPrevProvider)
			if !ok {
				continue
			}

			pos := np.getNextPrev()
			if pos == nil {
				continue
			}

			pos.nextPage = nil
			pos.prevPage = nil

			if i > 0 {
				pos.nextPage = regularPages[i-1]
			}

			if i < len(regularPages)-1 {
				pos.prevPage = regularPages[i+1]
			}
		}
		return nil, nil
	})

<<<<<<< HEAD
	s.init.prevNextInSection = init.Branch(func() (any, error) {
		var sections page.Pages
		s.home.treeRef.m.collectSectionsRecursiveIncludingSelf(pageMapQuery{Prefix: s.home.treeRef.key}, func(n *contentNode) {
			sections = append(sections, n.p)
		})

=======
	s.init.prevNextInSection = init.Branch(func() (interface{}, error) {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		setNextPrev := func(pas page.Pages) {
			for i, p := range pas {
				np, ok := p.(nextPrevInSectionProvider)
				if !ok {
					continue
				}

				pos := np.getNextPrevInSection()
				if pos == nil {
					continue
				}

				pos.nextPage = nil
				pos.prevPage = nil

				if i > 0 {
					pos.nextPage = pas[i-1]
				}

				if i < len(pas)-1 {
					pos.prevPage = pas[i+1]
				}
			}
		}

		s.pageMap.WalkBranches(func(s string, b *contentBranchNode) bool {
			if b.n.IsView() {
				return false
			}
			if contentTreeNoListAlwaysFilter(s, b.n) {
				return false
			}
			var pas page.Pages
			b.pages.Walk(
				contentTreeNoListAlwaysFilter,
				func(s string, c *contentNode) bool {
					pas = append(pas, c.p)
					return false
				},
			)
			page.SortByDefault(pas)
			setNextPrev(pas)
			return false
		})

		return nil, nil
	})

	s.init.menus = init.Branch(func() (any, error) {
		s.assembleMenus()
		return nil, nil
	})

<<<<<<< HEAD
	s.init.taxonomies = init.Branch(func() (any, error) {
		err := s.pageMap.assembleTaxonomies()
		return nil, err
=======
	s.init.taxonomies = init.Branch(func() (interface{}, error) {
		return nil, s.pageMap.CreateSiteTaxonomies()
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	})
}

type siteRenderingContext struct {
	output.Format
}

func (s *Site) Menus() navigation.Menus {
	s.init.menus.Do()
	return s.menus
}

func (s *Site) initRenderFormats() {
	formatSet := make(map[string]bool)
	formats := output.Formats{}

	s.pageMap.WalkPagesAllPrefixSection("", nil, contentTreeNoRenderFilter, func(np contentNodeProvider) bool {
		n := np.GetNode()
		for _, f := range n.p.m.configuredOutputFormats {
			if !formatSet[f.Name] {
				formats = append(formats, f)
				formatSet[f.Name] = true
			}
		}
		return false
	})

	// Add the per kind configured output formats
	for _, kind := range allKindsInPages {
		if siteFormats, found := s.outputFormats[kind]; found {
			for _, f := range siteFormats {
				if !formatSet[f.Name] {
					formats = append(formats, f)
					formatSet[f.Name] = true
				}
			}
		}
	}

	sort.Sort(formats)
	s.renderFormats = formats
}

func (s *Site) GetRelatedDocsHandler() *page.RelatedDocsHandler {
	return s.relatedDocsHandler
}

func (s *Site) Language() *langs.Language {
	return s.language
}

func (s *Site) isEnabled(kind string) bool {
	return !s.disabledKinds[kind]
}

// reset returns a new Site prepared for rebuild.
func (s *Site) reset() *Site {
	return &Site{
		Deps:                s.Deps,
		disabledKinds:       s.disabledKinds,
		titleFunc:           s.titleFunc,
		relatedDocsHandler:  s.relatedDocsHandler.Clone(),
		siteRefLinker:       s.siteRefLinker,
		outputFormats:       s.outputFormats,
		rc:                  s.rc,
		outputFormatsConfig: s.outputFormatsConfig,
		frontmatterHandler:  s.frontmatterHandler,
		mediaTypesConfig:    s.mediaTypesConfig,
		language:            s.language,
		siteBucket:          s.siteBucket,
		h:                   s.h,
		publisher:           s.publisher,
		siteConfigConfig:    s.siteConfigConfig,
		init:                s.init,
		PageCollections:     s.PageCollections,
		siteCfg:             s.siteCfg,
	}
}

// newSite creates a new site with the given configuration.
func newSite(cfg deps.DepsCfg) (*Site, error) {
	if cfg.Language == nil {
		cfg.Language = langs.NewDefaultLanguage(cfg.Cfg)
	}
	if cfg.Logger == nil {
		panic("logger must be set")
	}

	ignoreErrors := cast.ToStringSlice(cfg.Language.Get("ignoreErrors"))
	ignoreWarnings := cast.ToStringSlice(cfg.Language.Get("ignoreWarnings"))
	ignorableLogger := loggers.NewIgnorableLogger(cfg.Logger, ignoreErrors, ignoreWarnings)

	disabledKinds := make(map[string]bool)
	for _, disabled := range cast.ToStringSlice(cfg.Language.Get("disableKinds")) {
		disabledKinds[disabled] = true
	}

	if disabledKinds["taxonomyTerm"] {
		// Correct from the value it had before Hugo 0.73.0.
		if disabledKinds[pagekinds.Taxonomy] {
			disabledKinds[pagekinds.Term] = true
		} else {
			disabledKinds[pagekinds.Taxonomy] = true
		}

		delete(disabledKinds, "taxonomyTerm")
	} else if disabledKinds[pagekinds.Taxonomy] && !disabledKinds[pagekinds.Term] {
		// This is a potentially ambigous situation. It may be correct.
		ignorableLogger.Errorsf(constants.ErrIDAmbigousDisableKindTaxonomy, `You have the value 'taxonomy' in the disabledKinds list. In Hugo 0.73.0 we fixed these to be what most people expect (taxonomy and term).
But this also means that your site configuration may not do what you expect. If it is correct, you can suppress this message by following the instructions below.`)
	}

	var (
		mediaTypesConfig    []map[string]any
		outputFormatsConfig []map[string]any

		siteOutputFormatsConfig output.Formats
		siteMediaTypesConfig    media.Types
		err                     error
	)

	// Add language last, if set, so it gets precedence.
	for _, cfg := range []config.Provider{cfg.Cfg, cfg.Language} {
		if cfg.IsSet("mediaTypes") {
			mediaTypesConfig = append(mediaTypesConfig, cfg.GetStringMap("mediaTypes"))
		}
		if cfg.IsSet("outputFormats") {
			outputFormatsConfig = append(outputFormatsConfig, cfg.GetStringMap("outputFormats"))
		}
	}

	siteMediaTypesConfig, err = media.DecodeTypes(mediaTypesConfig...)
	if err != nil {
		return nil, err
	}

	siteOutputFormatsConfig, err = output.DecodeFormats(siteMediaTypesConfig, outputFormatsConfig...)
	if err != nil {
		return nil, err
	}

	rssDisabled := disabledKinds["RSS"]
	if rssDisabled {
		// Legacy
		tmp := siteOutputFormatsConfig[:0]
		for _, x := range siteOutputFormatsConfig {
			if !strings.EqualFold(x.Name, "rss") {
				tmp = append(tmp, x)
			}
		}
		siteOutputFormatsConfig = tmp
	}

	var siteOutputs map[string]any
	if cfg.Language.IsSet("outputs") {
		siteOutputs = cfg.Language.GetStringMap("outputs")

		// Check and correct taxonomy kinds vs pre Hugo 0.73.0.
		v1, hasTaxonomyTerm := siteOutputs["taxonomyterm"]
		v2, hasTaxonomy := siteOutputs[pagekinds.Taxonomy]
		_, hasTerm := siteOutputs[pagekinds.Term]
		if hasTaxonomy && hasTaxonomyTerm {
			siteOutputs[pagekinds.Taxonomy] = v1
			siteOutputs[pagekinds.Term] = v2
			delete(siteOutputs, "taxonomyTerm")
		} else if hasTaxonomy && !hasTerm {
			// This is a potentially ambigous situation. It may be correct.
			ignorableLogger.Errorsf(constants.ErrIDAmbigousOutputKindTaxonomy, `You have configured output formats for 'taxonomy' in your site configuration. In Hugo 0.73.0 we fixed these to be what most people expect (taxonomy and term).
But this also means that your site configuration may not do what you expect. If it is correct, you can suppress this message by following the instructions below.`)
		}
		if !hasTaxonomy && hasTaxonomyTerm {
			siteOutputs[pagekinds.Taxonomy] = v1
			delete(siteOutputs, "taxonomyterm")
		}
	}

	outputFormats, err := createSiteOutputFormats(siteOutputFormatsConfig, siteOutputs, rssDisabled)
	if err != nil {
		return nil, err
	}

	taxonomies := cfg.Language.GetStringMapString("taxonomies")

	var relatedContentConfig related.Config

	if cfg.Language.IsSet("related") {
		relatedContentConfig, err = related.DecodeConfig(cfg.Language.GetParams("related"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode related config: %w", err)
		}
	} else {
		relatedContentConfig = related.DefaultConfig
		if _, found := taxonomies["tag"]; found {
			relatedContentConfig.Add(related.IndexConfig{Name: "tags", Weight: 80})
		}
	}

	titleFunc := helpers.GetTitleFunc(cfg.Language.GetString("titleCaseStyle"))

	frontMatterHandler, err := pagemeta.NewFrontmatterHandler(cfg.Logger, cfg.Cfg)
	if err != nil {
		return nil, err
	}

	timeout := 30 * time.Second
	if cfg.Language.IsSet("timeout") {
		v := cfg.Language.Get("timeout")
		d, err := types.ToDurationE(v)
		if err == nil {
			timeout = d
		}
	}

	siteConfig := siteConfigHolder{
		sitemap:          config.DecodeSitemap(config.Sitemap{Priority: -1, Filename: "sitemap.xml"}, cfg.Language.GetStringMap("sitemap")),
		taxonomiesConfig: taxonomies,
		timeout:          timeout,
		hasCJKLanguage:   cfg.Language.GetBool("hasCJKLanguage"),
		enableEmoji:      cfg.Language.Cfg.GetBool("enableEmoji"),
	}

	var siteBucket *pagesMapBucket
	if cfg.Language.IsSet("cascade") {
		var err error
		cascade, err := page.DecodeCascade(cfg.Language.Get("cascade"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode cascade config: %s", err)
		}

		siteBucket = &pagesMapBucket{
			cascade: cascade,
		}

	}

	s := &Site{
		language:      cfg.Language,
		siteBucket:    siteBucket,
		disabledKinds: disabledKinds,

		outputFormats:       outputFormats,
		outputFormatsConfig: siteOutputFormatsConfig,
		mediaTypesConfig:    siteMediaTypesConfig,

		siteCfg: siteConfig,

		titleFunc: titleFunc,

		rc: &siteRenderingContext{output.HTMLFormat},

		frontmatterHandler: frontMatterHandler,
		relatedDocsHandler: page.NewRelatedDocsHandler(relatedContentConfig),
	}

	s.prepareInits()

	return s, nil
}

// NewSiteDefaultLang creates a new site in the default language.
// The site will have a template system loaded and ready to use.
// Note: This is mainly used in single site tests.
// TODO(bep) test refactor -- remove
func NewSiteDefaultLang(withTemplate ...func(templ tpl.TemplateManager) error) (*Site, error) {
	l := configLoader{cfg: config.New()}
	if err := l.applyConfigDefaults(); err != nil {
		return nil, err
	}
	return newSiteForLang(langs.NewDefaultLanguage(l.cfg), withTemplate...)
}

// newSiteForLang creates a new site in the given language.
func newSiteForLang(lang *langs.Language, withTemplate ...func(templ tpl.TemplateManager) error) (*Site, error) {
	withTemplates := func(templ tpl.TemplateManager) error {
		for _, wt := range withTemplate {
			if err := wt(templ); err != nil {
				return err
			}
		}
		return nil
	}

	cfg := deps.DepsCfg{WithTemplate: withTemplates, Cfg: lang}

	return NewSiteForCfg(cfg)
}

// NewSiteForCfg creates a new site for the given configuration.
// The site will have a template system loaded and ready to use.
// Note: This is mainly used in single site tests.
func NewSiteForCfg(cfg deps.DepsCfg) (*Site, error) {
	h, err := NewHugoSites(cfg)
	if err != nil {
		return nil, err
	}
	return h.Sites[0], nil
}

var _ identity.IdentityLookupProvider = (*SiteInfo)(nil)

type SiteInfo struct {
	Authors page.AuthorList
	Social  SiteSocial

	hugoInfo     hugo.Info
	title        string
	RSSLink      string
	Author       map[string]any
	LanguageCode string
	Copyright    string

	permalinks map[string]string

	LanguagePrefix string
	Languages      langs.Languages

	BuildDrafts bool

	canonifyURLs bool
	relativeURLs bool
	uglyURLs     func(p page.Page) bool

	owner                          *HugoSites
	s                              *Site
	language                       *langs.Language
	defaultContentLanguageInSubdir bool
	sectionPagesMenu               string
}

func (s *SiteInfo) LookupIdentity(name string) (identity.Identity, bool) {
	return siteidentities.FromString(name)
}

func (s *SiteInfo) Pages() page.Pages {
	return s.s.Pages()
}

func (s *SiteInfo) RegularPages() page.Pages {
	return s.s.RegularPages()
}

func (s *SiteInfo) AllPages() page.Pages {
	return s.s.AllPages()
}

func (s *SiteInfo) AllRegularPages() page.Pages {
	return s.s.AllRegularPages()
}

func (s *SiteInfo) LastChange() time.Time {
	return s.s.lastmod
}

func (s *SiteInfo) Title() string {
	return s.title
}

func (s *SiteInfo) Site() page.Site {
	return s
}

func (s *SiteInfo) Menus() navigation.Menus {
	return s.s.Menus()
}

// TODO(bep) type
func (s *SiteInfo) Taxonomies() any {
	return s.s.Taxonomies()
}

func (s *SiteInfo) Params() maps.Params {
	return s.s.Language().Params()
}

func (s *SiteInfo) Data() map[string]any {
	return s.s.h.Data()
}

func (s *SiteInfo) Language() *langs.Language {
	return s.language
}

func (s *SiteInfo) Config() SiteConfig {
	return s.s.siteConfigConfig
}

func (s *SiteInfo) Hugo() hugo.Info {
	return s.hugoInfo
}

// Sites is a convenience method to get all the Hugo sites/languages configured.
func (s *SiteInfo) Sites() page.Sites {
	return s.s.h.siteInfos()
}

// Current returns the currently rendered Site.
// If that isn't set yet, which is the situation before we start rendering,
// if will return the Site itself.
func (s *SiteInfo) Current() page.Site {
	if s.s.h.currentSite == nil {
		return s
	}
	return s.s.h.currentSite.Info
}

func (s *SiteInfo) String() string {
	return fmt.Sprintf("Site(%q)", s.title)
}

func (s *SiteInfo) BaseURL() template.URL {
	return template.URL(s.s.PathSpec.BaseURL.String())
}

// ServerPort returns the port part of the BaseURL, 0 if none found.
func (s *SiteInfo) ServerPort() int {
	ps := s.s.PathSpec.BaseURL.URL().Port()
	if ps == "" {
		return 0
	}
	p, err := strconv.Atoi(ps)
	if err != nil {
		return 0
	}
	return p
}

// GoogleAnalytics is kept here for historic reasons.
func (s *SiteInfo) GoogleAnalytics() string {
	return s.Config().Services.GoogleAnalytics.ID
}

// DisqusShortname is kept here for historic reasons.
func (s *SiteInfo) DisqusShortname() string {
	return s.Config().Services.Disqus.Shortname
}

// SiteSocial is a place to put social details on a site level. These are the
// standard keys that themes will expect to have available, but can be
// expanded to any others on a per site basis
// github
// facebook
// facebook_admin
// twitter
// twitter_domain
// pinterest
// instagram
// youtube
// linkedin
type SiteSocial map[string]string

// Param is a convenience method to do lookups in SiteInfo's Params map.
//
// This method is also implemented on Page.
func (s *SiteInfo) Param(key any) (any, error) {
	return resource.Param(s, nil, key)
}

func (s *SiteInfo) IsMultiLingual() bool {
	return len(s.Languages) > 1
}

func (s *SiteInfo) IsServer() bool {
	return s.owner.running
}

type siteRefLinker struct {
	s *Site

	errorLogger *log.Logger
	notFoundURL string
}

func newSiteRefLinker(cfg config.Provider, s *Site) (siteRefLinker, error) {
	logger := s.Log.Error()

	notFoundURL := cfg.GetString("refLinksNotFoundURL")
	errLevel := cfg.GetString("refLinksErrorLevel")
	if strings.EqualFold(errLevel, "warning") {
		logger = s.Log.Warn()
	}
	return siteRefLinker{s: s, errorLogger: logger, notFoundURL: notFoundURL}, nil
}

func (s siteRefLinker) logNotFound(ref, what string, p page.Page, position text.Position) {
	if position.IsValid() {
		s.errorLogger.Printf("[%s] REF_NOT_FOUND: Ref %q: %s: %s", s.s.Lang(), ref, position.String(), what)
	} else if p == nil {
		s.errorLogger.Printf("[%s] REF_NOT_FOUND: Ref %q: %s", s.s.Lang(), ref, what)
	} else {
		s.errorLogger.Printf("[%s] REF_NOT_FOUND: Ref %q from page %q: %s", s.s.Lang(), ref, p.Path(), what)
	}
}

func (s *siteRefLinker) refLink(ref string, source any, relative bool, outputFormat string) (string, error) {
	p, err := unwrapPage(source)
	if err != nil {
		return "", err
	}

	var refURL *url.URL

	ref = filepath.ToSlash(ref)

	refURL, err = url.Parse(ref)

	if err != nil {
		return s.notFoundURL, err
	}

	var target page.Page
	var link string

	if refURL.Path != "" {
		var err error
		target, err = s.s.getPageRef(p, refURL.Path)

		var pos text.Position
		if err != nil || target == nil {
			if p, ok := source.(text.Positioner); ok {
				pos = p.Position()
			}
		}

		if err != nil {
			s.logNotFound(refURL.Path, err.Error(), p, pos)
			return s.notFoundURL, nil
		}

		if target == nil {
			s.logNotFound(refURL.Path, "page not found", p, pos)
			return s.notFoundURL, nil
		}

		var permalinker Permalinker = target

		if outputFormat != "" {
			o := target.OutputFormats().Get(outputFormat)

			if o == nil {
				s.logNotFound(refURL.Path, fmt.Sprintf("output format %q", outputFormat), p, pos)
				return s.notFoundURL, nil
			}
			permalinker = o
		}

		if relative {
			link = permalinker.RelPermalink()
		} else {
			link = permalinker.Permalink()
		}
	}

	if refURL.Fragment != "" {
		_ = target
		link = link + "#" + refURL.Fragment

		if pctx, ok := target.(pageContext); ok {
			if refURL.Path != "" {
				if di, ok := pctx.getContentConverter().(converter.DocumentInfo); ok {
					link = link + di.AnchorSuffix()
				}
			}
		} else if pctx, ok := p.(pageContext); ok {
			if di, ok := pctx.getContentConverter().(converter.DocumentInfo); ok {
				link = link + di.AnchorSuffix()
			}
		}

	}

	return link, nil
}

func (s *Site) running() bool {
	return s.h != nil && s.h.running
}

func (s *Site) multilingual() *Multilingual {
	return s.h.multilingual
}

type whatChanged struct {
	mu sync.Mutex

	contentChanged bool
	identitySet    identity.Identities
}

func (w *whatChanged) Add(ids ...identity.Identity) {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.identitySet == nil {
		return
	}

	for _, id := range ids {
		w.identitySet[id] = true
	}
}

func (w *whatChanged) Changes() []identity.Identity {
	if w == nil || w.identitySet == nil {
		return nil
	}
	return w.identitySet.AsSlice()
}

// RegisterMediaTypes will register the Site's media types in the mime
// package, so it will behave correctly with Hugo's built-in server.
func (s *Site) RegisterMediaTypes() {
	for _, mt := range s.mediaTypesConfig {
		for _, suffix := range mt.Suffixes() {
			_ = mime.AddExtensionType(mt.Delimiter+suffix, mt.Type()+"; charset=utf-8")
		}
	}
}

func (s *Site) filterFileEvents(events []fsnotify.Event) []fsnotify.Event {
	var filtered []fsnotify.Event
	seen := make(map[fsnotify.Event]bool)

	for _, ev := range events {
		// Avoid processing the same event twice.
		if seen[ev] {
			continue
		}
		seen[ev] = true

		if s.SourceSpec.IgnoreFile(ev.Name) {
			continue
		}

		// Throw away any directories
		isRegular, err := s.SourceSpec.IsRegularSourceFile(ev.Name)
		if err != nil && os.IsNotExist(err) && (ev.Op&fsnotify.Remove == fsnotify.Remove || ev.Op&fsnotify.Rename == fsnotify.Rename) {
			// Force keep of event
			isRegular = true
		}
		if !isRegular {
			continue
		}

		if runtime.GOOS == "darwin" { // When a file system is HFS+, its filepath is in NFD form.
			ev.Name = norm.NFC.String(ev.Name)
		}

		filtered = append(filtered, ev)
	}

	return filtered
}

func (s *Site) translateFileEvents(events []fsnotify.Event) []fsnotify.Event {
	var filtered []fsnotify.Event

	eventMap := make(map[string][]fsnotify.Event)

	// We often get a Remove etc. followed by a Create, a Create followed by a Write.
	// Remove the superfluous events to make the update logic simpler.
	for _, ev := range events {
		eventMap[ev.Name] = append(eventMap[ev.Name], ev)
	}

	for _, ev := range events {
		mapped := eventMap[ev.Name]

		// Keep one
		found := false
		var kept fsnotify.Event
		for i, ev2 := range mapped {
			if i == 0 {
				kept = ev2
			}

			if ev2.Op&fsnotify.Write == fsnotify.Write {
				kept = ev2
				found = true
			}

			if !found && ev2.Op&fsnotify.Create == fsnotify.Create {
				kept = ev2
			}
		}

		filtered = append(filtered, kept)
	}

	return filtered
}

var (
	renderHookImageTemplateRe   = regexp.MustCompile(`/_markup/render-image\.`)
	renderHookLinkTemplateRe    = regexp.MustCompile(`/_markup/render-link\.`)
	renderHookHeadingTemplateRe = regexp.MustCompile(`/_markup/render-heading\.`)
)

// processPartial prepares the Sites' sources for a partial rebuild.
// TODO1 .CurrentSection -- no win slashes. Issue?
func (s *Site) processPartial(config *BuildCfg, init func(config *BuildCfg) error, events []fsnotify.Event) error {
	events = s.filterFileEvents(events)
	events = s.translateFileEvents(events)

	h := s.h

	var (
		tmplChanged    bool
		tmplAdded      bool
		i18nChanged    bool
		contentChanged bool

		// prevent spamming the log on changes
		logger = helpers.NewDistinctErrorLogger()
	)

	// Paths relative to their component folder.
	// Changes and addition.
	// pathSetChanges := make(identity.PathIdentitySet)
	// Deletes.
	// pathSetDeletes := make(identity.PathIdentitySet)

	var (
		pathsChanges []*paths.PathInfo
		pathsDeletes []*paths.PathInfo
	)

	for _, ev := range events {
		removed := false

		if ev.Op&fsnotify.Remove == fsnotify.Remove {
			removed = true
		}

		// Some editors (Vim) sometimes issue only a Rename operation when writing an existing file
		// Sometimes a rename operation means that file has been renamed other times it means
		// it's been updated.
		if ev.Op&fsnotify.Rename == fsnotify.Rename {
			// If the file is still on disk, it's only been updated, if it's not, it's been moved
			if ex, err := afero.Exists(s.Fs.Source, ev.Name); !ex || err != nil {
				removed = true
			}
		}

		paths := s.BaseFs.CollectPathIdentities(ev.Name)

		if removed {
			pathsDeletes = append(pathsDeletes, paths...)
		} else {
			pathsChanges = append(pathsChanges, paths...)
		}

	}

	var addedOrChangedContent []*paths.PathInfo

	// Find the most specific identity possible (the most specific being the Go pointer to a given Page).
	// TODO1 bookmark1
	var (
		identities []identity.Identity
	)

	handleChange := func(id *paths.PathInfo, delete bool) {
		switch id.Component() {
		case files.ComponentFolderContent:
			logger.Println("Source changed", id.Filename())
			contentChanged = true
			m := h.getContentMaps()

			var found bool
			for _, pm := range m.pmaps {
				n, tree := pm.GetNodeAndTree(id.Base())
				if n != nil {
					found = true
					identities = append(identities, n.GetIdentity())

					if delete {
						// TODO1 resources
						tree.Delete(id.Base())
					}

					if id.IsBranchBundle() && n.isCascadingEdit() {
						pm.WalkPagesAllPrefixSection(id.Base(), nil, nil, func(node contentNodeProvider) bool {
							n := node.GetNode()
							if n.p != nil {
								n.p.buildState++
							}
							return false
						})
					}

				}
			}

			if delete || !found {
				// TODO1 tie this in with the reset logic, also Add.
				identities = append(identities, siteidentities.PageCollections)

				if !found {
					// A new content file is added. Collect all branch nodes.
					// This will handle all $blog.RegularPages and similar, both direct and indirectly via site.GetPage.
					for _, pm := range m.pmaps {
						pm.WalkBranches(func(s string, n *contentBranchNode) bool {
							identities = append(identities, n.n)
							return false
						})
					}
				}
			}

			if !delete {
				addedOrChangedContent = append(addedOrChangedContent, id)
			}

		case files.ComponentFolderLayouts:
			tmplChanged = true
			if !s.Tmpl().HasTemplate(id.Base()) {
				tmplAdded = true
			}
			identities = append(identities, id)
			if tmplAdded {
				logger.Println("Template added", id.Filename())
				// A new template requires a more coarse grained build.
				if renderHookImageTemplateRe.MatchString(id.Base()) {
					identities = append(identities, converter.FeatureRenderHookImage)
				} else if renderHookLinkTemplateRe.MatchString(id.Base()) {
					identities = append(identities, converter.FeatureRenderHookLink)
				} else if renderHookHeadingTemplateRe.MatchString(id.Base()) {
					identities = append(identities, converter.FeatureRenderHookHeading)
				} else {
					// TODO1
				}
			} else {
				logger.Println("Template changed", id.Filename())
				identities = append(identities, id)
			}
		case files.ComponentFolderAssets:
			r, _ := h.ResourceSpec.ResourceCache.Get(context.Background(), memcache.CleanKey(id.Base()))
			if !identity.WalkIdentities(r, func(rid identity.Identity) bool {
				identities = append(identities, rid)
				return false
			}) {
				identities = append(identities, id)
			}

		case files.ComponentFolderData:
			logger.Println("Data changed", id.Filename())

			// This should cover all usage of site.Data.
			// Currently very coarse grained.
			identities = append(identities, siteidentities.Data)
			s.h.init.data.Reset()
		case files.ComponentFolderI18n:
			logger.Println("i18n changed", id.Filename())
			i18nChanged = true
			identities = append(identities, id)
		default:
			panic(fmt.Sprintf("unknown component: %q", id.Component()))
		}
	}

	for _, id := range pathsDeletes {
		handleChange(id, true)
	}

	for _, id := range pathsChanges {
		handleChange(id, false)
	}

	// TODO1 if config.ErrRecovery || tmplAdded {

	resourceFiles := addedOrChangedContent // TODO1 + remove the PathIdentities .ToPathIdentities().Sort()

	changed := &whatChanged{
		contentChanged: contentChanged,
		identitySet:    make(identity.Identities),
	}
	changed.Add(identities...)

	config.whatChanged = changed

	if err := init(config); err != nil {
		return err
	}

	if tmplChanged || i18nChanged {
		sites := s.h.Sites
		first := sites[0]

		s.h.init.layouts.Reset()
		s.h.init.translations.Reset()

		// TOD(bep) globals clean
		if err := first.Deps.LoadResources(); err != nil {
			return err
		}

		for i := 1; i < len(sites); i++ {
			site := sites[i]
			var err error
			depsCfg := deps.DepsCfg{
				Language:      site.language,
				MediaTypes:    site.mediaTypesConfig,
				OutputFormats: site.outputFormatsConfig,
			}
			site.Deps, err = first.Deps.ForLanguage(depsCfg, func(d *deps.Deps) error {
				d.Site = site.Info
				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	if resourceFiles != nil {
		if err := s.readAndProcessContent(*config, resourceFiles); err != nil {
			return err
		}
	}

	return nil
}

func (s *Site) process(config BuildCfg) (err error) {
	if err = s.initialize(); err != nil {
		err = fmt.Errorf("initialize: %w", err)
		return
	}
<<<<<<< HEAD
	if err = s.readAndProcessContent(config); err != nil {
		err = fmt.Errorf("readAndProcessContent: %w", err)
=======
	if err = s.readAndProcessContent(config, nil); err != nil {
		err = errors.Wrap(err, "readAndProcessContent")
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		return
	}
	return err
}

func (s *Site) render(ctx *siteRenderContext) (err error) {
	if err := page.Clear(); err != nil {
		return err
	}

	if ctx.outIdx == 0 {
		// Note that even if disableAliases is set, the aliases themselves are
		// preserved on page. The motivation with this is to be able to generate
		// 301 redirects in a .htacess file and similar using a custom output format.
		if !s.Cfg.GetBool("disableAliases") {
			// Aliases must be rendered before pages.
			// Some sites, Hugo docs included, have faulty alias definitions that point
			// to itself or another real page. These will be overwritten in the next
			// step.
			if err = s.renderAliases(); err != nil {
				return
			}
		}
	}

	if err = s.renderPages(ctx); err != nil {
		return
	}

	if !ctx.shouldRenderSingletonPages() {
		return
	}

	if err = s.renderMainLanguageRedirect(); err != nil {
		return
	}

	return
}

func (s *Site) Initialise() (err error) {
	return s.initialize()
}

func (s *Site) initialize() (err error) {
	return s.initializeSiteInfo()
}

// HomeAbsURL is a convenience method giving the absolute URL to the home page.
func (s *SiteInfo) HomeAbsURL() string {
	base := ""
	if s.IsMultiLingual() {
		base = s.Language().Lang
	}
	return s.owner.AbsURL(base, false)
}

// SitemapAbsURL is a convenience method giving the absolute URL to the sitemap.
func (s *SiteInfo) SitemapAbsURL() string {
	p := s.HomeAbsURL()
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	p += s.s.siteCfg.sitemap.Filename
	return p
}

func (s *Site) initializeSiteInfo() error {
	var (
		lang      = s.language
		languages langs.Languages
	)

	if s.h != nil && s.h.multilingual != nil {
		languages = s.h.multilingual.Languages
	}

	permalinks := s.Cfg.GetStringMapString("permalinks")

	defaultContentInSubDir := s.Cfg.GetBool("defaultContentLanguageInSubdir")
	defaultContentLanguage := s.Cfg.GetString("defaultContentLanguage")

	languagePrefix := ""
	if s.multilingualEnabled() && (defaultContentInSubDir || lang.Lang != defaultContentLanguage) {
		languagePrefix = "/" + lang.Lang
	}

	uglyURLs := func(p page.Page) bool {
		return false
	}

	v := s.Cfg.Get("uglyURLs")
	if v != nil {
		switch vv := v.(type) {
		case bool:
			uglyURLs = func(p page.Page) bool {
				return vv
			}
		case string:
			// Is what be get from CLI (--uglyURLs)
			vvv := cast.ToBool(vv)
			uglyURLs = func(p page.Page) bool {
				return vvv
			}
		default:
			m := maps.ToStringMapBool(v)
			uglyURLs = func(p page.Page) bool {
				return m[p.Section()]
			}
		}
	}

	// Assemble dependencies to be used in hugo.Deps.
	// TODO(bep) another reminder: We need to clean up this Site vs HugoSites construct.
	var deps []*hugo.Dependency
	var depFromMod func(m modules.Module) *hugo.Dependency
	depFromMod = func(m modules.Module) *hugo.Dependency {
		dep := &hugo.Dependency{
			Path:    m.Path(),
			Version: m.Version(),
			Time:    m.Time(),
			Vendor:  m.Vendor(),
		}

		// These are pointers, but this all came from JSON so there's no recursive navigation,
		// so just create new values.
		if m.Replace() != nil {
			dep.Replace = depFromMod(m.Replace())
		}
		if m.Owner() != nil {
			dep.Owner = depFromMod(m.Owner())
		}
		return dep
	}
	for _, m := range s.Paths.AllModules {
		deps = append(deps, depFromMod(m))
	}

	s.Info = &SiteInfo{
		title:                          lang.GetString("title"),
		Author:                         lang.GetStringMap("author"),
		Social:                         lang.GetStringMapString("social"),
		LanguageCode:                   lang.GetString("languageCode"),
		Copyright:                      lang.GetString("copyright"),
		language:                       lang,
		LanguagePrefix:                 languagePrefix,
		Languages:                      languages,
		defaultContentLanguageInSubdir: defaultContentInSubDir,
		sectionPagesMenu:               lang.GetString("sectionPagesMenu"),
		BuildDrafts:                    s.Cfg.GetBool("buildDrafts"),
		canonifyURLs:                   s.Cfg.GetBool("canonifyURLs"),
		relativeURLs:                   s.Cfg.GetBool("relativeURLs"),
		uglyURLs:                       uglyURLs,
		permalinks:                     permalinks,
		owner:                          s.h,
		s:                              s,
		hugoInfo:                       hugo.NewInfo(s.Cfg.GetString("environment"), deps),
	}

	rssOutputFormat, found := s.outputFormats[pagekinds.Home].GetByName(output.RSSFormat.Name)

	if found {
		s.Info.RSSLink = s.permalink(rssOutputFormat.BaseFilename())
	}

	return nil
}

func (s *Site) readAndProcessContent(buildConfig BuildCfg, ids paths.PathInfos) error {
	sourceSpec := source.NewSourceSpec(s.PathSpec, buildConfig.ContentInclusionFilter, s.BaseFs.Content.Fs)

	proc := newPagesProcessor(s.h, sourceSpec)

	c := newPagesCollector(sourceSpec, s.h.getContentMaps(), s.Log, s.h.ContentChanges, proc, ids)

	if err := c.Collect(); err != nil {
		return err
	}

	return nil
}

func (s *Site) getMenusFromConfig() navigation.Menus {
	ret := navigation.Menus{}

	if menus := s.language.GetStringMap("menus"); menus != nil {
		for name, menu := range menus {
			m, err := cast.ToSliceE(menu)
			if err != nil {
				s.Log.Errorf("menus in site config contain errors\n")
				s.Log.Errorln(err)
			} else {
				handleErr := func(err error) {
					if err == nil {
						return
					}
					s.Log.Errorf("menus in site config contain errors\n")
					s.Log.Errorln(err)
				}

				for _, entry := range m {
					s.Log.Debugf("found menu: %q, in site config\n", name)

					menuEntry := navigation.MenuEntry{Menu: name}
					ime, err := maps.ToStringMapE(entry)
					handleErr(err)

					err = menuEntry.MarshallMap(ime)
					handleErr(err)

					// TODO(bep) clean up all of this
					menuEntry.ConfiguredURL = s.Info.createNodeMenuEntryURL(menuEntry.ConfiguredURL)

					if ret[name] == nil {
						ret[name] = navigation.Menu{}
					}
					ret[name] = ret[name].Add(&menuEntry)
				}
			}
		}
		return ret
	}
	return ret
}

func (s *SiteInfo) createNodeMenuEntryURL(in string) string {
	if !strings.HasPrefix(in, "/") {
		return in
	}
	// make it match the nodes
	menuEntryURL := in
	menuEntryURL = paths.URLEscape(s.s.PathSpec.URLize(menuEntryURL))
	if !s.canonifyURLs {
		menuEntryURL = paths.AddContextRoot(s.s.PathSpec.BaseURL.String(), menuEntryURL)
	}
	return menuEntryURL
}

func (s *Site) assembleMenus() {
	s.menus = make(navigation.Menus)

	type twoD struct {
		MenuName, EntryName string
	}
	flat := map[twoD]*navigation.MenuEntry{}
	children := map[twoD]navigation.Menu{}

	// add menu entries from config to flat hash
	menuConfig := s.getMenusFromConfig()
	for name, menu := range menuConfig {
		for _, me := range menu {
			if types.IsNil(me.Page) && me.PageRef != "" {
				// Try to resolve the page.
				me.Page, _ = s.getPageNew(nil, me.PageRef)
			}
			flat[twoD{name, me.KeyName()}] = me
		}
	}

	sectionPagesMenu := s.Info.sectionPagesMenu

	if sectionPagesMenu != "" {
<<<<<<< HEAD
		s.pageMap.sections.Walk(func(s string, v any) bool {
			p := v.(*contentNode).p
			if p.IsHome() {
=======
		s.pageMap.WalkPagesAllPrefixSection("", noTaxonomiesFilter, contentTreeNoListAlwaysFilter, func(np contentNodeProvider) bool {
			s := np.Key()
			n := np.GetNode()

			if s == "" {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
				return false
			}

			p := n.p

			// From Hugo 0.22 we have nested sections, but until we get a
			// feel of how that would work in this setting, let us keep
			// this menu for the top level only.
			id := p.Section()
			if _, ok := flat[twoD{sectionPagesMenu, id}]; ok {
				return false
			}

			me := navigation.MenuEntry{
				Identifier: id,
				Name:       p.LinkTitle(),
				Weight:     p.Weight(),
				Page:       p,
			}
			flat[twoD{sectionPagesMenu, me.KeyName()}] = &me

			return false
		})
	}

	// Add menu entries provided by pages
	s.pageMap.WalkPagesAllPrefixSection("", noTaxonomiesFilter, contentTreeNoRenderFilter, func(np contentNodeProvider) bool {
		n := np.GetNode()
		p := n.p

		for name, me := range p.pageMenus.menus() {
			if _, ok := flat[twoD{name, me.KeyName()}]; ok {
				err := p.wrapError(fmt.Errorf("duplicate menu entry with identifier %q in menu %q", me.KeyName(), name))
				s.Log.Warnln(err)
				continue
			}
			flat[twoD{name, me.KeyName()}] = me
		}

		return false
	})

	// Create Children Menus First
	for _, e := range flat {
		if e.Parent != "" {
			children[twoD{e.Menu, e.Parent}] = children[twoD{e.Menu, e.Parent}].Add(e)
		}
	}

	// Placing Children in Parents (in flat)
	for p, childmenu := range children {
		_, ok := flat[twoD{p.MenuName, p.EntryName}]
		if !ok {
			// if parent does not exist, create one without a URL
			flat[twoD{p.MenuName, p.EntryName}] = &navigation.MenuEntry{Name: p.EntryName}
		}
		flat[twoD{p.MenuName, p.EntryName}].Children = childmenu
	}

	// Assembling Top Level of Tree
	for menu, e := range flat {
		if e.Parent == "" {
			_, ok := s.menus[menu.MenuName]
			if !ok {
				s.menus[menu.MenuName] = navigation.Menu{}
			}
			s.menus[menu.MenuName] = s.menus[menu.MenuName].Add(e)
		}
	}
}

// get any language code to prefix the target file path with.
func (s *Site) getLanguageTargetPathLang(alwaysInSubDir bool) string {
	if s.h.IsMultihost() {
		return s.Language().Lang
	}

	return s.getLanguagePermalinkLang(alwaysInSubDir)
}

// get any lanaguagecode to prefix the relative permalink with.
func (s *Site) getLanguagePermalinkLang(alwaysInSubDir bool) string {
	if !s.Info.IsMultiLingual() || s.h.IsMultihost() {
		return ""
	}

	if alwaysInSubDir {
		return s.Language().Lang
	}

	isDefault := s.Language().Lang == s.multilingual().DefaultLang.Lang

	if !isDefault || s.Info.defaultContentLanguageInSubdir {
		return s.Language().Lang
	}

	return ""
}

func (s *Site) getTaxonomyKey(key string) string {
	if s.PathSpec.DisablePathToLower {
		return s.PathSpec.MakePath(key)
	}
	return strings.ToLower(s.PathSpec.MakePath(key))
}

// Prepare site for a new full build.
func (s *Site) resetBuildState(sourceChanged bool) {
	s.relatedDocsHandler = s.relatedDocsHandler.Clone()
	s.init.Reset()

	if sourceChanged {
		s.pageMap.pageReverseIndex.Reset()
		s.PageCollections = newPageCollections(s.pageMap)
		s.pageMap.WithEveryBundlePage(func(p *pageState) bool {
			if p.bucket != nil {
				p.bucket.pagesMapBucketPages = &pagesMapBucketPages{}
			}
			p.Scratcher = maps.NewScratcher()
			return false
		})

	} else {
		s.pageMap.WithEveryBundlePage(func(p *pageState) bool {
			p.Scratcher = maps.NewScratcher()
			return false
		})
	}
}

func (s *Site) errorCollator(results <-chan error, errs chan<- error) {
	var errors []error
	for e := range results {
		errors = append(errors, e)
	}

	errs <- s.h.pickOneAndLogTheRest(errors)

	close(errs)
}

// GetPage looks up a page of a given type for the given ref.
// In Hugo <= 0.44 you had to add Page Kind (section, home) etc. as the first
// argument and then either a unix styled path (with or without a leading slash))
// or path elements separated.
// When we now remove the Kind from this API, we need to make the transition as painless
// as possible for existing sites. Most sites will use {{ .Site.GetPage "section" "my/section" }},
// i.e. 2 arguments, so we test for that.
func (s *SiteInfo) GetPage(ref ...string) (page.Page, error) {
	p, err := s.s.getPageOldVersion(ref...)

	if p == nil {
		// The nil struct has meaning in some situations, mostly to avoid breaking
		// existing sites doing $nilpage.IsDescendant($p), which will always return
		// false.
		p = page.NilPage
	}

	return p, err
}

func (s *Site) permalink(link string) string {
	return s.PathSpec.PermalinkForBaseURL(link, s.PathSpec.BaseURL.String())
}

func (s *Site) absURLPath(targetPath string) string {
	var path string
	if s.Info.relativeURLs {
		path = helpers.GetDottedRelativePath(targetPath)
	} else {
		url := s.PathSpec.BaseURL.String()
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		path = url
	}

	return path
}

func (s *Site) lookupLayouts(layouts ...string) tpl.Template {
	for _, l := range layouts {
		if templ, found := s.Tmpl().Lookup(l); found {
			return templ
		}
	}

	return nil
}

func (s *Site) renderAndWriteXML(statCounter *uint64, name string, targetPath string, d any, templ tpl.Template) error {
	s.Log.Debugf("Render XML for %q to %q", name, targetPath)
	renderBuffer := bp.GetBuffer()
	defer bp.PutBuffer(renderBuffer)

	if err := s.renderForTemplate(name, "", d, renderBuffer, templ); err != nil {
		return err
	}

	pd := publisher.Descriptor{
		Src:         renderBuffer,
		TargetPath:  targetPath,
		StatCounter: statCounter,
		// For the minification part of XML,
		// we currently only use the MIME type.
		OutputFormat: output.RSSFormat,
		AbsURLPath:   s.absURLPath(targetPath),
	}

	return s.publisher.Publish(pd)
}

func (s *Site) renderAndWritePage(statCounter *uint64, name string, targetPath string, p *pageState, templ tpl.Template) error {
	s.Log.Debugf("Render %s to %q", name, targetPath)
	s.h.IncrPageRender()
	renderBuffer := bp.GetBuffer()
	defer bp.PutBuffer(renderBuffer)

	of := p.outputFormat()
	p.pageOutput.renderState++

	if err := s.renderForTemplate(p.Kind(), of.Name, p, renderBuffer, templ); err != nil {
		return err
	}

	if renderBuffer.Len() == 0 {
		return nil
	}

	isHTML := of.IsHTML
	isRSS := of.Name == "RSS"

	pd := publisher.Descriptor{
		Src:          renderBuffer,
		TargetPath:   targetPath,
		StatCounter:  statCounter,
		OutputFormat: p.outputFormat(),
	}

	if isRSS {
		// Always canonify URLs in RSS
		pd.AbsURLPath = s.absURLPath(targetPath)
	} else if isHTML {
		if s.Info.relativeURLs || s.Info.canonifyURLs {
			pd.AbsURLPath = s.absURLPath(targetPath)
		}

		if s.running() && s.Cfg.GetBool("watch") && !s.Cfg.GetBool("disableLiveReload") {
			pd.LiveReloadBaseURL = s.PathSpec.BaseURL.URL()
			if s.Cfg.GetInt("liveReloadPort") != -1 {
				pd.LiveReloadBaseURL.Host = fmt.Sprintf("%s:%d", pd.LiveReloadBaseURL.Hostname(), s.Cfg.GetInt("liveReloadPort"))
			}
		}

		// For performance reasons we only inject the Hugo generator tag on the home page.
		if p.IsHome() {
			pd.AddHugoGeneratorTag = !s.Cfg.GetBool("disableHugoGeneratorInject")
		}

	}

	return s.publisher.Publish(pd)
}

var infoOnMissingLayout = map[string]bool{
	// The 404 layout is very much optional in Hugo, but we do look for it.
	"404": true,
}

// hookRendererTemplate is the canonical implementation of all hooks.ITEMRenderer,
// where ITEM is the thing being hooked.
type hookRendererTemplate struct {
	templateHandler tpl.TemplateHandler
<<<<<<< HEAD
	identity.SearchProvider
	templ           tpl.Template
	resolvePosition func(ctx any) text.Position
=======
	identity.Identity
	templ tpl.Template
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func (hr hookRendererTemplate) RenderLink(w io.Writer, ctx hooks.LinkContext) error {
	return hr.templateHandler.Execute(hr.templ, w, ctx)
}

func (hr hookRendererTemplate) RenderHeading(w io.Writer, ctx hooks.HeadingContext) error {
	return hr.templateHandler.Execute(hr.templ, w, ctx)
}

<<<<<<< HEAD
func (hr hookRendererTemplate) RenderCodeblock(w hugio.FlexiWriter, ctx hooks.CodeblockContext) error {
	return hr.templateHandler.Execute(hr.templ, w, ctx)
}

func (hr hookRendererTemplate) ResolvePosition(ctx any) text.Position {
	return hr.resolvePosition(ctx)
}

func (hr hookRendererTemplate) IsDefaultCodeBlockRenderer() bool {
	return false
}

func (s *Site) renderForTemplate(name, outputFormat string, d any, w io.Writer, templ tpl.Template) (err error) {
=======
func (hr hookRenderer) Template() identity.Identity {
	return hr.templ.(tpl.Info)
}

func (s *Site) renderForTemplate(name, outputFormat string, d interface{}, w io.Writer, templ tpl.Template) (err error) {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	if templ == nil {
		s.logMissingLayout(name, "", "", outputFormat)
		return nil
	}

	if err = s.Tmpl().Execute(templ, w, d); err != nil {
<<<<<<< HEAD
		return fmt.Errorf("render of %q failed: %w", name, err)
=======
		return errors.Wrapf(err, "render of %q failed", name)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	}
	return
}

func (s *Site) lookupTemplate(layouts ...string) (tpl.Template, bool) {
	for _, l := range layouts {
		if templ, found := s.Tmpl().Lookup(l); found {
			return templ, true
		}
	}

	return nil, false
}

func (s *Site) publish(statCounter *uint64, path string, r io.Reader, fs afero.Fs) (err error) {
	s.PathSpec.ProcessingStats.Incr(statCounter)

	return helpers.WriteToDisk(filepath.Clean(path), r, fs)
}

func (s *Site) newPageFromTreeRef(np contentTreeRefProvider, pc pageContent) (*pageState, error) {
	n := np.GetNode()

	var f *source.File

	fi := n.FileInfo()
	if fi != nil {
		var err error
		f, err = source.NewFileInfo(fi)
		if err != nil {
			return nil, err
		}
	}

	container := np.GetContainerNode()
	branch := np.GetBranch()
	bundled := container != nil && container.p.IsPage()

	var kindProvider contentKindProvider
	if kp, ok := n.traits.(contentKindProvider); ok {
		kindProvider = kp
	} else {
		var kind string
		if np.Key() == "" {
			kind = pagekinds.Home
		} else if container != nil && container.IsView() {
			panic("2 TODO1 remove me")
		} else if n.IsView() {
			panic(fmt.Sprintf("2 TODO1 remove me: %T", n.traits))
		} else if branch.n == n {
			kind = pagekinds.Section
		} else {
			kind = pagekinds.Page
		}

		kindProvider = stringKindProvider(kind)

	}

	metaProvider := &pageMeta{
		contentKindProvider:     kindProvider,
		treeRef:                 np,
		contentNodeInfoProvider: np,
		bundled:                 bundled,
		s:                       s,
		f:                       f,
	}

<<<<<<< HEAD
	return s.kindFromSectionPath(path.Join(sections...))
}

func (s *Site) kindFromSectionPath(sectionPath string) string {
	for _, plural := range s.siteCfg.taxonomiesConfig {
		if plural == sectionPath {
			return page.KindTaxonomy
		}

		if strings.HasPrefix(sectionPath, plural) {
			return page.KindTerm
		}

	}

	return page.KindSection
}

func (s *Site) newPage(
	n *contentNode,
	parentbBucket *pagesMapBucket,
	kind, title string,
	sections ...string) *pageState {
	m := map[string]any{}
	if title != "" {
		m["title"] = title
	}

	p, err := newPageFromMeta(
		n,
		parentbBucket,
		m,
		&pageMeta{
			s:        s,
			kind:     kind,
			sections: sections,
		})
=======
	ps, err := newPageBase(metaProvider)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	if err != nil {
		return nil, err
	}
	n.p = ps

	if fi != nil && fi.Meta().IsRootFile {
		// Make sure that the bundle/section we start walking from is always
		// rendered.
		// This is only relevant in server fast render mode.
		// TODO1 ps.forceRender = true
	}

	var parentBucket *pagesMapBucket
	if kindProvider.Kind() == pagekinds.Home {
		parentBucket = ps.s.siteBucket
	} else if bundled {
		parentBucket = branch.n.p.bucket
	} else if container != nil {
		parentBucket = container.p.bucket
	}

	if ps.IsNode() {
		ps.bucket = newPageBucket(parentBucket, ps)
	}

	if fi == nil {
		var meta map[string]interface{}
		if kindProvider.Kind() == pagekinds.Term {
			meta = map[string]interface{}{
				"title": n.traits.(viewInfoTrait).ViewInfo().Term(),
			}
		}
		if err := metaProvider.setMetadata(parentBucket, n, meta); err != nil {
			return nil, ps.wrapError(err)
		}
	} else {
		gi, err := s.h.gitInfoForPage(ps)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Git data")
		}
		ps.gitInfo = gi

		if pc.source.posMainContent > 0 { // TODO1
			ps.pageContent = pc
		} else {
			ps.pageContent = pageContent{
				source: rawPageContent{
					parsed:         n.pageContent,
					posMainContent: -1,
					posSummaryEnd:  -1,
					posBodyStart:   -1,
				},
			}
		}

		ps.shortcodeState = newShortcodeHandler(ps, ps.s, nil)

		// TODO1
		meta, err := ps.mapContent(metaProvider)
		if err != nil {
			return nil, ps.wrapError(err)
		}

		if err := metaProvider.setMetadata(parentBucket, n, meta); err != nil {
			return nil, ps.wrapError(err)
		}

	}

	if err := metaProvider.applyDefaultValues(np); err != nil {
		return nil, err
	}

	ps.init.Add(func() (interface{}, error) {
		pp, err := newPagePaths(s, n, metaProvider)
		if err != nil {
			return nil, err
		}

		var outputFormatsForPage output.Formats
		var renderFormats output.Formats

		if !n.IsStandalone() {
			outputFormatsForPage = ps.m.outputFormats()
			renderFormats = ps.s.h.renderFormats
		} else {
			// One of the fixed output format pages, e.g. 404.
			outputFormatsForPage = output.Formats{n.traits.(kindOutputFormatTrait).OutputFormat()}
			renderFormats = outputFormatsForPage
		}

		// Prepare output formats for all sites.
		// We do this even if this page does not get rendered on
		// its own. It may be referenced via .Site.GetPage and
		// it will then need an output format.
		ps.pageOutputs = make([]*pageOutput, len(renderFormats))
		created := make(map[string]*pageOutput)
		shouldRenderPage := !ps.m.noRender()

		for i, f := range renderFormats {
			if po, found := created[f.Name]; found {
				ps.pageOutputs[i] = po
				continue
			}

			render := shouldRenderPage
			if render {
				_, render = outputFormatsForPage.GetByName(f.Name)
			}

			po := newPageOutput(ps, pp, f, render)

			// Create a content provider for the first,
			// we may be able to reuse it.
			if i == 0 {
				contentProvider, err := newPageContentOutput(po)
				if err != nil {
					return nil, err
				}
				po.initContentProvider(contentProvider)
			}

			ps.pageOutputs[i] = po
			created[f.Name] = po

		}

		if err := ps.initCommonProviders(pp); err != nil {
			return nil, err
		}

		return nil, nil
	})

	return ps, nil
}

func (s *Site) shouldBuild(p *pageState) bool {
	dates := p.pageCommon.m.getTemporaryDates()
	return shouldBuild(s.BuildFuture, s.BuildExpired,
		s.BuildDrafts, p.Draft(), dates.PublishDate(), dates.ExpiryDate())
}

func shouldBuild(buildFuture bool, buildExpired bool, buildDrafts bool, Draft bool,
	publishDate time.Time, expiryDate time.Time) bool {
	if !(buildDrafts || !Draft) {
		return false
	}
	hnow := htime.Now()
	if !buildFuture && !publishDate.IsZero() && publishDate.After(hnow) {
		return false
	}
	if !buildExpired && !expiryDate.IsZero() && expiryDate.Before(hnow) {
		return false
	}
	return true
}
