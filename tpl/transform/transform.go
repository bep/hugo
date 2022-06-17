// Copyright 2021 The Hugo Authors. All rights reserved.
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

// Package transform provides template functions for transforming content.
package transform

import (
	"html"
	"html/template"

	"github.com/gohugoio/hugo/cache/memcache"

	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/helpers"
	"github.com/spf13/cast"
)

// New returns a new instance of the transform-namespaced template functions.
func New(deps *deps.Deps) *Namespace {
	if deps.MemCache == nil {
		panic("must provide MemCache")
	}
	return &Namespace{
		deps:  deps,
		cache: deps.MemCache.GetOrCreatePartition("tpl/transform", memcache.ClearOnChange),
	}
}

// Namespace provides template functions for the "transform" namespace.
type Namespace struct {
	deps  *deps.Deps
	cache memcache.Getter
}

// Emojify returns a copy of s with all emoji codes replaced with actual emojis.
//
// See http://www.emoji-cheat-sheet.com/
func (ns *Namespace) Emojify(s any) (template.HTML, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	return template.HTML(helpers.Emojify([]byte(ss))), nil
}

// Highlight returns a copy of s as an HTML string with syntax
// highlighting applied.
func (ns *Namespace) Highlight(s any, lang string, opts ...any) (template.HTML, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	sopts := ""
	if len(opts) > 0 {
		sopts, err = cast.ToStringE(opts[0])
		if err != nil {
			return "", err
		}
	}

	highlighted, _ := ns.deps.ContentSpec.Converters.Highlight(ss, lang, sopts)
	return template.HTML(highlighted), nil
}

// HTMLEscape returns a copy of s with reserved HTML characters escaped.
func (ns *Namespace) HTMLEscape(s any) (string, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	return html.EscapeString(ss), nil
}

// HTMLUnescape returns a copy of with HTML escape requences converted to plain
// text.
func (ns *Namespace) HTMLUnescape(s any) (string, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	return html.UnescapeString(ss), nil
}

// Markdownify renders a given input from Markdown to HTML.
func (ns *Namespace) Markdownify(s any) (template.HTML, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	b, err := ns.deps.ContentSpec.RenderMarkdown([]byte(ss))
	if err != nil {
		return "", err
	}

	// Strip if this is a short inline type of text.
	b = ns.deps.ContentSpec.TrimShortHTML(b)

	return helpers.BytesToHTML(b), nil
}

// Plainify returns a copy of s with all HTML tags removed.
func (ns *Namespace) Plainify(s any) (string, error) {
	ss, err := cast.ToStringE(s)
	if err != nil {
		return "", err
	}

	return helpers.StripHTML(ss), nil
}
