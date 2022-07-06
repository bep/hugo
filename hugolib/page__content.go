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
	"html/template"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/gohugoio/hugo/resources/page"
)

var (
	internalSummaryDividerBase      = "HUGOMORE42"
	internalSummaryDividerBaseBytes = []byte(internalSummaryDividerBase)
	internalSummaryDividerPre       = []byte("\n\n" + internalSummaryDividerBase + "\n\n")
)

var zeroContent = pageContent{}

// The content related items on a Page.
type pageContent struct {
	selfLayout string
	truncated  bool

	cmap *pageContentMap

	source rawPageContent
}

// returns the content to be processed by Goldmark or similar.
func (p pageContent) contentToRender(parsed pageparser.Result, pm *pageContentMap, renderedShortcodes map[string]string) []byte {
	source := parsed.Input()

	c := make([]byte, 0, len(source)+(len(source)/10))

	for _, it := range pm.items {
		switch v := it.(type) {
		case pageparser.Item:
			c = append(c, source[v.Pos():v.Pos()+len(v.Val(source))]...)
		case pageContentReplacement:
			c = append(c, v.val...)
		case *shortcode:
			if !v.insertPlaceholder() {
				// Insert the rendered shortcode.
				renderedShortcode, found := renderedShortcodes[v.placeholder]
				if !found {
					// This should never happen.
					panic(fmt.Sprintf("rendered shortcode %q not found", v.placeholder))
				}

				c = append(c, []byte(renderedShortcode)...)

			} else {
				// Insert the placeholder so we can insert the content after
				// markdown processing.
				c = append(c, []byte(v.placeholder)...)
			}
		default:
			panic(fmt.Sprintf("unknown item type %T", it))
		}
	}

	return c
}

func (p pageContent) selfLayoutForOutput(f output.Format) string {
	if p.selfLayout == "" {
		return ""
	}
	return p.selfLayout + f.Name
}

type rawPageContent struct {
	hasSummaryDivider bool

	// The AST of the parsed page. Contains information about:
	// shortcodes, front matter, summary indicators.
	parsed pageparser.Result

	// Returns the position in bytes after any front matter.
	posMainContent int

	// These are set if we're able to determine this from the source.
	posSummaryEnd int
	posBodyStart  int
}

type pageContentReplacement struct {
	val []byte

	source pageparser.Item
}

type pageContentMap struct {

	// If not, we can skip any pre-rendering of shortcodes.
	hasMarkdownShortcode bool

	// Indicates whether we must do placeholder replacements.
	hasNonMarkdownShortcode bool

	//  *shortcode, pageContentReplacement or pageparser.Item
	items []any
}

func (p *pageContentMap) AddBytes(item pageparser.Item) {
	p.items = append(p.items, item)
}

func (p *pageContentMap) AddReplacement(val []byte, source pageparser.Item) {
	p.items = append(p.items, pageContentReplacement{val: val, source: source})
}

func (p *pageContentMap) AddShortcode(s *shortcode) {
	p.items = append(p.items, s)
	if s.insertPlaceholder() {
		p.hasNonMarkdownShortcode = true
	} else {
		p.hasMarkdownShortcode = true
	}
}

var _ page.ContentProvider = &cachedContentProvider{}

type cachedContentProvider struct {
	cache        memcache.Getter
	cacheBaseKey string

	summary struct {
		summary   template.HTML
		truncated bool
	}

	stats struct {
		wordCount      int
		fuzzyWordCount int
		readingTime    int
	}
}

func (c *cachedContentProvider) foo() {

	/*

			initContent := func() (err error) {
			if po.ps.cmap == nil {
				// Nothing to do.
				return nil
			}
			defer func() {
				// See https://github.com/gohugoio/hugo/issues/6210
				if r := recover(); r != nil {
					err = fmt.Errorf("%s", r)
					p.s.Log.Errorf("[BUG] Got panic:\n%s\n%s", r, string(debug.Stack()))
				}
			}()

			if err := po.cp.initRenderHooks(); err != nil {
				return err
			}

			var hasShortcodeVariants bool

			f := po.f
			cp.contentPlaceholders, hasShortcodeVariants, err = p.shortcodeState.renderShortcodesForPage(p, f)
			if err != nil {
				return err
			}

			if hasShortcodeVariants {
				p.pageOutputTemplateVariationsState.Store(2)
			}

			cp.workContent = p.contentToRender(p.source.parsed, p.cmap, cp.contentPlaceholders)

			isHTML := p.m.markup == "html"

			if !isHTML {
				r, err := cp.renderContent(cp.workContent, true)
				if err != nil {
					return err
				}

				cp.workContent = r.Bytes()

				if tocProvider, ok := r.(converter.TableOfContentsProvider); ok {
					cfg := p.s.ContentSpec.Converters.GetMarkupConfig()
					cp.tableOfContents = template.HTML(
						tocProvider.TableOfContents().ToHTML(
							cfg.TableOfContents.StartLevel,
							cfg.TableOfContents.EndLevel,
							cfg.TableOfContents.Ordered,
						),
					)
				} else {
					tmpContent, tmpTableOfContents := helpers.ExtractTOC(cp.workContent)
					cp.tableOfContents = helpers.BytesToHTML(tmpTableOfContents)
					cp.workContent = tmpContent
				}
			}

			if cp.placeholdersEnabled {
				// ToC was accessed via .Page.TableOfContents in the shortcode,
				// at a time when the ToC wasn't ready.
				cp.contentPlaceholders[tocShortcodePlaceholder] = string(cp.tableOfContents)
			}

			if p.cmap.hasNonMarkdownShortcode || cp.placeholdersEnabled {
				// There are one or more replacement tokens to be replaced.
				cp.workContent, err = replaceShortcodeTokens(cp.workContent, cp.contentPlaceholders)
				if err != nil {
					return err
				}
			}

			if p.source.hasSummaryDivider {
				if isHTML {
					src := p.source.parsed.Input()

					// Use the summary sections as they are provided by the user.
					if p.source.posSummaryEnd != -1 {
						cp.summary = helpers.BytesToHTML(src[p.source.posMainContent:p.source.posSummaryEnd])
					}

					if p.source.posBodyStart != -1 {
						cp.workContent = src[p.source.posBodyStart:]
					}

				} else {
					summary, content, err := splitUserDefinedSummaryAndContent(p.m.markup, cp.workContent)
					if err != nil {
						p.s.Log.Errorf("Failed to set user defined summary for page %q: %s", p.pathOrTitle(), err)
					} else {
						cp.workContent = content
						cp.summary = helpers.BytesToHTML(summary)
					}
				}
			} else if p.m.summary != "" {
				b, err := cp.renderContent([]byte(p.m.summary), false)
				if err != nil {
					return err
				}
				html := p.s.ContentSpec.TrimShortHTML(b.Bytes())
				cp.summary = helpers.BytesToHTML(html)
			}

			cp.content = helpers.BytesToHTML(cp.workContent)

			return nil
		}

	*/

}

func (c *cachedContentProvider) Content() (any, error) {
	panic("not implemented") // TODO: Implement
}

// Plain returns the Page Content stripped of HTML markup.
func (c *cachedContentProvider) Plain() string {
	panic("not implemented") // TODO: Implement
}

// PlainWords returns a string slice from splitting Plain using https://pkg.go.dev/strings#Fields.
func (c *cachedContentProvider) PlainWords() []string {
	panic("not implemented") // TODO: Implement
}

// Summary returns a generated summary of the content.
// The breakpoint can be set manually by inserting a summary separator in the source file.
func (c *cachedContentProvider) Summary() template.HTML {
	return c.summary.summary
}

// Truncated returns whether the Summary  is truncated or not.
func (c *cachedContentProvider) Truncated() bool {
	return c.summary.truncated
}

// FuzzyWordCount returns the approximate number of words in the content.
func (c *cachedContentProvider) FuzzyWordCount() int {
	return c.stats.fuzzyWordCount
}

// WordCount returns the number of words in the content.
func (c *cachedContentProvider) WordCount() int {
	return c.stats.wordCount
}

// ReadingTime returns the reading time based on the length of plain text.
func (c *cachedContentProvider) ReadingTime() int {
	return c.stats.readingTime
}

// Len returns the length of the content.
func (c *cachedContentProvider) Len() int {
	panic("not implemented") // TODO: Implement
}
