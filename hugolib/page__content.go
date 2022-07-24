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
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/hugio"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/markup/converter"
	"github.com/gohugoio/hugo/parser/metadecoders"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/gohugoio/hugo/resources/resource"
	"github.com/gohugoio/hugo/tpl"
)

var (
	internalSummaryDividerBase      = "HUGOMORE42"
	internalSummaryDividerBaseBytes = []byte(internalSummaryDividerBase)
	internalSummaryDividerPre       = []byte("\n\n" + internalSummaryDividerBase + "\n\n")
)

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

func (p *pageContentMap) contentToRender(source []byte, renderedShortcodes map[string]string) []byte {
	c := make([]byte, 0, len(source)+(len(source)/10))

	for _, it := range p.items {
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

func newCachedContent(m *pageMeta) (*cachedContent, error) {
	var openSource resource.OpenReadSeekCloser
	if m.f != nil {
		openSource = func() (hugio.ReadSeekCloser, error) {
			return m.f.Open()
		}
	}
	c := &cachedContent{
		cache:          m.s.pageMap.cacheContent,
		shortcodeState: newShortcodeHandler(m.s),
		pageContentMap: &pageContentMap{},
		cacheBaseKey:   m.Path(),
		openSource:     openSource,
	}

	if err := c.parseHeader(); err != nil {
		return nil, err
	}

	return c, nil

}

type cachedContent struct {
	cache        memcache.Getter
	cacheBaseKey string

	// The source bytes.
	openSource resource.OpenReadSeekCloser

	shortcodeState *shortcodeHandler
	pageContentMap *pageContentMap
	items          pageparser.Items
	frontMatter    map[string]any

	enableEmoji bool // TODO1

	// Whether the parsed content contains a summary separator.
	hasSummaryDivider bool

	// Returns the position in bytes after any front matter.
	posMainContent int

	// These are set if we're able to determine this from the source.
	posSummaryEnd int
	posBodyStart  int

	summary struct {
		summary   template.HTML
		truncated bool
	}

	stats struct {
		wordCount      int
		fuzzyWordCount int
		readingTime    int
	}

	contentMapInit sync.Once
}

func (c *cachedContent) IsZero() bool {
	return len(c.items) == 0
}

func (c *cachedContent) parseHeader() error {
	if c.openSource == nil {
		return nil
	}

	// TODO1 store away the file/content size so we can parse everything right away if it's small enough (remember front matter in parseContent).

	source, err := c.sourceHead()
	if err != nil {
		return err
	}

	items, err := pageparser.ParseBytesIntroOnly(
		source,
		pageparser.Config{},
	)

	if err != nil || (len(items) > 0 && items[len(items)-1].IsError()) {
		// Probably too short buffer, fall back to parsing the comple file.

		source, err := c.getOrReadSource()
		if err != nil {
			return err
		}
		return c.initContentMap(source)
	}

	if err != nil {
		return err
	}

	return c.mapHeader(items, source)
}

func (c *cachedContent) initContentMap(source []byte) error {
	var err error

	c.contentMapInit.Do(func() {
		err = c.parseContent(source)
	})

	return err

}

func (c *cachedContent) parseContent(source []byte) error {
	if c.openSource == nil {
		return nil
	}

	items, err := pageparser.ParseBytes(
		source,
		pageparser.Config{EnableEmoji: c.enableEmoji},
	)

	if err != nil {
		return err
	}

	c.items = items

	return c.mapContent(source)

}

func (c *cachedContent) mapHeader(items pageparser.Items, source []byte) error {
	if items == nil {
		return nil
	}

	iter := pageparser.NewIterator(items)

Loop:
	for {
		it := iter.Next()

		switch {
		case it.Type == pageparser.TypeIgnore:
		case it.IsFrontMatter():
			if err := c.parseFrontMatter(it, iter, source); err != nil {
				return err
			}
			break Loop
		case it.IsEOF():
			break Loop
		case it.IsError():
			return it.Err

		}
	}

	return nil
}

func (c *cachedContent) parseFrontMatter(it pageparser.Item, iter *pageparser.Iterator, source []byte) error {
	if c.frontMatter != nil {
		return nil
	}

	f := pageparser.FormatFromFrontMatterType(it.Type)
	var err error
	c.frontMatter, err = metadecoders.Default.UnmarshalToMap(it.Val(source), f)
	if err != nil {
		if fe, ok := err.(herrors.FileError); ok {
			pos := fe.Position()
			// Apply the error to the content file.
			pos.Filename = "TODO1" // m.f.Filename()
			// Offset the starting position of front matter.
			offset := iter.LineNumber(source) - 1
			if f == metadecoders.YAML {
				offset -= 1
			}
			pos.LineNumber += offset

			fe.UpdatePosition(pos)

			return fe
		} else {
			return err
		}
	}

	return nil

}

func (c *cachedContent) mapContent(source []byte) error {
	if c.items == nil {
		return nil
	}

	s := c.shortcodeState
	rn := c.pageContentMap
	iter := pageparser.NewIterator(c.items)

	// the parser is guaranteed to return items in proper order or fail, so …
	// … it's safe to keep some "global" state
	var ordinal int

Loop:
	for {
		it := iter.Next()

		switch {
		case it.Type == pageparser.TypeIgnore:
		case it.IsFrontMatter():
			if err := c.parseFrontMatter(it, iter, source); err != nil {
				return err
			}
			next := iter.Peek()
			if !next.IsDone() {
				c.posMainContent = next.Pos()
			}
		case it.Type == pageparser.TypeLeadSummaryDivider:
			posBody := -1
			f := func(item pageparser.Item) bool {
				if posBody == -1 && !item.IsDone() {
					posBody = item.Pos()
				}

				if item.IsNonWhitespace(source) {
					c.summary.truncated = true

					// Done
					return false
				}
				return true
			}
			iter.PeekWalk(f)

			c.posSummaryEnd = it.Pos()
			c.posBodyStart = posBody
			c.hasSummaryDivider = true

			if true { // TODO1 if m.markup != "html" {
				// The content will be rendered by Goldmark or similar,
				// and we need to track the summary.
				rn.AddReplacement(internalSummaryDividerPre, it)
			}
		// Handle shortcode
		case it.IsLeftShortcodeDelim():
			// let extractShortcode handle left delim (will do so recursively)
			iter.Backup()

			currShortcode, err := s.extractShortcode(ordinal, 0, source, iter)
			if err != nil {
				return err
			}

			currShortcode.pos = it.Pos()
			currShortcode.length = iter.Current().Pos() - it.Pos()
			if currShortcode.placeholder == "" {
				currShortcode.placeholder = createShortcodePlaceholder("s", currShortcode.ordinal)
			}

			if currShortcode.name != "" {
				s.addName(currShortcode.name)
			}

			if currShortcode.params == nil {
				var s []string
				currShortcode.params = s
			}

			currShortcode.placeholder = createShortcodePlaceholder("s", ordinal)
			ordinal++
			s.shortcodes = append(s.shortcodes, currShortcode)

			rn.AddShortcode(currShortcode)

		case it.Type == pageparser.TypeEmoji:
			if emoji := helpers.Emoji(it.ValStr(source)); emoji != nil {
				rn.AddReplacement(emoji, it)
			} else {
				rn.AddBytes(it)
			}

		case it.IsEOF():
			break Loop
		case it.IsError():
			return it.Err
		default:
			rn.AddBytes(it)
		}
	}

	return nil
}

func (c *cachedContent) mustSource() []byte {
	source, err := c.getOrReadSource()
	if err != nil {
		panic(err)
	}
	return source
}

func (c *cachedContent) getOrReadSource() ([]byte, error) {
	key := c.cacheBaseKey + "/source"
	v, err := c.getOrCreate(key, func(ctx context.Context) (any, error) {
		return c.readSourceAll()
	})

	if err != nil {
		return nil, err
	}

	return v.([]byte), nil
}

func (c *cachedContent) readSourceAll() ([]byte, error) {
	if c.openSource == nil {
		return []byte{}, nil
	}
	r, err := c.openSource()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

func (c *cachedContent) sourceHead() ([]byte, error) {
	r, err := c.openSource()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	b := make([]byte, 512)

	i, err := io.ReadFull(r, b)
	if err != nil && err != io.ErrUnexpectedEOF {
		if err == io.EOF {
			// Empty source.
			return nil, nil
		}
		return nil, err
	}

	return b[:i], nil

}

func (c *cachedContent) getOrCreate(key string, fn func(ctx context.Context) (any, error)) (any, error) {
	ctx := context.TODO()
	v, err := c.cache.GetOrCreate(ctx, key, func() *memcache.Entry {
		b, err := fn(ctx)
		return &memcache.Entry{Value: b, Err: err, ClearWhen: memcache.ClearOnChange}
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

type contentTableOfContents struct {
	content         template.HTML
	tableOfContents template.HTML
}

type plainPlainWords struct {
	plain      string
	plainWords []string

	summary          template.HTML
	summaryTruncated bool

	wordCount      int
	fuzzyWordCount int
	readingTime    int
}

func (c *cachedContent) contentRendered(cp *pageContentOutput) (contentTableOfContents, error) {
	key := c.cacheBaseKey + "/content-rendered/" + cp.key

	v, err := c.getOrCreate(key, func(ctx context.Context) (any, error) {
		source, err := c.getOrReadSource()
		if err != nil {
			return "", err
		}

		if len(source) == 0 {
			return contentTableOfContents{}, nil
		}

		if err := c.initContentMap(source); err != nil {
			return "", err
		}

		if err := cp.initRenderHooks(); err != nil {
			return "", err
		}

		var (
			hasShortcodeVariants bool
			result               contentTableOfContents
		)

		f := cp.po.f
		contentPlaceholders, hasShortcodeVariants, err := c.shortcodeState.renderShortcodesForPage(cp.po.ps, f)
		if err != nil {
			return "", err
		}

		if hasShortcodeVariants {
			// TODO1 question?
			cp.po.ps.pageOutputTemplateVariationsState.Store(2)
		}

		contentToRender := c.pageContentMap.contentToRender(source, contentPlaceholders)

		isHTML := cp.po.ps.m.markup == "html"

		var workContent []byte
		var placeholdersEnabled bool // TODO1

		if !isHTML {
			r, err := cp.renderContent(contentToRender, true)
			if err != nil {
				return "", err
			}
			cp.po.ps.s.h.IncrContentRender()

			workContent = r.Bytes()

			if tocProvider, ok := r.(converter.TableOfContentsProvider); ok {
				cfg := cp.po.ps.s.ContentSpec.Converters.GetMarkupConfig()
				result.tableOfContents = template.HTML(
					tocProvider.TableOfContents().ToHTML(
						cfg.TableOfContents.StartLevel,
						cfg.TableOfContents.EndLevel,
						cfg.TableOfContents.Ordered,
					),
				)
			} else {
				tmpContent, tmpTableOfContents := helpers.ExtractTOC(workContent)
				result.tableOfContents = helpers.BytesToHTML(tmpTableOfContents)
				workContent = tmpContent
			}

			if placeholdersEnabled {
				// ToC was accessed via .Page.TableOfContents in the shortcode,
				// at a time when the ToC wasn't ready.
				contentPlaceholders[tocShortcodePlaceholder] = string(result.tableOfContents)
			}
		}

		if c.pageContentMap.hasNonMarkdownShortcode || placeholdersEnabled {
			workContent, err = replaceShortcodeTokens(workContent, contentPlaceholders)
			if err != nil {
				return "", err
			}
		}

		result.content = helpers.BytesToHTML(workContent)

		return result, nil
	})

	if err != nil {

		return contentTableOfContents{}, cp.po.ps.wrapError(err)
	}

	return v.(contentTableOfContents), nil
}

func (c *cachedContent) contentPlain(cp *pageContentOutput) (plainPlainWords, error) {
	key := c.cacheBaseKey + "/content-plain" + cp.key

	v, err := c.getOrCreate(key, func(ctx context.Context) (any, error) {
		var result plainPlainWords

		rendered, err := c.contentRendered(cp)
		if err != nil {
			return result, err
		}

		result.plain = tpl.StripHTML(string(rendered.content))
		result.plainWords = strings.Fields(result.plain)

		isCJKLanguage := cp.po.ps.m.isCJKLanguage

		if isCJKLanguage {
			result.wordCount = 0
			for _, word := range result.plainWords {
				runeCount := utf8.RuneCountInString(word)
				if len(word) == runeCount {
					result.wordCount++
				} else {
					result.wordCount += runeCount
				}
			}
		} else {
			result.wordCount = helpers.TotalWords(result.plain)
		}

		// TODO(bep) is set in a test. Fix that.
		if result.fuzzyWordCount == 0 {
			result.fuzzyWordCount = (result.wordCount + 100) / 100 * 100
		}

		if isCJKLanguage {
			result.readingTime = (result.wordCount + 500) / 501
		} else {
			result.readingTime = (result.wordCount + 212) / 213
		}

		// TODO1 hasSummaryDivider

		summary := cp.po.ps.m.summary
		var truncated bool

		if summary == "" {
			if isCJKLanguage {
				summary, truncated = cp.po.ps.s.ContentSpec.TruncateWordsByRune(result.plainWords)
			} else {
				summary, truncated = cp.po.ps.s.ContentSpec.TruncateWordsToWholeSentence(result.plain)
			}
		}
		result.summary = template.HTML(summary)
		result.summaryTruncated = truncated

		return result, nil
	})

	return v.(plainPlainWords), err
}

func (c *cachedContent) foo() {

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
