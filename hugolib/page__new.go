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
	"html/template"
	"strings"

	"go.uber.org/atomic"

	"github.com/gohugoio/hugo/common/hugo"
	"fmt"

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/lazy"
	"github.com/gohugoio/hugo/resources"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/page/pagekinds"
	"go.uber.org/atomic"
)

// bookmark
func (h *HugoSites) newPage(m *pageMeta) (*pageState, error) {
	if m.pathInfo != nil {
		if m.f != nil {
			m.pathInfo = m.f.FileInfo().Meta().PathInfo
		}
		if m.pathInfo == nil {
			panic(fmt.Sprintf("missing pathInfo in %v", m))
		}
	}

	m.Staler = &resources.AtomicStaler{}

	if m.s == nil {
		// Identify the Site/language to associate this Page with.
		var lang string
		if m.f != nil {
			lang = m.f.Lang()
		} else {
			lang = m.pathInfo.Lang()
		}

		if lang == "" {
			return nil, fmt.Errorf("no language set for %q", m.pathInfo.Path())
		}
		m.s = h.Sites[0]
		for _, ss := range h.Sites {
			if ss.Lang() == lang {
				m.s = ss
				break
			}
		}
	}

	// Identify Page Kind.
	if m.kind == "" {
		m.kind = pagekinds.Section
		if m.pathInfo.Base() == "/" {
			m.kind = pagekinds.Home
		} else if m.pathInfo.IsBranchBundle() {
			// A section, taxonomy or term.
			tc := m.s.pageMap.cfg.getTaxonomyConfig(m.Path())
			if !tc.IsZero() {
				// Either a taxonomy or a term.
				if tc.pluralTreeKey == m.Path() {
					m.kind = pagekinds.Taxonomy
				} else {
					m.kind = pagekinds.Term
				}
			}
		} else if m.f != nil {
			m.kind = pagekinds.Page
		}
	}

	// Parse page content.
	cachedContent, err := newCachedContent(m)
	if err != nil {
		return nil, err
	}

	// bookmark
	var dependencyManager identity.Manager = identity.NopManager
	if m.s.running() {
		dependencyManager = identity.NewManager(identity.Anonymous)
	}

	ps := &pageState{
		pageOutput:                        nopPageOutput,
		pageOutputTemplateVariationsState: atomic.NewUint32(0),
		Staler:                            m,
		pageCommon: &pageCommon{
			content:                 cachedContent,
			FileProvider:            m,
			AuthorProvider:          m,
			Scratcher:               maps.NewScratcher(),
			store:                   maps.NewScratch(),
			Positioner:              page.NopPage,
			InSectionPositioner:     page.NopPage,
			ResourceMetaProvider:    m,
			ResourceParamsProvider:  m,
			PageMetaProvider:        m,
			RelatedKeywordsProvider: m,
			OutputFormatsProvider:   page.NopPage,
			ResourceTypeProvider:    pageTypesProvider,
			MediaTypeProvider:       pageTypesProvider,
			RefProvider:             page.NopPage,
			ShortcodeInfoProvider:   page.NopPage,
			LanguageProvider:        m.s,

<<<<<<< HEAD
			InternalDependencies: s,
			init:                 lazy.New(),
			m:                    metaProvider,
			s:                    s,
			sWrapped:             page.WrapSite(s),
=======
			dependencyManagerPage: dependencyManager,
			InternalDependencies:  m.s,
			init:                  lazy.New(),
			m:                     m,
			s:                     m.s,
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
		},
	}

	if m.f != nil {
		gi, err := m.s.h.gitInfoForPage(ps)
		if err != nil {
			return nil, fmt.Errorf("failed to load Git data: %w", err)
		}
		ps.gitInfo = gi

		owners, err := m.s.h.codeownersForPage(ps)
		if err != nil {
			return nil, fmt.Errorf("failed to load CODEOWNERS: %w", err)
		}
		ps.codeowners = owners
	}

	ps.pageMenus = &pageMenus{p: ps}
	ps.PageMenusProvider = ps.pageMenus
	ps.GetPageProvider = pageSiteAdapter{s: m.s, p: ps}
	ps.GitInfoProvider = ps
	ps.TranslationsProvider = ps
	ps.ResourceDataProvider = &pageData{pageState: ps}
	ps.RawContentProvider = ps
	ps.ChildCareProvider = ps
	ps.TreeProvider = pageTree{p: ps}
	ps.Eqer = ps
	ps.TranslationKeyProvider = ps
	ps.ShortcodeInfoProvider = ps
	ps.AlternativeOutputFormatsProvider = ps

	if err := ps.setMetadataPre(); err != nil {
		return nil, ps.wrapError(err)
	}

	if err := ps.initLazyProviders(); err != nil {
		return nil, ps.wrapError(err)
	}

	return ps, nil
<<<<<<< HEAD
}

func newPageBucket(p *pageState) *pagesMapBucket {
	return &pagesMapBucket{owner: p, pagesMapBucketPages: &pagesMapBucketPages{}}
}

func newPageFromMeta(
	n *contentNode,
	parentBucket *pagesMapBucket,
	meta map[string]any,
	metaProvider *pageMeta) (*pageState, error) {
	if metaProvider.f == nil {
		metaProvider.f = page.NewZeroFile(metaProvider.s.LogDistinct)
	}

	ps, err := newPageBase(metaProvider)
	if err != nil {
		return nil, err
	}

	bucket := parentBucket

	if ps.IsNode() {
		ps.bucket = newPageBucket(ps)
	}

	if meta != nil || parentBucket != nil {
		if err := metaProvider.setMetadata(bucket, ps, meta); err != nil {
			return nil, ps.wrapError(err)
		}
	}

	if err := metaProvider.applyDefaultValues(n); err != nil {
		return nil, err
	}

	ps.init.Add(func(context.Context) (any, error) {
		pp, err := newPagePaths(metaProvider.s, ps, metaProvider)
		if err != nil {
			return nil, err
		}

		makeOut := func(f output.Format, render bool) *pageOutput {
			return newPageOutput(ps, pp, f, render)
		}

		shouldRenderPage := !ps.m.noRender()

		if ps.m.standalone {
			ps.pageOutput = makeOut(ps.m.outputFormats()[0], shouldRenderPage)
		} else {
			outputFormatsForPage := ps.m.outputFormats()

			// Prepare output formats for all sites.
			// We do this even if this page does not get rendered on
			// its own. It may be referenced via .Site.GetPage and
			// it will then need an output format.
			ps.pageOutputs = make([]*pageOutput, len(ps.s.h.renderFormats))
			created := make(map[string]*pageOutput)
			for i, f := range ps.s.h.renderFormats {
				po, found := created[f.Name]
				if !found {
					render := shouldRenderPage
					if render {
						_, render = outputFormatsForPage.GetByName(f.Name)
					}
					po = makeOut(f, render)
					created[f.Name] = po
				}
				ps.pageOutputs[i] = po
			}
		}

		if err := ps.initCommonProviders(pp); err != nil {
			return nil, err
		}

		return nil, nil
	})

	return ps, err
}

// Used by the legacy 404, sitemap and robots.txt rendering
func newPageStandalone(m *pageMeta, f output.Format) (*pageState, error) {
	m.configuredOutputFormats = output.Formats{f}
	m.standalone = true
	p, err := newPageFromMeta(nil, nil, nil, m)
	if err != nil {
		return nil, err
	}

	if err := p.initPage(); err != nil {
		return nil, err
	}

	return p, nil
}

type pageDeprecatedWarning struct {
	p *pageState
}

func (p *pageDeprecatedWarning) IsDraft() bool          { return p.p.m.draft }
func (p *pageDeprecatedWarning) Hugo() hugo.HugoInfo    { return p.p.s.Hugo() }
func (p *pageDeprecatedWarning) LanguagePrefix() string { return p.p.s.GetLanguagePrefix() }
func (p *pageDeprecatedWarning) GetParam(key string) any {
	return p.p.m.params[strings.ToLower(key)]
}

func (p *pageDeprecatedWarning) RSSLink() template.URL {
	f := p.p.OutputFormats().Get("RSS")
	if f == nil {
		return ""
	}
	return template.URL(f.Permalink())
}

func (p *pageDeprecatedWarning) URL() string {
	if p.p.IsPage() && p.p.m.urlPaths.URL != "" {
		// This is the url set in front matter
		return p.p.m.urlPaths.URL
	}
	// Fall back to the relative permalink.
	return p.p.RelPermalink()
=======

>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}
