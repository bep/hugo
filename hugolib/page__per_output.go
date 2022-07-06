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
	"html/template"
	"strings"
	"sync"

	"errors"

	"github.com/gohugoio/hugo/common/text"
	"github.com/gohugoio/hugo/common/types/hstring"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cast"

	"github.com/gohugoio/hugo/markup/converter/hooks"

	"github.com/gohugoio/hugo/markup/converter"

	"github.com/alecthomas/chroma/v2/lexers"

	bp "github.com/gohugoio/hugo/bufferpool"
	"github.com/gohugoio/hugo/tpl"

	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
)

var (
	nopTargetPath    = targetPathsHolder{}
	nopPagePerOutput = struct {
		resource.ResourceLinksProvider
		page.ContentProvider
		page.PageRenderProvider
		page.PaginatorProvider
		page.TableOfContentsProvider
		page.AlternativeOutputFormatsProvider

		targetPather
	}{
		page.NopPage,
		page.NopPage,
		page.NopPage,
		page.NopPage,
		page.NopPage,
		page.NopPage,
		nopTargetPath,
	}
)

func newPageContentOutput(po *pageOutput) (*pageContentOutput, error) {
	cp := &pageContentOutput{
		key:         po.f.Name,
		po:          po,
		renderHooks: &renderHooks{},
	}
	return cp, nil
}

type renderHooks struct {
	getRenderer hooks.GetRendererFunc
	init        sync.Once
}

// pageContentOutput represents the Page content for a given output format.
type pageContentOutput struct {
	po  *pageOutput // TODO1 make this a ps
	key string

	placeholdersEnabled     bool
	placeholdersEnabledInit sync.Once

	// Renders Markdown hooks.
	renderHooks *renderHooks
}

func (p *pageContentOutput) trackDependency(id identity.Identity) {
	p.po.dependencyManagerOutput.AddIdentity(id)
}

func (p *pageContentOutput) Reset() {
	p.po.dependencyManagerOutput.Reset()
	p.renderHooks = &renderHooks{}
}

func (p *pageContentOutput) Content() (any, error) {
	r, err := p.po.ps.content.contentRendered(p)
	return r.content, err
}

func (p *pageContentOutput) TableOfContents() template.HTML {
	r, err := p.po.ps.content.contentRendered(p)
	if err != nil {
		panic(err)
	}
	return r.tableOfContents
}

func (p *pageContentOutput) Len() int {
	return len(p.mustContentRendered().content)
}

func (p *pageContentOutput) mustContentRendered() contentTableOfContents {
	r, err := p.po.ps.content.contentRendered(p)
	if err != nil {
		panic(err)
	}
	return r
}

func (p *pageContentOutput) mustContentPlain() plainPlainWords {
	r, err := p.po.ps.content.contentPlain(p)
	if err != nil {
		panic(err)
	}
	return r
}

func (p *pageContentOutput) Plain() string {
	return p.mustContentPlain().plain
}

func (p *pageContentOutput) PlainWords() []string {
	return p.mustContentPlain().plainWords
}

func (p *pageContentOutput) ReadingTime() int {
	return p.mustContentPlain().readingTime
}

func (p *pageContentOutput) WordCount() int {
	return p.mustContentPlain().wordCount
}

func (p *pageContentOutput) FuzzyWordCount() int {
	return p.mustContentPlain().fuzzyWordCount
}

func (p *pageContentOutput) Summary() template.HTML {
	return p.mustContentPlain().summary
}

func (p *pageContentOutput) Truncated() bool {
	return p.mustContentPlain().summaryTruncated
}

func (p *pageContentOutput) RenderString(args ...any) (template.HTML, error) {
	if len(args) < 1 || len(args) > 2 {
		return "", errors.New("want 1 or 2 arguments")
	}

	var contentToRender string
	opts := defaultRenderStringOpts
	sidx := 1

	if len(args) == 1 {
		sidx = 0
	} else {
		m, ok := args[0].(map[string]any)
		if !ok {
			return "", errors.New("first argument must be a map")
		}

		if err := mapstructure.WeakDecode(m, &opts); err != nil {
			return "", fmt.Errorf("failed to decode options: %w", err)
		}
	}

	contentToRenderv := args[sidx]

	if _, ok := contentToRenderv.(hstring.RenderedString); ok {
		// This content is already rendered, this is potentially
		// a infinite recursion.
		return "", errors.New("text is already rendered, repeating it may cause infinite recursion")
	}

	var err error
	contentToRender, err = cast.ToStringE(contentToRenderv)
	if err != nil {
		return "", err
	}

	if err = p.initRenderHooks(); err != nil {
		return "", err
	}

	conv := p.po.ps.getContentConverter()
	if opts.Markup != "" && opts.Markup != p.po.ps.m.markup {
		var err error
		// TODO(bep) consider cache
		conv, err = p.po.ps.m.newContentConverter(p.po.ps, opts.Markup)
		if err != nil {
			return "", p.po.ps.wrapError(err)
		}
	}

	var rendered []byte

	if strings.Contains(contentToRender, "{{") {
		// Probably a shortcode.
		parsed, err := pageparser.ParseMain(strings.NewReader(contentToRender), pageparser.Config{})
		if err != nil {
			return "", err
		}
		pm := &pageContentMap{
			items: make([]any, 0, 20),
		}
		s := newShortcodeHandler(p.po.ps.s)

		if _, err := p.po.ps.mapContentForResult(
			parsed,
			s,
			pm,
			opts.Markup,
			nil,
		); err != nil {
			return "", err
		}

		placeholders, hasShortcodeVariants, err := s.renderShortcodesForPage(p.po.ps, p.po.f)
		if err != nil {
			return "", err
		}

		if hasShortcodeVariants {
			p.po.ps.pageOutputTemplateVariationsState.Store(2)
		}

		b, err := p.renderContentWithConverter(conv, pm.contentToRender(parsed.Input(), placeholders), false)
		if err != nil {
			return "", p.po.ps.wrapError(err)
		}
		rendered = b.Bytes()

		if p.placeholdersEnabled {
			// ToC was accessed via .Page.TableOfContents in the shortcode,
			// at a time when the ToC wasn't ready.
			placeholders[tocShortcodePlaceholder] = string(p.po.ps.TableOfContents())
		}

		if pm.hasNonMarkdownShortcode || p.placeholdersEnabled {
			rendered, err = replaceShortcodeTokens(rendered, placeholders)
			if err != nil {
				return "", err
			}
		}

		// We need a consolidated view in $page.HasShortcode
		p.po.ps.content.shortcodeState.transferNames(s)

	} else {
		c, err := p.renderContentWithConverter(conv, []byte(contentToRender), false)
		if err != nil {
			return "", p.po.ps.wrapError(err)
		}

		rendered = c.Bytes()
	}

	if opts.Display == "inline" {
		// We may have to rethink this in the future when we get other
		// renderers.
		rendered = p.po.ps.s.ContentSpec.TrimShortHTML(rendered)
	}

	return template.HTML(string(rendered)), nil
}

func (p *pageContentOutput) Render(ctx context.Context, layout ...string) (template.HTML, error) {
	templ, found, err := p.po.ps.resolveTemplate(layout...)
	if err != nil {
		return "", p.po.ps.wrapError(err)
	}

	if !found {
		return "", nil
	}

	p.po.ps.addDependency(templ.(tpl.Info))

	// Make sure to send the *pageState and not the *pageContentOutput to the template.
	res, err := executeToString(ctx, p.po.ps.s.Tmpl(), templ, p.po.ps)
	if err != nil {
		return "", p.po.ps.wrapError(fmt.Errorf("failed to execute template %s: %w", templ.Name(), err))
	}
	return template.HTML(res), nil
}

func (p *pageContentOutput) initRenderHooks() error {
	if p == nil {
		return nil
	}

	p.renderHooks.init.Do(func() {
		if p.po.ps.pageOutputTemplateVariationsState.Load() == 0 {
			p.po.ps.pageOutputTemplateVariationsState.Store(1)
		}

		type cacheKey struct {
			tp hooks.RendererType
			id any
			f  output.Format
		}

		renderCache := make(map[cacheKey]any)
		var renderCacheMu sync.Mutex

		resolvePosition := func(ctx any) text.Position {
			var offset int

			switch v := ctx.(type) {
			case hooks.CodeblockContext:
				offset = bytes.Index(p.po.ps.content.mustSource(), []byte(v.Inner()))
			}

			pos := p.po.ps.posFromInput(p.po.ps.content.mustSource(), offset)

			if pos.LineNumber > 0 {
				// Move up to the code fence delimiter.
				// This is in line with how we report on shortcodes.
				pos.LineNumber = pos.LineNumber - 1
			}

			return pos
		}

		p.renderHooks.getRenderer = func(tp hooks.RendererType, id any) any {
			renderCacheMu.Lock()
			defer renderCacheMu.Unlock()

			key := cacheKey{tp: tp, id: id, f: p.po.f}
			if r, ok := renderCache[key]; ok {
				return r
			}

			layoutDescriptor := p.po.ps.getLayoutDescriptor()
			layoutDescriptor.RenderingHook = true
			layoutDescriptor.LayoutOverride = false
			layoutDescriptor.Layout = ""

			switch tp {
			case hooks.LinkRendererType:
				layoutDescriptor.Kind = "render-link"
			case hooks.ImageRendererType:
				layoutDescriptor.Kind = "render-image"
			case hooks.HeadingRendererType:
				layoutDescriptor.Kind = "render-heading"
			case hooks.CodeBlockRendererType:
				layoutDescriptor.Kind = "render-codeblock"
				if id != nil {
					lang := id.(string)
					lexer := lexers.Get(lang)
					if lexer != nil {
						layoutDescriptor.KindVariants = strings.Join(lexer.Config().Aliases, ",")
					} else {
						layoutDescriptor.KindVariants = lang
					}
				}
			}

			getHookTemplate := func(f output.Format) (tpl.Template, bool) {
				templ, found, err := p.po.ps.s.Tmpl().LookupLayout(layoutDescriptor, f)
				if err != nil {
					panic(err)
				}
				return templ, found
			}

			templ, found1 := getHookTemplate(p.po.f)

			if p.po.ps.reusePageOutputContent() {
				// Check if some of the other output formats would give a different template.
				for _, f := range p.po.ps.s.renderFormats {
					if f.Name == p.po.f.Name {
						continue
					}
					templ2, found2 := getHookTemplate(f)
					if found2 {
						if !found1 {
							templ = templ2
							found1 = true
							break
						}

						if templ != templ2 {
							p.po.ps.pageOutputTemplateVariationsState.Store(2)
							break
						}
					}
				}
			}
			if !found1 {
				if tp == hooks.CodeBlockRendererType {
					// No user provided tempplate for code blocks, so we use the native Go code version -- which is also faster.
					r := p.po.ps.s.ContentSpec.Converters.GetHighlighter()
					renderCache[key] = r
					return r
				}
				return nil
			}

			r := hookRendererTemplate{
				templateHandler: p.po.ps.s.Tmpl(),
				templ:           templ,
				resolvePosition: resolvePosition,
			}
			renderCache[key] = r
			return r
		}
	})

	return nil
}

func (p *pageContentOutput) setAutoSummary() error {
	// TODO1
	return nil
	/*
		if p.po.ps.source.hasSummaryDivider || p.po.ps.m.summary != "" {
			return nil
		}

		var summary string
		var truncated bool

		if p.po.ps.m.isCJKLanguage {
			summary, truncated = p.po.ps.s.ContentSpec.TruncateWordsByRune(p.plainWords)
		} else {
			summary, truncated = p.po.ps.s.ContentSpec.TruncateWordsToWholeSentence(p.plain)
		}
		p.summary = template.HTML(summary)

		p.truncated = truncated

		return nil
	*/
}

func (cp *pageContentOutput) renderContent(content []byte, renderTOC bool) (converter.Result, error) {
	if err := cp.initRenderHooks(); err != nil {
		return nil, err
	}
	c := cp.po.ps.getContentConverter()
	return cp.renderContentWithConverter(c, content, renderTOC)
}

func (cp *pageContentOutput) renderContentWithConverter(c converter.Converter, content []byte, renderTOC bool) (converter.Result, error) {
	r, err := c.Convert(
		converter.RenderContext{
			Src:                       content,
			RenderTOC:                 renderTOC,
			DependencyManagerProvider: cp.po,
			GetRenderer:               cp.renderHooks.getRenderer,
		})

	if err == nil {
		// TODO1 check if we can remove IdentitiesProvider
		if ids, ok := r.(identity.IdentitiesProvider); ok {
			for id := range ids.GetIdentities() {
				cp.trackDependency(id)
			}
		}
	}

	return r, err
}

// A callback to signal that we have inserted a placeholder into the rendered
// content. This avoids doing extra replacement work.
func (p *pageContentOutput) enablePlaceholders() {
	p.placeholdersEnabledInit.Do(func() {
		p.placeholdersEnabled = true
	})
}

// these will be shifted out when rendering a given output format.
type pagePerOutputProviders interface {
	targetPather
	page.PaginatorProvider
	resource.ResourceLinksProvider
}

type targetPather interface {
	targetPaths() page.TargetPaths
}

type targetPathsHolder struct {
	paths page.TargetPaths
	page.OutputFormat
}

func (t targetPathsHolder) targetPaths() page.TargetPaths {
	return t.paths
}

func executeToString(ctx context.Context, h tpl.TemplateHandler, templ tpl.Template, data any) (string, error) {
	b := bp.GetBuffer()
	defer bp.PutBuffer(b)
	if err := h.ExecuteWithContext(ctx, templ, b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func splitUserDefinedSummaryAndContent(markup string, c []byte) (summary []byte, content []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("summary split failed: %s", r)
		}
	}()

	startDivider := bytes.Index(c, internalSummaryDividerBaseBytes)

	if startDivider == -1 {
		return
	}

	startTag := "p"
	switch markup {
	case "asciidocext":
		startTag = "div"
	}

	// Walk back and forward to the surrounding tags.
	start := bytes.LastIndex(c[:startDivider], []byte("<"+startTag))
	end := bytes.Index(c[startDivider:], []byte("</"+startTag))

	if start == -1 {
		start = startDivider
	} else {
		start = startDivider - (startDivider - start)
	}

	if end == -1 {
		end = startDivider + len(internalSummaryDividerBase)
	} else {
		end = startDivider + end + len(startTag) + 3
	}

	var addDiv bool

	switch markup {
	case "rst":
		addDiv = true
	}

	withoutDivider := append(c[:start], bytes.Trim(c[end:], "\n")...)

	if len(withoutDivider) > 0 {
		summary = bytes.TrimSpace(withoutDivider[:start])
	}

	if addDiv {
		// For the rst
		summary = append(append([]byte(nil), summary...), []byte("</div>")...)
	}

	if err != nil {
		return
	}

	content = bytes.TrimSpace(withoutDivider)

	return
}
