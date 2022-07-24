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
	"net/url"

	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/output"

	"github.com/gohugoio/hugo/resources/page"
)

func newPagePaths(ps *pageState) (pagePaths, error) {
	s := ps.s
	pm := ps.m

	targetPathDescriptor, err := createTargetPathDescriptorNew(ps)
	if err != nil {
		return pagePaths{}, err
	}

	var outputFormats output.Formats

	if ps.m.isStandalone() {
		outputFormats = output.Formats{ps.m.standaloneOutputFormat}
	} else {
		outputFormats = pm.outputFormats()
		if len(outputFormats) == 0 {
			return pagePaths{}, nil
		}

		if pm.noRender() {
			outputFormats = outputFormats[:1]
		}
	}

	pageOutputFormats := make(page.OutputFormats, len(outputFormats))
	targets := make(map[string]targetPathsHolder)

	for i, f := range outputFormats {
		desc := targetPathDescriptor
		desc.Type = f
		paths := page.CreateTargetPaths(desc)
		var relPermalink, permalink string

		// If a page is headless or bundled in another,
		// it will not get published on its own and it will have no links.
		// We also check the build options if it's set to not render or have
		// a link.
		if !pm.noLink() && !pm.bundled {
			relPermalink = paths.RelPermalink(s.PathSpec)
			permalink = paths.PermalinkForOutputFormat(s.PathSpec, f)
		}

		pageOutputFormats[i] = page.NewOutputFormat(relPermalink, permalink, len(outputFormats) == 1, f)

		// Use the main format for permalinks, usually HTML.
		permalinksIndex := 0
		if f.Permalinkable {
			// Unless it's permalinkable
			permalinksIndex = i
		}

		targets[f.Name] = targetPathsHolder{
			paths:        paths,
			OutputFormat: pageOutputFormats[permalinksIndex],
		}

	}

	var out page.OutputFormats
	if !pm.noLink() {
		out = pageOutputFormats
	}

	return pagePaths{
		outputFormats:        out,
		firstOutputFormat:    pageOutputFormats[0],
		targetPaths:          targets,
		targetPathDescriptor: targetPathDescriptor,
	}, nil
}

type pagePaths struct {
	outputFormats     page.OutputFormats
	firstOutputFormat page.OutputFormat

	targetPaths          map[string]targetPathsHolder
	targetPathDescriptor page.TargetPathDescriptor
}

func (l pagePaths) OutputFormats() page.OutputFormats {
	return l.outputFormats
}

// TODO1
func createTargetPathDescriptorNew(p *pageState) (page.TargetPathDescriptor, error) {
	s := p.s
	d := s.Deps
	pm := p.m
	pi := pm.pathInfo

	alwaysInSubDir := p.Kind() == pagekinds.Sitemap

	desc := page.TargetPathDescriptor{
		PathSpec:    d.PathSpec,
		Kind:        p.Kind(),
		Sections:    p.SectionsEntries(),
		UglyURLs:    s.Info.uglyURLs(p),
		ForcePrefix: s.h.IsMultihost() || alwaysInSubDir,
		Dir:         pi.ContainerDir(),
		URL:         pm.urlPaths.URL,
	}

	if pm.Slug() != "" {
		desc.BaseName = pm.Slug()
	} else {
		desc.BaseName = pm.pathInfo.BaseNameNoIdentifier()
	}

	desc.PrefixFilePath = s.getLanguageTargetPathLang(alwaysInSubDir)
	desc.PrefixLink = s.getLanguagePermalinkLang(alwaysInSubDir)

	// Expand only pagekinds.KindPage and pagekinds.KindTaxonomy; don't expand other Kinds of Pages
	// like pagekinds.KindSection or pagekinds.KindTaxonomyTerm because they are "shallower" and
	// the permalink configuration values are likely to be redundant, e.g.
	// naively expanding /category/:slug/ would give /category/categories/ for
	// the "categories" pagekinds.KindTaxonomyTerm.
	if p.Kind() == pagekinds.Page || p.Kind() == pagekinds.Term {
		opath, err := d.ResourceSpec.Permalinks.Expand(p.Section(), p)
		if err != nil {
			return desc, err
		}

		if opath != "" {
			opath, _ = url.QueryUnescape(opath)
			desc.ExpandedPermalink = opath
		}

	}

	return desc, nil
}
