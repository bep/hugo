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

package source

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/common/hugio"

	"github.com/gohugoio/hugo/hugofs"

	"github.com/gohugoio/hugo/helpers"
)

// File describes a source file.
type File struct {
	fim hugofs.FileMetaDirEntry

	uniqueID string
	lazyInit sync.Once
}

// Filename returns a file's absolute path and filename on disk.
func (fi *File) Filename() string { return fi.fim.Meta().Filename }

// Path gets the relative path including file name and extension.  The directory
// is relative to the content root.
func (fi *File) Path() string { return filepath.Join(fi.p().Dir()[1:], fi.p().Name()) }

// Dir gets the name of the directory that contains this file.  The directory is
// relative to the content root.
func (fi *File) Dir() string {
	return fi.pathToDir(fi.p().Dir())
}

// Extension is an alias to Ext().
func (fi *File) Extension() string {
	helpers.Deprecated(".File.Extension", "Use .File.Ext instead. ", false)
	return fi.Ext()
}

// Ext returns a file's extension without the leading period (e.g. "md").
// Deprecated: Use Extension() instead.
func (fi *File) Ext() string { return fi.p().Ext() }

// Lang returns a file's language (e.g. "sv").
func (fi *File) Lang() string {
	return fi.fim.Meta().Lang
}

// LogicalName returns a file's name and extension (e.g. "page.sv.md").
func (fi *File) LogicalName() string {
	return fi.p().Name()
}

// BaseFileName returns a file's name without extension (e.g. "page.sv").
func (fi *File) BaseFileName() string {
	return fi.p().NameNoExt()
}

// TranslationBaseName returns a file's translation base name without the
// language segment (e.g. "page").
func (fi *File) TranslationBaseName() string { return fi.p().NameNoIdentifier() }

// ContentBaseName is a either TranslationBaseName or name of containing folder
// if file is a bundle.
func (fi *File) ContentBaseName() string {
	return fi.p().BaseNameNoIdentifier()
}

// Section returns a file's section.
func (fi *File) Section() string {
	return fi.p().Section()
}

// UniqueID returns a file's unique, MD5 hash identifier.
func (fi *File) UniqueID() string {
	fi.init()
	return fi.uniqueID
}

// FileInfo returns a file's underlying os.FileInfo.
func (fi *File) FileInfo() hugofs.FileMetaDirEntry { return fi.fim }

func (fi *File) String() string { return fi.BaseFileName() }

// Open implements ReadableFile.
func (fi *File) Open() (hugio.ReadSeekCloser, error) {
	f, err := fi.fim.Meta().Open()

	return f, err
}

func (fi *File) IsZero() bool {
	return fi == nil
}

// We create a lot of these FileInfo objects, but there are parts of it used only
// in some cases that is slightly expensive to construct.
func (fi *File) init() {
	fi.lazyInit.Do(func() {
		fi.uniqueID = helpers.MD5String(filepath.ToSlash(fi.Path()))
	})
}

func (fi *File) pathToDir(s string) string {
	if s == "" {
		return s
	}
	return filepath.FromSlash(s[1:] + "/")
}

func (fi *File) p() *paths.Path {
	return fi.fim.Meta().PathInfo
}

func NewFileInfoFrom(path, filename string) (*File, error) {
	meta := &hugofs.FileMeta{
		Filename: filename,
		Path:     path,
		PathInfo: paths.Parse(filepath.ToSlash(path)),
	}

	return NewFileInfo(hugofs.NewFileMetaDirEntry(nil, meta))
}

func NewFileInfo(fi hugofs.FileMetaDirEntry) (*File, error) {
	if fi.Meta().PathInfo == nil {
		return nil, errors.New("no path info")
	}

	f := &File{
		fim: fi,
	}

	return f, nil
}
