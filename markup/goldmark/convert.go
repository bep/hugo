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

// Package goldmark converts Markdown to HTML using Goldmark.
package goldmark

import (
	"bytes"

	"github.com/gohugoio/hugo/markup/goldmark/codeblocks"
	"github.com/gohugoio/hugo/markup/goldmark/internal/extensions/attributes"
	"github.com/gohugoio/hugo/markup/goldmark/internal/render"

	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/markup/converter"
	"github.com/gohugoio/hugo/markup/tableofcontents"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Provider is the package entry point.
var Provider converter.ProviderProvider = provide{}

type provide struct{}

func (p provide) New(cfg converter.ProviderConfig) (converter.Provider, error) {
	md := newMarkdown(cfg)

	return converter.NewProvider("goldmark", func(ctx converter.DocumentContext) (converter.Converter, error) {
		return &goldmarkConverter{
			ctx: ctx,
			cfg: cfg,
			md:  md,
			sanitizeAnchorName: func(s string) string {
				return sanitizeAnchorNameString(s, cfg.MarkupConfig.Goldmark.Parser.AutoHeadingIDType)
			},
		}, nil
	}), nil
}

var _ converter.AnchorNameSanitizer = (*goldmarkConverter)(nil)

type goldmarkConverter struct {
	md  goldmark.Markdown
	ctx converter.DocumentContext
	cfg converter.ProviderConfig

	sanitizeAnchorName func(s string) string
}

func (c *goldmarkConverter) SanitizeAnchorName(s string) string {
	return c.sanitizeAnchorName(s)
}

func newMarkdown(pcfg converter.ProviderConfig) goldmark.Markdown {
	mcfg := pcfg.MarkupConfig
	cfg := pcfg.MarkupConfig.Goldmark
	var rendererOptions []renderer.Option

	if cfg.Renderer.HardWraps {
		rendererOptions = append(rendererOptions, html.WithHardWraps())
	}

	if cfg.Renderer.XHTML {
		rendererOptions = append(rendererOptions, html.WithXHTML())
	}

	if cfg.Renderer.Unsafe {
		rendererOptions = append(rendererOptions, html.WithUnsafe())
	}

	var (
		extensions = []goldmark.Extender{
			newLinks(cfg),
			newTocExtension(rendererOptions),
		}
		parserOptions []parser.Option
	)

	if mcfg.Highlight.CodeFences {
		extensions = append(extensions, codeblocks.New())
	}

	if cfg.Extensions.Table {
		extensions = append(extensions, extension.Table)
	}

	if cfg.Extensions.Strikethrough {
		extensions = append(extensions, extension.Strikethrough)
	}

	if cfg.Extensions.Linkify {
		extensions = append(extensions, extension.Linkify)
	}

	if cfg.Extensions.TaskList {
		extensions = append(extensions, extension.TaskList)
	}

	if cfg.Extensions.Typographer {
		extensions = append(extensions, extension.Typographer)
	}

	if cfg.Extensions.DefinitionList {
		extensions = append(extensions, extension.DefinitionList)
	}

	if cfg.Extensions.Footnote {
		extensions = append(extensions, extension.Footnote)
	}

	if cfg.Parser.AutoHeadingID {
		parserOptions = append(parserOptions, parser.WithAutoHeadingID())
	}

	if cfg.Parser.Attribute.Title {
		parserOptions = append(parserOptions, parser.WithAttribute())
	}

	if cfg.Parser.Attribute.Block {
		extensions = append(extensions, attributes.New())
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			extensions...,
		),
		goldmark.WithParserOptions(
			parserOptions...,
		),
		goldmark.WithRendererOptions(
			rendererOptions...,
		),
	)

	return md
}

type converterResult struct {
	converter.Result
	toc tableofcontents.Root
}

func (c converterResult) TableOfContents() tableofcontents.Root {
	return c.toc
}

<<<<<<< HEAD
func (c converterResult) GetIdentities() identity.Identities {
	return c.ids
}

var converterIdentity = identity.KeyValueIdentity{Key: "goldmark", Value: "converter"}
=======
type bufWriter struct {
	*bytes.Buffer
}

const maxInt = 1<<(bits.UintSize-1) - 1

func (b *bufWriter) Available() int {
	return maxInt
}

func (b *bufWriter) Buffered() int {
	return b.Len()
}

func (b *bufWriter) Flush() error {
	return nil
}

type renderContext struct {
	*bufWriter
	pos int
	renderContextData
}

type renderContextData interface {
	RenderContext() converter.RenderContext
	DocumentContext() converter.DocumentContext
}

type renderContextDataHolder struct {
	rctx converter.RenderContext
	dctx converter.DocumentContext
}

func (ctx *renderContextDataHolder) RenderContext() converter.RenderContext {
	return ctx.rctx
}

func (ctx *renderContextDataHolder) DocumentContext() converter.DocumentContext {
	return ctx.dctx
}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

func (c *goldmarkConverter) Convert(ctx converter.RenderContext) (result converter.Result, err error) {

	buf := &render.BufWriter{Buffer: &bytes.Buffer{}}
	result = buf
	pctx := c.newParserContext(ctx)
	reader := text.NewReader(ctx.Src)

	doc := c.md.Parser().Parse(
		reader,
		parser.WithContext(pctx),
	)

<<<<<<< HEAD
	rcx := &render.RenderContextDataHolder{
		Rctx: ctx,
		Dctx: c.ctx,
		IDs:  identity.NewManager(converterIdentity),
=======
	rcx := &renderContextDataHolder{
		rctx: ctx,
		dctx: c.ctx,
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	}

	w := &render.Context{
		BufWriter:   buf,
		ContextData: rcx,
	}

	if err := c.md.Renderer().Render(w, ctx.Src, doc); err != nil {
		return nil, err
	}

	return converterResult{
		Result: buf,
<<<<<<< HEAD
		ids:    rcx.IDs.GetIdentities(),
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		toc:    pctx.TableOfContents(),
	}, nil
}

var featureSet = map[identity.Identity]bool{
	converter.FeatureRenderHookHeading: true,
	converter.FeatureRenderHookImage:   true,
	converter.FeatureRenderHookLink:    true,
}

func (c *goldmarkConverter) Supports(feature identity.Identity) bool {
	return featureSet[feature]
}

func (c *goldmarkConverter) newParserContext(rctx converter.RenderContext) *parserContext {
	ctx := parser.NewContext(parser.WithIDs(newIDFactory(c.cfg.MarkupConfig.Goldmark.Parser.AutoHeadingIDType)))
	ctx.Set(tocEnableKey, rctx.RenderTOC)
	return &parserContext{
		Context: ctx,
	}
}

type parserContext struct {
	parser.Context
}

func (p *parserContext) TableOfContents() tableofcontents.Root {
	if v := p.Get(tocResultKey); v != nil {
		return v.(tableofcontents.Root)
	}
	return tableofcontents.Root{}
}
