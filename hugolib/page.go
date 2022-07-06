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
	"bytes"
	"context"
	"fmt"
	"path"
	"html/template"
	"os"
	"path/filepath"

	"go.uber.org/atomic"

	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/output/layouts"
	"github.com/gohugoio/hugo/related"

	"github.com/gohugoio/hugo/markup/converter"
	"github.com/gohugoio/hugo/markup/tableofcontents"

	"github.com/gohugoio/hugo/tpl"

	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/bep/gitmap"

	"github.com/gohugoio/hugo/helpers"

	"github.com/gohugoio/hugo/common/herrors"

	"github.com/gohugoio/hugo/source"
	"github.com/gohugoio/hugo/output"

	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/resources/page/pagekinds"

	"github.com/gohugoio/hugo/common/collections"
	"github.com/gohugoio/hugo/common/text"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
)

var (
	_ page.Page                          = (*pageState)(nil)
	_ collections.Grouper                = (*pageState)(nil)
	_ collections.Slicer                 = (*pageState)(nil)
	_ identity.DependencyManagerProvider = (*pageState)(nil)
)

var (
	pageTypesProvider = resource.NewResourceTypesProvider(media.Builtin.OctetType, pageResourceType)
	nopPageOutput     = &pageOutput{
		pagePerOutputProviders: nopPagePerOutput,
		ContentProvider:        page.NopPage,
	}
)

// pageContext provides contextual information about this page, for error
// logging and similar.
type pageContext interface {
	posOffset(offset int) text.Position
	wrapError(err error) error
	getContentConverter() converter.Converter
	addDependency(dep identity.Identity)
}

// wrapErr adds some context to the given error if possible.
func wrapErr(err error, ctx any) error {
	if pc, ok := ctx.(pageContext); ok {
		return pc.wrapError(err)
	}
	return err
}

type pageSiteAdapter struct {
	p page.Page
	s *Site
}

func (pa pageSiteAdapter) GetPage(ref string) (page.Page, error) {
	p, err := pa.s.getPageNew(pa.p, ref)
	if p == nil {
		// The nil struct has meaning in some situations, mostly to avoid breaking
		// existing sites doing $nilpage.IsDescendant($p), which will always return
		// false.
		p = page.NilPage
	}
	return p, err
}

type pageState struct {
	// This slice will be of same length as the number of global slice of output
	// formats (for all sites).
	// TODO1 update doc
	pageOutputs []*pageOutput

	// Used to determine if we can reuse content across output formats.
	pageOutputTemplateVariationsState *atomic.Uint32

	// This will be shifted out when we start to render a new output format.
	pageOutputIdx int
	*pageOutput

	// Common for all output formats.
	*pageCommon

	resource.Staler
}

func (p *pageState) reusePageOutputContent() bool {
	return p.pageOutputTemplateVariationsState.Load() == 1
}

func (p *pageState) Err() resource.ResourceError {
	return nil
}

// Eq returns whether the current page equals the given page.
// This is what's invoked when doing `{{ if eq $page $otherPage }}`
func (p *pageState) Eq(other any) bool {
	pp, err := unwrapPage(other)
	if err != nil {
		return false
	}

	return p == pp
}

<<<<<<< HEAD
// GetIdentity is for internal use.
func (p *pageState) GetIdentity() identity.Identity {
	return identity.NewPathIdentity(files.ComponentFolderContent, filepath.FromSlash(p.Pathc()))
=======
func (p *pageState) GetDependencyManager() identity.Manager {
	return p.dependencyManagerPage
}

func (p *pageState) IdentifierBase() any {
	return p.Path()
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}

func (p *pageState) HeadingsFiltered(context.Context) tableofcontents.Headings {
	return nil
}

type pageHeadingsFiltered struct {
	*pageState
	headings tableofcontents.Headings
}

func (p *pageHeadingsFiltered) HeadingsFiltered(context.Context) tableofcontents.Headings {
	return p.headings
}

func (p *pageHeadingsFiltered) page() page.Page {
	return p.pageState
}

// For internal use by the related content feature.
func (p *pageState) ApplyFilterToHeadings(ctx context.Context, fn func(*tableofcontents.Heading) bool) related.Document {
	if p.pageOutput.cp.tableOfContents == nil {
		return p
	}
	headings := p.pageOutput.cp.tableOfContents.Headings.FilterBy(fn)
	return &pageHeadingsFiltered{
		pageState: p,
		headings:  headings,
	}
}

func (p *pageState) GitInfo() source.GitInfo {
	return p.gitInfo
}

func (p *pageState) CodeOwners() []string {
	return p.codeowners
}

// GetTerms gets the terms defined on this page in the given taxonomy.
// The pages returned will be ordered according to the front matter.
func (p *pageState) GetTerms(taxonomy string) page.Pages {
	return p.s.pageMap.getTermsForPageInTaxonomy(p.Path(), taxonomy)
}

func (p *pageState) MarshalJSON() ([]byte, error) {
	return page.MarshalPageToJSON(p)
}

func (p *pageState) RegularPagesRecursive() page.Pages {
	switch p.Kind() {
	case pagekinds.Section, pagekinds.Home:
		return p.s.pageMap.getPagesInSection(
			pageMapQueryPagesInSection{
				pageMapQueryPagesBelowPath: pageMapQueryPagesBelowPath{
					Path:         p.Path(),
					KindsInclude: pagekinds.Page,
				},
				Recursive: true,
			},
		)
	default:
		return p.RegularPages()
	}
}

func (p *pageState) RegularPages() page.Pages {
	switch p.Kind() {
	case pagekinds.Page:
	case pagekinds.Section, pagekinds.Home, pagekinds.Taxonomy:
		return p.s.pageMap.getPagesInSection(
			pageMapQueryPagesInSection{
				pageMapQueryPagesBelowPath: pageMapQueryPagesBelowPath{
					Path:         p.Path(),
					KindsInclude: pagekinds.Page,
				},
			},
		)
	case pagekinds.Term:
		return p.s.pageMap.getPagesWithTerm(
			pageMapQueryPagesBelowPath{
				Path:         p.Path(),
				KindsInclude: pagekinds.Page,
			},
		)
	default:
		return p.s.RegularPages()
	}
	return nil
}

func (p *pageState) Pages() page.Pages {
	switch p.Kind() {
	case pagekinds.Page:
	case pagekinds.Section, pagekinds.Home:
		return p.s.pageMap.getPagesInSection(
			pageMapQueryPagesInSection{
				pageMapQueryPagesBelowPath: pageMapQueryPagesBelowPath{
					Path: p.Path(),
				},
			},
		)
	case pagekinds.Term:
		return p.s.pageMap.getPagesWithTerm(
			pageMapQueryPagesBelowPath{
				Path: p.Path(),
			},
		)
	case pagekinds.Taxonomy:
		return p.s.pageMap.getPagesInSection(
			pageMapQueryPagesInSection{
				pageMapQueryPagesBelowPath: pageMapQueryPagesBelowPath{
					Path:         p.Path(),
					KindsInclude: pagekinds.Term,
				},
				Recursive: true,
			},
		)
	default:
		return p.s.Pages()
	}
	return nil
}

// RawContent returns the un-rendered source content without
// any leading front matter.
func (p *pageState) RawContent() string {
	source, err := p.content.contentSource()
	if err != nil {
		panic(err)
	}
	if p.content.items == nil {
		return ""
	}
	start := p.content.posMainContent
	if start == -1 {
		start = 0
	}
	return string(source[start:])
}

func (p *pageState) Resources() resource.Resources {
	return p.s.pageMap.getResourcesForPage(p)
}

func (p *pageState) HasShortcode(name string) bool {
	if p.content.shortcodeState == nil {
		return false
	}

	return p.content.shortcodeState.hasName(name)
}

func (p *pageState) Site() page.Site {
	return p.sWrapped
}

func (p *pageState) String() string {
	if pth := p.Path(); pth != "" {
		return fmt.Sprintf("Page(%s)", helpers.AddLeadingSlash(filepath.ToSlash(pth)))
	}
	return fmt.Sprintf("Page(%q)", p.Title())
}

// IsTranslated returns whether this content file is translated to
// other language(s).
func (p *pageState) IsTranslated() bool {
<<<<<<< HEAD
	p.s.h.init.translations.Do(context.Background())
	return len(p.translations) > 0
=======
	return len(p.Translations()) > 0
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}

// TODO1 deprecate
func (p *pageState) TranslationKey() string {
	return p.Path()
}

// AllTranslations returns all translations, including the current Page.
func (p *pageState) AllTranslations() page.Pages {
<<<<<<< HEAD
	p.s.h.init.translations.Do(context.Background())
	return p.allTranslations
=======
	cacheKey := p.Path() + "/" + "all-translations"
	pages, err := p.s.pageMap.getOrCreatePagesFromCache(cacheKey, func(string) (page.Pages, error) {
		all := p.s.pageMap.treePages.GetDimension(p.Path(), pageTreeDimensionLanguage)
		var pas page.Pages
		for _, p := range all {
			if p == nil {
				continue
			}
			pas = append(pas, p.(page.Page))
		}
		return pas, nil
	})

	if err != nil {
		panic(err)
	}

	return pages

>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}

// Translations returns the translations excluding the current Page.
func (p *pageState) Translations() page.Pages {
<<<<<<< HEAD
	p.s.h.init.translations.Do(context.Background())
	return p.translations
=======
	cacheKey := p.Path() + "/" + "translations"
	pages, err := p.s.pageMap.getOrCreatePagesFromCache(cacheKey, func(string) (page.Pages, error) {
		var pas page.Pages
		for _, pp := range p.AllTranslations() {
			if !pp.Eq(p) {
				pas = append(pas, pp)
			}
		}
		return pas, nil
	})
	if err != nil {
		panic(err)
	}
	return pages
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}

func (ps *pageState) initCommonProviders(pp pagePaths) error {
	if ps.IsPage() {
		ps.posNextPrev = &nextPrev{init: ps.s.init.prevNext}
		ps.posNextPrevSection = &nextPrev{init: ps.s.init.prevNextInSection}
		ps.InSectionPositioner = newPagePositionInSection(ps.posNextPrevSection)
		ps.Positioner = newPagePosition(ps.posNextPrev)
	}

	ps.OutputFormatsProvider = pp
	ps.targetPathDescriptor = pp.targetPathDescriptor
	ps.RefProvider = newPageRef(ps)
	ps.SitesProvider = ps.s

	return nil
}

func (p *pageState) getLayoutDescriptor() layouts.LayoutDescriptor {
	p.layoutDescriptorInit.Do(func() {
		var section string
		sections := p.SectionsEntries()
		switch p.Kind() {
		case pagekinds.Section:
			if len(sections) > 0 {
				section = sections[0]
			}
		case pagekinds.Taxonomy, pagekinds.Term:
			// TODO1, singular
			section = p.SectionsEntries()[0]
		default:
		}

		p.layoutDescriptor = layouts.LayoutDescriptor{
			Kind:    p.Kind(),
			Type:    p.Type(),
			Lang:    p.Language().Lang,
			Layout:  p.Layout(),
			Section: section,
		}
	})

	return p.layoutDescriptor
}

func (p *pageState) resolveTemplate(layouts ...string) (tpl.Template, bool, error) {
	f := p.outputFormat()

	/*if len(layouts) == 0 {
		// TODO1
		selfLayout := p.selfLayoutForOutput(f)
		if selfLayout != "" {
			templ, found := p.s.Tmpl().Lookup(selfLayout)
			return templ, found, nil
		}
	}*/

	d := p.getLayoutDescriptor()

	if len(layouts) > 0 {
		d.Layout = layouts[0]
		d.LayoutOverride = true
	}

	tp, found, err := p.s.Tmpl().LookupLayout(d, f)

	return tp, found, err
}

// This is serialized.
func (p *pageState) initOutputFormat(isRenderingSite bool, idx int) error {
	if err := p.shiftToOutputFormat(isRenderingSite, idx); err != nil {
		return err
	}

	return nil
}

// Must be run after the site section tree etc. is built and ready.
func (p *pageState) initPage() error {
	if _, err := p.init.Do(context.Background()); err != nil {
		return err
	}
	return nil
}

func (p *pageState) renderResources() (err error) {
	p.resourcesPublishInit.Do(func() {
		for _, r := range p.Resources() {

			if _, ok := r.(page.Page); ok {
				// Pages gets rendered with the owning page but we count them here.
				p.s.PathSpec.ProcessingStats.Incr(&p.s.PathSpec.ProcessingStats.Pages)
				continue
			}

			src, ok := r.(resource.Source)
			if !ok {
				err = fmt.Errorf("Resource %T does not support resource.Source", src)
				return
			}

			if err := src.Publish(); err != nil {
<<<<<<< HEAD
				if herrors.IsNotExist(err) {
					// The resource has been deleted from the file system.
					// This should be extremely rare, but can happen on live reload in server
					// mode when the same resource is member of different page bundles.
					toBeDeleted = append(toBeDeleted, i)
				} else {
=======
				if !os.IsNotExist(err) {
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
					p.s.Log.Errorf("Failed to publish Resource for page %q: %s", p.pathOrTitle(), err)
				}
			} else {
				p.s.PathSpec.ProcessingStats.Incr(&p.s.PathSpec.ProcessingStats.Files)
			}
		}

	})

	return
}

func (p *pageState) getTargetPaths() page.TargetPaths {
	return p.targetPaths()
}

func (p *pageState) AlternativeOutputFormats() page.OutputFormats {
	f := p.outputFormat()
	var o page.OutputFormats
	for _, of := range p.OutputFormats() {
		if of.Format.NotAlternative || of.Format.Name == f.Name {
			continue
		}

		o = append(o, of)
	}
	return o
}

type renderStringOpts struct {
	Display string
	Markup  string
}

var defaultRenderStringOpts = renderStringOpts{
	Display: "inline",
	Markup:  "", // Will inherit the page's value when not set.
}

<<<<<<< HEAD
func (p *pageState) addDependency(dep identity.Provider) {
	if !p.s.watching() || p.pageOutput.cp == nil {
=======
func (p *pageState) addDependency(dep identity.Identity) {
	if !p.s.running() || p.pageOutput.cp == nil {
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
		return
	}
	p.pageOutput.dependencyManagerOutput.AddIdentity(dep)
}

func (p *pageState) Render(ctx context.Context, layout ...string) (template.HTML, error) {
	templ, found, err := p.resolveTemplate(layout...)
	if err != nil {
		return "", p.wrapError(err)
	}

	if !found {
		return "", nil
	}

	res, err := executeToString(ctx, p.s.Tmpl(), templ, p)
	if err != nil {
		return "", p.wrapError(fmt.Errorf("failed to execute template %q: %w", layout, err))
	}
	return template.HTML(res), nil
}

// wrapError adds some more context to the given error if possible/needed
func (p *pageState) wrapError(err error) error {
	if err == nil {
		panic("wrapError with nil")
	}

	if p.File().IsZero() {
		// No more details to add.
		return fmt.Errorf("%q: %w", p.Path(), err)
	}

	filename := p.File().Filename()

	// Check if it's already added.
	for _, ferr := range herrors.UnwrapFileErrors(err) {
		errfilename := ferr.Position().Filename
		if errfilename == filename {
			if ferr.ErrorContext() == nil {
				f, ioerr := p.s.SourceSpec.Fs.Source.Open(filename)
				if ioerr != nil {
					return err
				}
				defer f.Close()
				ferr.UpdateContent(f, nil)
			}
			return err
		}
	}

	lineMatcher := herrors.NopLineMatcher

	if textSegmentErr, ok := err.(*herrors.TextSegmentError); ok {
		lineMatcher = herrors.ContainsMatcher(textSegmentErr.Segment)
	}

	return herrors.NewFileErrorFromFile(err, filename, p.s.SourceSpec.Fs.Source, lineMatcher)

}

func (p *pageState) getContentConverter() converter.Converter {
	var err error
	p.m.contentConverterInit.Do(func() {
		markup := p.m.markup
		if markup == "html" {
			// Only used for shortcode inner content.
			markup = "markdown"
		}
		p.m.contentConverter, err = p.m.newContentConverter(p, markup)
	})

	if err != nil {
		p.s.Log.Errorln("Failed to create content converter:", err)
	}
	return p.m.contentConverter
}

func (p *pageState) errorf(err error, format string, a ...any) error {
	if herrors.UnwrapFileError(err) != nil {
		// More isn't always better.
		return err
	}
	args := append([]any{p.Language().Lang, p.pathOrTitle()}, a...)
	args = append(args, err)
	format = "[%s] page %q: " + format + ": %w"
	if err == nil {
		return fmt.Errorf(format, args...)
	}
	return fmt.Errorf(format, args...)
}

func (p *pageState) outputFormat() (f output.Format) {
	if p.pageOutput == nil {
		panic("no pageOutput")
	}
	return p.pageOutput.f
}

// TODO1 move these.
func parseError(err error, filename string, input []byte, offset int) error {
	pos := posFromInput(filename, input, offset)
	return herrors.NewFileErrorFromName(err, filename).UpdatePosition(pos)
}

func posFromInput(filename string, input []byte, offset int) text.Position {
	if offset < 0 {
		return text.Position{
			Filename: filename,
		}
	}
	lf := []byte("\n")
	input = input[:offset]
	lineNumber := bytes.Count(input, lf) + 1
	endOfLastLine := bytes.LastIndex(input, lf)

	return text.Position{
		Filename:     filename,
		LineNumber:   lineNumber,
		ColumnNumber: offset - endOfLastLine,
		Offset:       offset,
	}
}

func (p *pageState) pathOrTitle() string {
	if p.File() != nil {
		return p.File().Filename()
	}

	if p.Path() != "" {
		return p.Path()
	}

	return p.Title()
}

func (p *pageState) posFromPage(offset int) text.Position {
	return posFromInput(p.pathOrTitle(), p.content.mustSource(), offset)
}

func (p *pageState) posOffset(offset int) text.Position {
	return posFromInput(p.pathOrTitle(), p.content.mustSource(), offset)
}

// shiftToOutputFormat is serialized. The output format idx refers to the
// full set of output formats for all sites.
func (p *pageState) shiftToOutputFormat(isRenderingSite bool, idx int) error {
	if err := p.initPage(); err != nil {
		return err
	}

	if len(p.pageOutputs) == 1 {
		idx = 0
	}

	p.pageOutputIdx = idx
	p.pageOutput = p.pageOutputs[idx]
	if p.pageOutput == nil {
		panic(fmt.Sprintf("pageOutput is nil for output idx %d", idx))
	}

	// Reset any built paginator. This will trigger when re-rendering pages in
	// server mode.
	if isRenderingSite && p.pageOutput.paginator != nil && p.pageOutput.paginator.current != nil {
		p.pageOutput.paginator.reset()
	}

	if isRenderingSite {
		cp := p.pageOutput.cp
		if cp == nil && p.reusePageOutputContent() {
			// Look for content to reuse.
			for i := 0; i < len(p.pageOutputs); i++ {
				if i == idx {
					continue
				}
				po := p.pageOutputs[i]

				if po.cp != nil {
					cp = po.cp
					break
				}
			}
		}

		if cp == nil {
			var err error
			cp, err = newPageContentOutput(p.pageOutput)
			if err != nil {
				return err
			}
		}
		p.pageOutput.setContentProvider(cp)
	} else {
		// We attempt to assign pageContentOutputs while preparing each site
		// for rendering and before rendering each site. This lets us share
		// content between page outputs to conserve resources. But if a template
		// unexpectedly calls a method of a ContentProvider that is not yet
		// initialized, we assign a LazyContentProvider that performs the
		// initialization just in time.
		if lcp, ok := (p.pageOutput.ContentProvider.(*page.LazyContentProvider)); ok {
			lcp.Reset()
		} else {
			lcp = page.NewLazyContentProvider(func() (page.OutputFormatContentProvider, error) {
				cp, err := newPageContentOutput(p.pageOutput)
				if err != nil {
					return nil, err
				}
				return cp, nil
			})
			p.pageOutput.contentRenderer = lcp
			p.pageOutput.ContentProvider = lcp
			p.pageOutput.PageRenderProvider = lcp
			p.pageOutput.TableOfContentsProvider = lcp
		}
	}

	return nil
}

var (
	_ contentNodeI = (*pageState)(nil)
)

// isContentNodeBranch
func (p *pageState) isContentNodeBranch() bool {
	return p.IsNode()
}

func (p *pageState) isContentNodeResource() bool {
	return p.m.bundled
}

var (
	_ page.Page         = (*pageWithOrdinal)(nil)
	_ collections.Order = (*pageWithOrdinal)(nil)
	_ pageWrapper       = (*pageWithOrdinal)(nil)
	_ pageWrapper       = (*pageWithWeight0)(nil)
)

type pageWithOrdinal struct {
	ordinal int
	*pageState
}

func (p pageWithOrdinal) Ordinal() int {
	return p.ordinal
}

func (p pageWithOrdinal) page() page.Page {
	return p.pageState
}

type pageWithWeight0 struct {
	weight0 int
	*pageState
}

func (p pageWithWeight0) Weight0() int {
	return p.weight0
}

func (p pageWithWeight0) page() page.Page {
	return p.pageState
}
