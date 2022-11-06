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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gobuffalo/flect"
	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/langs"

	"github.com/gohugoio/hugo/markup/converter"

	"github.com/gohugoio/hugo/hugofs/files"

	"github.com/gohugoio/hugo/common/hugo"
	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/related"

	"github.com/gohugoio/hugo/source"

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/helpers"

	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/page/pagemeta"
	"github.com/gohugoio/hugo/resources/resource"
	"github.com/spf13/cast"
)

var cjkRe = regexp.MustCompile(`\p{Han}|\p{Hangul}|\p{Hiragana}|\p{Katakana}`)

var (
	_ resource.Dated  = (*pageMeta)(nil)
	_ resource.Staler = (*pageMeta)(nil)
)

type pageMeta struct {
	// kind is the discriminator that identifies the different page types
	// in the different page collections. This can, as an example, be used
	// to to filter regular pages, find sections etc.
	// Kind will, for the pages available to the templates, be one of:
	// page, home, section, taxonomy and term.
	// It is of string type to make it easy to reason about in
	// the templates.
	kind string

	resource.Staler

	// Set for standalone pages, e.g. robotsTXT.
	standaloneOutputFormat output.Format

	draft       bool // Only published when running with -D flag
	buildConfig pagemeta.BuildConfig
	bundleType  files.ContentClass

	// params contains configuration defined in the params section of page frontmatter.
	params maps.Params
	// cascade contains default configuration to be cascaded downwards.
	cascade map[page.PageMatcher]maps.Params

	title     string
	linkTitle string

	summary string

	resourcePath string

	weight int

	markup      string
	contentType string

	// whether the content is in a CJK language.
	isCJKLanguage bool

	layout string

	aliases []string

	description string
	keywords    []string

	urlPaths pagemeta.URLPath

	// The 4 front matter dates that Hugo cares about.
	// Note that we have a mapping setup that maps from other keys.
	pageMetaDates

	// Set if this page is bundled inside another.
	bundled bool

	// A key that maps to translation(s) of this page. This value is fetched
	// from the page front matter.
	translationKey string

	// From front matter.
	configuredOutputFormats output.Formats

	// This is the raw front matter metadata that is going to be assigned to
	// the Resources above.
	resourcesMetadata []map[string]any

	// Always set. Thi is the cannonical path to the Page.
	pathInfo *paths.Path

	// Set if backed by a file.
	f *source.File

	// Sitemap overrides from front matter.
	sitemap config.Sitemap

	// Convenience shortcuts to the Site.
	s *Site

	contentConverterInit sync.Once
	contentConverter     converter.Converter
}

type pageMetaDates struct {
	dates resource.Dates
}

func (d *pageMetaDates) Date() time.Time {
	return d.dates.Date()
}

func (d *pageMetaDates) Lastmod() time.Time {
	return d.dates.Lastmod()
}

func (d *pageMetaDates) PublishDate() time.Time {
	return d.dates.PublishDate()
}

func (d *pageMetaDates) ExpiryDate() time.Time {
	return d.dates.ExpiryDate()
}

func (p *pageMeta) Aliases() []string {
	return p.aliases
}

func (p *pageMeta) Author() page.Author {
	helpers.Deprecated(".Author", "Use taxonomies.", false)
	authors := p.Authors()

	for _, author := range authors {
		return author
	}
	return page.Author{}
}

func (p *pageMeta) Authors() page.AuthorList {
	helpers.Deprecated(".Authors", "Use taxonomies.", false)
	authorKeys, ok := p.params["authors"]
	if !ok {
		return page.AuthorList{}
	}
	authors := authorKeys.([]string)
	if len(authors) < 1 || len(p.s.Info.Authors) < 1 {
		return page.AuthorList{}
	}

	al := make(page.AuthorList)
	for _, author := range authors {
		a, ok := p.s.Info.Authors[author]
		if ok {
			al[author] = a
		}
	}
	return al
}

func (p *pageMeta) BundleType() files.ContentClass {
	return p.bundleType
}

func (p *pageMeta) Description() string {
	return p.description
}

func (p *pageMeta) Lang() string {
	return p.s.Lang()
}

func (p *pageMeta) Draft() bool {
	return p.draft
}

func (p *pageMeta) File() *source.File {
	return p.f
}

func (p *pageMeta) IsHome() bool {
	return p.Kind() == pagekinds.Home
}

func (p *pageMeta) Kind() string {
	return p.kind
}

func (p *pageMeta) Keywords() []string {
	return p.keywords
}

func (p *pageMeta) Layout() string {
	return p.layout
}

func (p *pageMeta) LinkTitle() string {
	if p.linkTitle != "" {
		return p.linkTitle
	}

	return p.Title()
}

func (p *pageMeta) Name() string {
	if p.resourcePath != "" {
		return p.resourcePath
	}
	return p.Title()
}

func (p *pageMeta) IsNode() bool {
	return !(p.IsPage() || p.isStandalone())
}

func (p *pageMeta) IsPage() bool {
	return p.Kind() == pagekinds.Page
}

// Param is a convenience method to do lookups in Page's and Site's Params map,
// in that order.
//
// This method is also implemented on SiteInfo.
// TODO(bep) interface
func (p *pageMeta) Param(key any) (any, error) {
	return resource.Param(p, p.s.Info.Params(), key)
}

func (p *pageMeta) Params() maps.Params {
	return p.params
}

func (p *pageMeta) Path() string {
	return p.pathInfo.Base()
}

// RelatedKeywords implements the related.Document interface needed for fast page searches.
func (p *pageMeta) RelatedKeywords(cfg related.IndexConfig) ([]related.Keyword, error) {
	v, err := p.Param(cfg.Name)
	if err != nil {
		return nil, err
	}

	return cfg.ToKeywords(v)
}

func (p *pageMeta) IsSection() bool {
	return p.Kind() == pagekinds.Section
}

func (p *pageMeta) Section() string {
	// TODO1 make sure pathInfo is always set.
	return p.pathInfo.Section()
}

func (p *pageMeta) Sitemap() config.Sitemap {
	return p.sitemap
}

func (p *pageMeta) Title() string {
	return p.title
}

const defaultContentType = "page"

func (p *pageMeta) Type() string {
	if p.contentType != "" {
		return p.contentType
	}

	if sect := p.Section(); sect != "" {
		return sect
	}

	return defaultContentType
}

func (p *pageMeta) Weight() int {
	return p.weight
}

func (ps *pageState) initLazyProviders() error {
	ps.init.Add(func() (any, error) {
		pp, err := newPagePaths(ps)
		if err != nil {
			return nil, err
		}

		var outputFormatsForPage output.Formats
		var renderFormats output.Formats

		if ps.m.standaloneOutputFormat.IsZero() {
			outputFormatsForPage = ps.m.outputFormats()
			renderFormats = ps.s.h.renderFormats
		} else {
			// One of the fixed output format pages, e.g. 404.
			outputFormatsForPage = output.Formats{ps.m.standaloneOutputFormat}
			renderFormats = outputFormatsForPage
		}

		// Prepare output formats for all sites.
		// We do this even if this page does not get rendered on
		// its own. It may be referenced via one of the site collections etc.
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
				po.setContentProvider(contentProvider)
			}

			ps.pageOutputs[i] = po
			created[f.Name] = po

		}

		if err := ps.initCommonProviders(pp); err != nil {
			return nil, err
		}

		return nil, nil
	})

	return nil
}

func (ps *pageState) setMetadatPost(cascade map[page.PageMatcher]maps.Params) error {
	// Apply cascades first so they can be overriden later.
	if cascade != nil {
		if ps.m.cascade != nil {
			for k, v := range cascade {
				vv, found := ps.m.cascade[k]
				if !found {
					ps.m.cascade[k] = v
				} else {
					// Merge
					for ck, cv := range v {
						if _, found := vv[ck]; !found {
							vv[ck] = cv
						}
					}
				}
			}
			cascade = ps.m.cascade
		}
	}

	if cascade == nil {
		cascade = ps.m.cascade
	}

	// Cascade is also applied to itself.
	if cascade != nil {
		for m, v := range cascade {
			if !m.Matches(ps) {
				continue
			}
			for kk, vv := range v {
				if _, found := ps.m.params[kk]; !found {
					ps.m.params[kk] = vv
				}
			}
		}

	}

	if err := ps.setMetaDataPostParams(); err != nil {
		return err
	}

	if err := ps.m.applyDefaultValues(); err != nil {
		return err
	}

	return nil
}

func (p *pageState) setMetaDataPostParams() error {
	pm := p.m
	var mtime time.Time
	var contentBaseName string
	if p.File() != nil {
		contentBaseName = p.File().ContentBaseName()
		if p.File().FileInfo() != nil {
			mtime = p.File().FileInfo().ModTime()
		}
	}

	var gitAuthorDate time.Time
	if p.gitInfo != nil {
		gitAuthorDate = p.gitInfo.AuthorDate
	}

	descriptor := &pagemeta.FrontMatterDescriptor{
		Frontmatter:   pm.params, // TODO1 remove me.
		Params:        pm.params,
		Dates:         &pm.pageMetaDates.dates,
		PageURLs:      &pm.urlPaths,
		BaseFilename:  contentBaseName,
		ModTime:       mtime,
		GitAuthorDate: gitAuthorDate,
		Location:      langs.GetLocation(pm.s.Language()),
	}

	// Handle the date separately
	// TODO(bep) we need to "do more" in this area so this can be split up and
	// more easily tested without the Page, but the coupling is strong.
	err := pm.s.frontmatterHandler.HandleDates(descriptor)
	if err != nil {
		p.s.Log.Errorf("Failed to handle dates for page %q: %s", p.pathOrTitle(), err)
	}

	pm.buildConfig, err = pagemeta.DecodeBuildConfig(pm.params["_build"])
	if err != nil {
		return err
	}

	// TODO1
	isStandalone := false
	if isStandalone {
		// Standalone pages, e.g. 404.
		pm.buildConfig.List = pagemeta.Never
	}

	var sitemapSet bool

	var draft, published, isCJKLanguage *bool
	for k, v := range pm.params {
		loki := strings.ToLower(k)

		if loki == "published" { // Intentionally undocumented
			vv, err := cast.ToBoolE(v)
			if err == nil {
				published = &vv
			}
			// published may also be a date
			continue
		}

		if pm.s.frontmatterHandler.IsDateKey(loki) {
			continue
		}

		switch loki {
		case "title":
			pm.title = cast.ToString(v)
			pm.params[loki] = pm.title
		case "linktitle":
			pm.linkTitle = cast.ToString(v)
			pm.params[loki] = pm.linkTitle
		case "summary":
			pm.summary = cast.ToString(v)
			pm.params[loki] = pm.summary
		case "description":
			pm.description = cast.ToString(v)
			pm.params[loki] = pm.description
		case "slug":
			// Don't start or end with a -
			pm.urlPaths.Slug = strings.Trim(cast.ToString(v), "-")
			pm.params[loki] = pm.Slug()
		case "url":
			url := cast.ToString(v)
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
				return fmt.Errorf("URLs with protocol (http*) not supported: %q. In page %q", url, p.pathOrTitle())
			}
			lang := p.s.GetLanguagePrefix()
			if lang != "" && !strings.HasPrefix(url, "/") && strings.HasPrefix(url, lang+"/") {
				if strings.HasPrefix(hugo.CurrentVersion.String(), "0.55") {
					// We added support for page relative URLs in Hugo 0.55 and
					// this may get its language path added twice.
					// TODO(bep) eventually remove this.
					p.s.Log.Warnf(`Front matter in %q with the url %q with no leading / has what looks like the language prefix added. In Hugo 0.55 we added support for page relative URLs in front matter, no language prefix needed. Check the URL and consider to either add a leading / or remove the language prefix.`, p.pathOrTitle(), url)
				}
			}
			pm.urlPaths.URL = url
			pm.params[loki] = url
		case "type":
			pm.contentType = cast.ToString(v)
			pm.params[loki] = pm.contentType
		case "keywords":
			pm.keywords = cast.ToStringSlice(v)
			pm.params[loki] = pm.keywords
		case "headless":
			// Legacy setting for leaf bundles.
			// This is since Hugo 0.63 handled in a more general way for all
			// pages.
			isHeadless := cast.ToBool(v)
			pm.params[loki] = isHeadless
			// TODO1 when File is nil.
			if p.File().TranslationBaseName() == "index" && isHeadless {
				pm.buildConfig.List = pagemeta.Never
				pm.buildConfig.Render = pagemeta.Never
			}
		case "outputs":
			o := cast.ToStringSlice(v)
			if len(o) > 0 {
				// Output formats are explicitly set in front matter, use those.
				outFormats, err := p.s.outputFormatsConfig.GetByNames(o...)

				if err != nil {
					p.s.Log.Errorf("Failed to resolve output formats: %s", err)
				} else {
					pm.configuredOutputFormats = outFormats
					pm.params[loki] = outFormats
				}

			}
		case "draft":
			draft = new(bool)
			*draft = cast.ToBool(v)
		case "layout":
			pm.layout = cast.ToString(v)
			pm.params[loki] = pm.layout
		case "markup":
			pm.markup = cast.ToString(v)
			pm.params[loki] = pm.markup
		case "weight":
			pm.weight = cast.ToInt(v)
			pm.params[loki] = pm.weight
		case "aliases":
			pm.aliases = cast.ToStringSlice(v)
			for i, alias := range pm.aliases {
				if strings.HasPrefix(alias, "http://") || strings.HasPrefix(alias, "https://") {
					return fmt.Errorf("http* aliases not supported: %q", alias)
				}
				pm.aliases[i] = filepath.ToSlash(alias)
			}
			pm.params[loki] = pm.aliases
		case "sitemap":
			p.m.sitemap = config.DecodeSitemap(p.s.siteCfg.sitemap, maps.ToStringMap(v))
			pm.params[loki] = p.m.sitemap
			sitemapSet = true
		case "iscjklanguage":
			isCJKLanguage = new(bool)
			*isCJKLanguage = cast.ToBool(v)
		case "translationkey":
			pm.translationKey = cast.ToString(v)
			pm.params[loki] = pm.translationKey
		case "resources":
			var resources []map[string]any
			handled := true

			switch vv := v.(type) {
			case []map[any]any:
				for _, vvv := range vv {
					resources = append(resources, maps.ToStringMap(vvv))
				}
			case []map[string]any:
				resources = append(resources, vv...)
			case []any:
				for _, vvv := range vv {
					switch vvvv := vvv.(type) {
					case map[any]any:
						resources = append(resources, maps.ToStringMap(vvvv))
					case map[string]any:
						resources = append(resources, vvvv)
					}
				}
			default:
				handled = false
			}

			if handled {
				pm.params[loki] = resources
				pm.resourcesMetadata = resources
				break
			}
			fallthrough

		default:
			// If not one of the explicit values, store in Params
			switch vv := v.(type) {
			case bool:
				pm.params[loki] = vv
			case string:
				pm.params[loki] = vv
			case int64, int32, int16, int8, int:
				pm.params[loki] = vv
			case float64, float32:
				pm.params[loki] = vv
			case time.Time:
				pm.params[loki] = vv
			default: // handle array of strings as well
				switch vvv := vv.(type) {
				case []any:
					if len(vvv) > 0 {
						switch vvv[0].(type) {
						case map[any]any:
							pm.params[loki] = vvv
						case map[string]any:
							pm.params[loki] = vvv
						case []any:
							pm.params[loki] = vvv
						default:
							a := make([]string, len(vvv))
							for i, u := range vvv {
								a[i] = cast.ToString(u)
							}

							pm.params[loki] = a
						}
					} else {
						pm.params[loki] = []string{}
					}
				default:
					pm.params[loki] = vv

				}
			}
		}
	}

	if !sitemapSet {
		pm.sitemap = p.s.siteCfg.sitemap
	}

	pm.markup = p.s.ContentSpec.ResolveMarkup(pm.markup)

	if draft != nil && published != nil {
		pm.draft = *draft
		p.m.s.Log.Warnf("page %q has both draft and published settings in its frontmatter. Using draft.", p.File().Filename())
	} else if draft != nil {
		pm.draft = *draft
	} else if published != nil {
		pm.draft = !*published
	}
	pm.params["draft"] = pm.draft

	if isCJKLanguage != nil {
		pm.isCJKLanguage = *isCJKLanguage
	} else if p.s.siteCfg.hasCJKLanguage && p.content.openSource != nil {
		if cjkRe.Match(p.content.mustSource()) {
			pm.isCJKLanguage = true
		} else {
			pm.isCJKLanguage = false
		}
	}

	pm.params["iscjklanguage"] = p.m.isCJKLanguage

	return nil
}

func (ps *pageState) setMetadataPre() error {
	pm := ps.m
	p := ps
	frontmatter := p.content.frontMatter

	if frontmatter != nil {
		// Needed for case insensitive fetching of params values
		maps.PrepareParams(frontmatter)
		pm.params = frontmatter
		if p.IsNode() {
			// Check for any cascade define on itself.
			if cv, found := frontmatter["cascade"]; found {
				var err error
				pm.cascade, err = page.DecodeCascade(cv)
				if err != nil {
					return err
				}

			}
		}
	} else {
		pm.params = make(maps.Params)
	}

	if true {
		return nil
	}

	return nil

}

func (p *pageMeta) noListAlways() bool {
	return p.buildConfig.List != pagemeta.Always
}

// shouldList returns whether this page should be included in the list of pages.
// glogal indicates site.Pages etc.
func (p *pageMeta) shouldList(global bool) bool {
	if p.isStandalone() {
		// Never list 404, sitemap and similar.
		return false
	}

	switch p.buildConfig.List {
	case pagemeta.Always:
		return true
	case pagemeta.Never:
		return false
	case pagemeta.ListLocally:
		return !global
	}
	return false
}

func (p *pageMeta) shouldBeCheckedForMenuDefinitions() bool {
	if !p.shouldList(false) {
		return false
	}

	return p.kind == pagekinds.Home || p.kind == pagekinds.Section || p.kind == pagekinds.Page
}

func (p *pageMeta) isStandalone() bool {
	return !p.standaloneOutputFormat.IsZero()
}

func (p *pageMeta) noRender() bool {
	return p.buildConfig.Render != pagemeta.Always
}

func (p *pageMeta) noLink() bool {
	return p.buildConfig.Render == pagemeta.Never
}

func (p *pageMeta) applyDefaultValues() error {
	if p.buildConfig.IsZero() {
		p.buildConfig, _ = pagemeta.DecodeBuildConfig(nil)
	}

	if !p.s.isEnabled(p.Kind()) {
		(&p.buildConfig).Disable()
	}

	if p.markup == "" {
		if p.File() != nil {
			// Fall back to file extension
			p.markup = p.s.ContentSpec.ResolveMarkup(p.File().Ext())
		}
		if p.markup == "" {
			p.markup = "markdown"
		}
	}

	if p.title == "" && p.f == nil {
		switch p.Kind() {
		case pagekinds.Home:
			p.title = p.s.Info.title
		case pagekinds.Section:
			sectionName := p.pathInfo.BaseNameNoIdentifier()
			sectionName = helpers.FirstUpper(sectionName)
			if p.s.Cfg.GetBool("pluralizeListTitles") {
				p.title = flect.Pluralize(sectionName)
			} else {
				p.title = sectionName
			}
		case pagekinds.Term, pagekinds.Taxonomy:
			p.title = strings.Replace(p.s.titleFunc(p.pathInfo.BaseNameNoIdentifier()), "-", " ", -1)
		case pagekinds.Status404:
			p.title = "404 Page not found"
		}
	}

	if p.IsNode() {
		p.bundleType = files.ContentClassBranch
	} else if p.File() != nil {
		class := p.File().FileInfo().Meta().Classifier
		switch class {
		case files.ContentClassBranch, files.ContentClassLeaf, files.ContentClassContent:
			p.bundleType = class

		}

	}

	return nil
}

func (p *pageMeta) newContentConverter(ps *pageState, markup string) (converter.Converter, error) {
	if ps == nil {
		panic("no Page provided")
	}
	cp := p.s.ContentSpec.Converters.Get(markup)
	if cp == nil {
		return converter.NopConverter, fmt.Errorf("no content renderer found for markup %q", p.markup)
	}

	var id string
	var filename string
	if !p.f.IsZero() {
		id = p.f.UniqueID()
		filename = p.f.Filename()
	}

	cpp, err := cp.New(
		converter.DocumentContext{
			Document:     newPageForRenderHook(ps),
			DocumentID:   id,
			DocumentName: p.Path(),
			Filename:     filename,
		},
	)
	if err != nil {
		return converter.NopConverter, err
	}

	return cpp, nil
}

// The output formats this page will be rendered to.
func (m *pageMeta) outputFormats() output.Formats {
	if len(m.configuredOutputFormats) > 0 {
		return m.configuredOutputFormats
	}

	return m.s.outputFormats[m.Kind()]
}

func (p *pageMeta) Slug() string {
	return p.urlPaths.Slug
}

func getParam(m resource.ResourceParamsProvider, key string, stringToLower bool) any {
	v := m.Params()[strings.ToLower(key)]

	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case bool:
		return val
	case string:
		if stringToLower {
			return strings.ToLower(val)
		}
		return val
	case int64, int32, int16, int8, int:
		return cast.ToInt(v)
	case float64, float32:
		return cast.ToFloat64(v)
	case time.Time:
		return val
	case []string:
		if stringToLower {
			return helpers.SliceToLower(val)
		}
		return v
	default:
		return v
	}
}

func getParamToLower(m resource.ResourceParamsProvider, key string) any {
	return getParam(m, key, true)
}
