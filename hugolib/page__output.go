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
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
)

func newPageOutput(
	ps *pageState,
	pp pagePaths,
	f output.Format,
	render bool) *pageOutput {

	var targetPathsProvider targetPathsHolder
	var linksProvider resource.ResourceLinksProvider

	ft, found := pp.targetPaths[f.Name]
	if !found {
		// Link to the main output format
		ft = pp.targetPaths[pp.firstOutputFormat.Format.Name]
	}
	targetPathsProvider = ft
	linksProvider = ft

	var paginatorProvider page.PaginatorProvider = page.NopPage
	var pag *pagePaginator

	if render && ps.IsNode() {
		pag = newPagePaginator(ps)
		paginatorProvider = pag
	}

	providers := struct {
		page.PaginatorProvider
		resource.ResourceLinksProvider
		targetPather
	}{
		paginatorProvider,
		linksProvider,
		targetPathsProvider,
	}

	var dependencyManager identity.Manager = identity.NopManager
	if ps.s.running() {
		dependencyManager = identity.NewManager(identity.Anonymous)
	}

	po := &pageOutput{
		f:                       f,
		dependencyManagerOutput: dependencyManager,
		pagePerOutputProviders:  providers,
		ContentProvider:         page.NopPage,
		TableOfContentsProvider: page.NopPage,
		PageRenderProvider:      page.NopPage,
		render:                  render,
		paginator:               pag,
		ps:                      ps,
	}

	return po
}

// We create a pageOutput for every output format combination, even if this
// particular page isn't configured to be rendered to that format.
type pageOutput struct {
	// Enabled if this page is configured to be rendered to this format.
	render bool

	f output.Format

	// Only set if render is set.
	// Note that this will be lazily initialized, so only used if actually
	// used in template(s).
	paginator *pagePaginator

	// These interface provides the functionality that is specific for this
	// output format.
	pagePerOutputProviders
	page.ContentProvider
	page.TableOfContentsProvider
	page.PageRenderProvider

	// We have one per output so we can do a fine grained page resets.
	dependencyManagerOutput identity.Manager

	ps *pageState

	// May be nil.
	cp *pageContentOutput

	renderState int
}

func (po *pageOutput) Reset() {
	po.cp.Reset()
	po.dependencyManagerOutput.Reset()
	po.renderState = 0
}

func (o *pageOutput) GetDependencyManager() identity.Manager {
	return o.dependencyManagerOutput
}

func (p *pageOutput) setContentProvider(cp *pageContentOutput) {
	if cp == nil {
		return
	}
	p.ContentProvider = cp
	p.TableOfContentsProvider = cp
	p.PageRenderProvider = cp
	p.cp = cp
}

func (p *pageOutput) enablePlaceholders() {
	if p.cp != nil {
		p.cp.enablePlaceholders()
	}
}
