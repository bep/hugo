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

package paths

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/gohugoio/hugo/identity"
)

var ForComponent = func(component string) func(b *Path) {
	return func(b *Path) {
		b.component = component
	}
}

// Parse parses s into Path using Hugo's content path rules.
func Parse(s string, parseOpts ...func(b *Path)) *Path {
	p, err := parse(s, parseOpts...)
	if err != nil {
		panic(err)
	}
	return p
}

func parse(s string, parseOpts ...func(b *Path)) (*Path, error) {
	p := &Path{
		component:        files.ComponentFolderContent,
		posContainerLow:  -1,
		posContainerHigh: -1,
		posSectionHigh:   -1,
	}

	for _, opt := range parseOpts {
		opt(p)
	}

	// All lower case.
	s = strings.ToLower(s)

	// Leading slash, no trailing slash.
	if p.component != files.ComponentFolderLayouts && !strings.HasPrefix(s, "/") {
		s = "/" + s
	}

	if s != "/" && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}

	p.s = s

	isWindows := runtime.GOOS == "windows"

	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]

		if isWindows && c == os.PathSeparator {
			return nil, errors.New("only forward slashes allowed")
		}

		switch c {
		case '.':
			if p.posContainerHigh == -1 {
				var high int
				if len(p.identifiers) > 0 {
					high = p.identifiers[len(p.identifiers)-1].Low - 1
				} else {
					high = len(p.s)
				}
				p.identifiers = append(p.identifiers, types.LowHigh{Low: i + 1, High: high})
			}
		case '/':
			if p.posContainerHigh == -1 {
				p.posContainerHigh = i + 1
			} else if p.posContainerLow == -1 {
				p.posContainerLow = i + 1
			}
			if i > 0 {
				p.posSectionHigh = i
			}
		}
	}

	isContent := p.component == files.ComponentFolderContent && files.IsContentExt(p.Ext())

	if isContent {
		id := p.identifiers[len(p.identifiers)-1]
		b := p.s[p.posContainerHigh : id.Low-1]
		switch b {
		case "index":
			p.bundleType = PathTypeLeaf
		case "_index":
			p.bundleType = PathTypeBranch
		default:
			p.bundleType = PathTypeContentSingle
		}
	}

	return p, nil
}

// TODO1 remvoe me
type _Path interface {
	identity.Identity
	Component() string
	Container() string
	Section() string
	Name() string
	NameNoExt() string
	NameNoIdentifier() string
	Base() string
	Dir() string
	Ext() string
	Identifiers() []string
	Identifier(i int) string
	IsContent() bool
	IsBundle() bool
	IsLeafBundle() bool
	IsBranchBundle() bool
	BundleType() PathType
}

func ModifyPathBundleTypeResource(p *Path) {
	if p.IsContent() {
		p.bundleType = PathTypeContentResource
	} else {
		p.bundleType = PathTypeFile
	}
}

type PathInfos []*PathInfo

type PathType int

const (
	// A generic resource, e.g. a JSON file.
	PathTypeFile PathType = iota

	// All below are content files.
	// A resource of a content type with front matter.
	PathTypeContentResource

	// E.g. /blog/my-post.md
	PathTypeContentSingle

	// All bewlow are bundled content files.

	// Leaf bundles, e.g. /blog/my-post/index.md
	PathTypeLeaf

	// Branch bundles, e.g. /blog/_index.md
	PathTypeBranch
)

// TODO1 consider creating some smaller interface for this.
type Path struct {
	s string

	posContainerLow  int
	posContainerHigh int
	posSectionHigh   int

	component  string
	bundleType PathType

	identifiers []types.LowHigh
}

type PathInfo struct {
	*Path
	component string
	filename  string
}

func (p *PathInfo) Filename() string {
	return p.filename
}

func WithInfo(p *Path, filename string) *PathInfo {
	return &PathInfo{
		Path:     p,
		filename: filename,
	}
}

// IdentifierBase satifies identity.Identity.
// TODO1 componnt?
func (p *Path) IdentifierBase() any {
	return p.Base()
}

func (p *Path) Component() string {
	return p.component
}

func (p *Path) Container() string {
	if p.posContainerLow == -1 {
		return ""
	}
	return p.s[p.posContainerLow : p.posContainerHigh-1]
}

func (p *Path) Section() string {
	if p.posSectionHigh == -1 {
		return ""
	}
	return p.s[1:p.posSectionHigh]
}

// IsContent returns true if the path is a content file (e.g. mypost.md).
// Note that this will also return true for content files in a bundle.
func (p *Path) IsContent() bool {
	return p.BundleType() >= PathTypeContentResource
}

// isContentPage returns true if the path is a content file (e.g. mypost.md),
// but nof if inside a leaf bundle.
func (p *Path) isContentPage() bool {
	return p.BundleType() >= PathTypeContentSingle
}

// Name returns the last element of path.
func (p *Path) Name() string {
	if p.posContainerHigh > 0 {
		return p.s[p.posContainerHigh:]
	}
	return p.s
}

// Name returns the last element of path withhout any extension.
func (p *Path) NameNoExt() string {
	if i := p.identifierIndex(0); i != -1 {
		return p.s[p.posContainerHigh : p.identifiers[i].Low-1]
	}
	return p.s[p.posContainerHigh:]
}

// Name returns the last element of path withhout any language identifier.
func (p *Path) NameNoLang() string {
	i := p.identifierIndex(1)
	if i == -1 {
		return p.Name()
	}

	return p.s[p.posContainerHigh:p.identifiers[i].Low-1] + p.s[p.identifiers[i].High:]
}

// BaseNameNoIdentifier returns the logcical base name for a resource without any idenifier (e.g. no extension).
// For bundles this will be the containing directory's name, e.g. "blog".
func (p *Path) BaseNameNoIdentifier() string {
	if p.IsBundle() {
		return p.Container()
	}
	return p.NameNoIdentifier()
}

func (p *Path) NameNoIdentifier() string {
	if len(p.identifiers) > 0 {
		return p.s[p.posContainerHigh : p.identifiers[len(p.identifiers)-1].Low-1]
	}
	if i := p.identifierIndex(0); i != -1 {
	}
	return p.s[p.posContainerHigh:]
}

func (p *Path) Dir() (d string) {
	if p.posContainerHigh > 0 {
		d = p.s[:p.posContainerHigh-1]
	}
	if d == "" {
		d = "/"
	}
	return
}

func (p *Path) Path() (d string) {
	return p.s
}

// For content files, Base returns the path without any identifiers (extension, language code etc.).
// Any 'index' as the last path element is ignored.
//
// For other files (Resources), any extension is kept.
func (p *Path) Base() string {
	if len(p.identifiers) > 0 {
		if !p.isContentPage() && len(p.identifiers) == 1 {
			// Preserve extension.
			return p.s
		}

		id := p.identifiers[len(p.identifiers)-1]
		high := id.Low - 1

		if p.IsBundle() {
			high = p.posContainerHigh - 1
		}

		if p.isContentPage() {
			return p.s[:high]
		}

		// For txt files etc. we want to preserve the extension.
		id = p.identifiers[0]

		return p.s[:high] + p.s[id.Low-1:id.High]
	}
	return p.s
}

func (p *Path) Ext() string {
	return p.identifierAsString(0)
}

func (p *Path) Lang() string {
	return p.identifierAsString(1)
}

func (p *Path) Identifier(i int) string {
	return p.identifierAsString(i)
}

func (p *Path) Identifiers() []string {
	ids := make([]string, len(p.identifiers))
	for i, id := range p.identifiers {
		ids[i] = p.s[id.Low:id.High]
	}
	return ids
}

func (p *Path) BundleType() PathType {
	return p.bundleType
}

func (p *Path) IsBundle() bool {
	return p.bundleType >= PathTypeLeaf
}

func (p *Path) IsBranchBundle() bool {
	return p.bundleType == PathTypeBranch
}

func (p *Path) IsLeafBundle() bool {
	return p.bundleType == PathTypeLeaf
}

func (p *Path) identifierAsString(i int) string {
	i = p.identifierIndex(i)
	if i == -1 {
		return ""
	}

	id := p.identifiers[i]
	return p.s[id.Low:id.High]
}

func (p *Path) identifierIndex(i int) int {
	if i < 0 || i >= len(p.identifiers) {
		return -1
	}
	return i
}

// HasExt returns true if the Unix styled path has an extension.
func HasExt(p string) bool {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '.' {
			return true
		}
		if p[i] == '/' {
			return false
		}
	}
	return false
}
