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

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/lazy"
	"github.com/gohugoio/hugo/parser/pageparser"
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
		m.kind = pagekinds.Page
		if m.pathInfo.Base() == "" {
			m.kind = pagekinds.Home
		} else if m.pathInfo.IsBranchBundle() {
			// TODO1
			m.kind = pagekinds.Section
		}
	}

	// Parse page content.
	var pc pageContent
	if m.f != nil {
		fi := m.f.FileInfo()
		r, err := fi.Meta().Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()

		parsed, err := pageparser.Parse(
			r,
			pageparser.Config{EnableEmoji: m.s.siteCfg.enableEmoji},
		)

		if err != nil {
			return nil, err
		}

		pc = pageContent{
			source: rawPageContent{
				parsed:         parsed,
				posMainContent: -1,
				posSummaryEnd:  -1,
				posBodyStart:   -1,
			},
		}
	}

	var dependencyManager identity.Manager = identity.NopManager
	if m.s.running() {
		dependencyManager = identity.NewManager(identity.Anonymous)
	}

	ps := &pageState{
		pageOutput:                        nopPageOutput,
		pageOutputTemplateVariationsState: atomic.NewUint32(0),
		pageCommon: &pageCommon{
			pageContent:             pc,
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

			dependencyManagerPage: dependencyManager,
			InternalDependencies:  m.s,
			init:                  lazy.New(),
			m:                     m,
			s:                     m.s,
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

	ps.shortcodeState = newShortcodeHandler(ps)
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

	meta, err := ps.mapContent(m)
	if err != nil {
		return nil, ps.wrapError(err)
	}

	if err := ps.setMetadataPre(meta); err != nil {
		return nil, ps.wrapError(err)
	}

	if err := ps.initLazyProviders(); err != nil {
		return nil, ps.wrapError(err)
	}

	return ps, nil

}
