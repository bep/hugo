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
	"sync"

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/compare"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/lazy"
	"github.com/gohugoio/hugo/navigation"
	"github.com/gohugoio/hugo/output/layouts"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
	"github.com/gohugoio/hugo/source"
)

type nextPrevProvider interface {
	getNextPrev() *nextPrev
}

func (p *pageCommon) getNextPrev() *nextPrev {
	return p.posNextPrev
}

type nextPrevInSectionProvider interface {
	getNextPrevInSection() *nextPrev
}

func (p *pageCommon) getNextPrevInSection() *nextPrev {
	return p.posNextPrevSection
}

type pageCommon struct {
	s *Site
	m *pageMeta

<<<<<<< HEAD
	sWrapped page.Site

	bucket  *pagesMapBucket
	treeRef *contentTreeRef
=======
	dependencyManagerPage identity.Manager
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)

	// Lazily initialized dependencies.
	init *lazy.Init

	// Store holds state that survives server rebuilds.
	store *maps.Scratch

	// All of these represents the common parts of a page.Page
	maps.Scratcher
	navigation.PageMenusProvider
	page.AuthorProvider
	page.AlternativeOutputFormatsProvider
	page.ChildCareProvider
	page.FileProvider
	page.GetPageProvider
	page.GitInfoProvider
	page.InSectionPositioner
	page.OutputFormatsProvider
	page.PageMetaProvider
	page.Positioner
	page.RawContentProvider
	page.RelatedKeywordsProvider
	page.RefProvider
	page.ShortcodeInfoProvider
	page.SitesProvider
	page.TranslationsProvider
	page.TreeProvider
	resource.LanguageProvider
	resource.ResourceDataProvider
	resource.ResourceMetaProvider
	resource.ResourceParamsProvider
	resource.ResourceTypeProvider
	resource.MediaTypeProvider
	resource.TranslationKeyProvider
	compare.Eqer

	// Describes how paths and URLs for this page and its descendants
	// should look like.
	targetPathDescriptor page.TargetPathDescriptor

	layoutDescriptor     layouts.LayoutDescriptor
	layoutDescriptorInit sync.Once

	// The source and the parsed page content.
	content *cachedContent

	// Set if feature enabled and this is in a Git repo.
	gitInfo    source.GitInfo
	codeowners []string

	// Positional navigation
	posNextPrev        *nextPrev
	posNextPrevSection *nextPrev

	// Menus
	pageMenus *pageMenus

	// Internal use
	page.InternalDependencies

	// Any bundled resources
	resourcesPublishInit sync.Once
}

func (p *pageCommon) Store() *maps.Scratch {
	return p.store
}

func (p *pageCommon) GetDependencyManager() identity.Manager {
	return p.dependencyManagerPage
}

func (p *pageCommon) IdentifierBase() any {
	return p.Path()
}
