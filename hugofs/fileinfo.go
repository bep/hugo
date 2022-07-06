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

// Package hugofs provides the file systems used by Hugo.
package hugofs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gohugoio/hugo/hugofs/glob"

	"github.com/gohugoio/hugo/hugofs/files"
	"golang.org/x/text/unicode/norm"

	"errors"

	"github.com/gohugoio/hugo/common/hreflect"
	"github.com/gohugoio/hugo/common/htime"
	"github.com/gohugoio/hugo/common/paths"

	"github.com/spf13/afero"
)

func NewFileMeta() *FileMeta {
	return &FileMeta{}
}

// PathFile returns the relative file path for the file source.
func (f *FileMeta) PathFile() string {
	if f.BaseDir == "" {
		return f.Filename
	}
	return strings.TrimPrefix(strings.TrimPrefix(f.Filename, f.BaseDir), filepathSeparator)
}

type FileMeta struct {
	PathInfo *paths.Path

	Name             string
	Filename         string
	Path             string
	PathWalk         string
	OriginalFilename string
	BaseDir          string

	SourceRoot string
	MountRoot  string
	Module     string
	Component  string

	Weight     int
	IsOrdered  bool
	IsSymlink  bool
	IsRootFile bool
	IsProject  bool
	Watch      bool

	Classifier files.ContentClass

	SkipDir bool

	Lang         string
	Translations []string

	Fs           afero.Fs                                    `json:"-"` // Only set for dirs.
	OpenFunc     func() (afero.File, error)                  `json:"-"`
	StatFunc     func() (FileMetaDirEntry, error)            `json:"-"`
	JoinStatFunc func(name string) (FileMetaDirEntry, error) `json:"-"`

	// Include only files or directories that match.
	InclusionFilter *glob.FilenameFilter `json:"-"`

	// Rename the name part of the file (not the directory).
	Rename func(name string, toFrom bool) string
}

func (m *FileMeta) String() string {
	s, _ := json.MarshalIndent(m, "", "  ")
	return string(s)
}

func (m *FileMeta) Copy() *FileMeta {
	if m == nil {
		return NewFileMeta()
	}
	c := *m
	return &c
}

var fileMetaNoMerge = map[string]bool{
	"Filename": true,
	"Name":     true,
}

func (m *FileMeta) Merge(from *FileMeta) {
	if m == nil || from == nil {
		return
	}
	dstv := reflect.Indirect(reflect.ValueOf(m))
	srcv := reflect.Indirect(reflect.ValueOf(from))

	for i := 0; i < dstv.NumField(); i++ {
		if fileMetaNoMerge[dstv.Type().Field(i).Name] {
			continue
		}
		v := dstv.Field(i)
		if !v.CanSet() {
			continue
		}
		if !hreflect.IsTruthfulValue(v) {
			v.Set(srcv.Field(i))
		}
	}

	if m.InclusionFilter == nil {
		m.InclusionFilter = from.InclusionFilter
	}
}

func (f *FileMeta) Open() (afero.File, error) {
	if f.OpenFunc == nil {
		return nil, errors.New("OpenFunc not set")
	}
	return f.OpenFunc()
}

func (f *FileMeta) Stat() (FileMetaDirEntry, error) {
	if f.StatFunc == nil {
		return nil, errors.New("StatFunc not set")
	}
	return f.StatFunc()
}

func (f *FileMeta) JoinStat(name string) (FileMetaDirEntry, error) {
	if f.JoinStatFunc == nil {
		return nil, os.ErrNotExist
	}
	return f.JoinStatFunc(name)
}

type FileMetaDirEntry interface {
	fs.DirEntry
	MetaProvider

	// This is a real hybrid as it also implements the fs.FileInfo interface.
	FileInfoOptionals
}

type MetaProvider interface {
	Meta() *FileMeta
}

type FileInfoOptionals interface {
	Size() int64
	Mode() fs.FileMode
	ModTime() time.Time
	Sys() any
}

type FileNameIsDir interface {
	Name() string
	IsDir() bool
}

type FileInfoProvider interface {
	FileInfo() FileMetaDirEntry
}

type filenameProvider interface {
	Filename() string
}

var (
	_ filenameProvider = (*dirEntryMeta)(nil)
)

type dirEntryMeta struct {
	fs.DirEntry
	m    *FileMeta
	name string

	fi     fs.FileInfo
	fiInit sync.Once
}

func (fi *dirEntryMeta) Meta() *FileMeta {
	return fi.m
}

// Filename returns the full filename.
func (fi *dirEntryMeta) Filename() string {
	return fi.m.Filename
}

func (fi *dirEntryMeta) fileInfo() fs.FileInfo {
	var err error
	fi.fiInit.Do(func() {
		fi.fi, err = fi.DirEntry.Info()
	})
	if err != nil {
		panic(err)
	}
	return fi.fi
}

func (fi *dirEntryMeta) Size() int64 {
	return fi.fileInfo().Size()
}

func (fi *dirEntryMeta) Mode() fs.FileMode {
	return fi.fileInfo().Mode()
}

func (fi *dirEntryMeta) ModTime() time.Time {
	return fi.fileInfo().ModTime()
}

func (fi *dirEntryMeta) Sys() any {
	return fi.fileInfo().Sys()
}

// Name returns the file's name. Note that we follow symlinks,
// if supported by the file system, and the Name given here will be the
// name of the symlink, which is what Hugo needs in all situations.
// TODO1
func (fi *dirEntryMeta) Name() string {
	if name := fi.m.Name; name != "" {
		return name
	}
	return fi.DirEntry.Name()
}

type fileInfoOptionals struct {
}

func (fileInfoOptionals) Size() int64        { panic("not supported") }
func (fileInfoOptionals) Mode() fs.FileMode  { panic("not supported") }
func (fileInfoOptionals) ModTime() time.Time { panic("not supported") }
func (fileInfoOptionals) Sys() any           { panic("not supported") }

func NewFileMetaDirEntry(fi FileNameIsDir, m *FileMeta) FileMetaDirEntry {
	if m == nil {
		panic("FileMeta must be set")
	}
	if fim, ok := fi.(MetaProvider); ok {
		m.Merge(fim.Meta())
	}
	switch v := fi.(type) {
	case fs.DirEntry:
		return &dirEntryMeta{DirEntry: v, m: m}
	case fs.FileInfo:
		return &dirEntryMeta{DirEntry: dirEntry{v}, m: m}
	case nil:
		return &dirEntryMeta{DirEntry: dirEntry{}, m: m}
	default:
		panic(fmt.Sprintf("Unsupported type: %T", fi))
	}

}

type dirNameOnlyFileInfo struct {
	name    string
	modTime time.Time
}

func (fi *dirNameOnlyFileInfo) Name() string {
	return fi.name
}

func (fi *dirNameOnlyFileInfo) Size() int64 {
	panic("not implemented")
}

func (fi *dirNameOnlyFileInfo) Mode() os.FileMode {
	return os.ModeDir
}

func (fi *dirNameOnlyFileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *dirNameOnlyFileInfo) IsDir() bool {
	return true
}

func (fi *dirNameOnlyFileInfo) Sys() any {
	return nil
}

func newDirNameOnlyFileInfo(name string, meta *FileMeta, fileOpener func() (afero.File, error)) FileMetaDirEntry {
	name = normalizeFilename(name)
	_, base := filepath.Split(name)

	m := meta.Copy()
	if m.Filename == "" {
		m.Filename = name
	}
	m.OpenFunc = fileOpener
	m.IsOrdered = false

	return NewFileMetaDirEntry(
		&dirNameOnlyFileInfo{name: base, modTime: htime.Now()},
		m,
	)
}

// TODO1 remove fs
func decorateFileInfo(
	fi FileNameIsDir,
	fs afero.Fs, opener func() (afero.File, error),
	filename, filepath string, inMeta *FileMeta) FileMetaDirEntry {

	var meta *FileMeta
	var fim FileMetaDirEntry

	filepath = strings.TrimPrefix(filepath, filepathSeparator)

	var ok bool
	if fim, ok = fi.(FileMetaDirEntry); ok {
		meta = fim.Meta()
	} else {
		meta = NewFileMeta()
		fim = NewFileMetaDirEntry(fi, meta)
	}

	if opener != nil {
		meta.OpenFunc = opener
	}

	if fs != nil && fi.IsDir() {
		meta.Fs = fs
	}

	nfilepath := normalizeFilename(filepath)
	nfilename := normalizeFilename(filename)
	if nfilepath != "" {
		meta.Path = nfilepath
	}
	if nfilename != "" {
		meta.Filename = nfilename
	}

	meta.Merge(inMeta)

	return fim
}

func isSymlink(fi os.FileInfo) bool {
	return fi != nil && fi.Mode()&os.ModeSymlink == os.ModeSymlink
}

func DirEntriesToFileMetaDirEntries(fis []fs.DirEntry) []FileMetaDirEntry {
	fims := make([]FileMetaDirEntry, len(fis))
	for i, v := range fis {
		fims[i] = v.(FileMetaDirEntry)
	}
	return fims
}

func normalizeFilename(filename string) string {
	if filename == "" {
		return ""
	}
	if runtime.GOOS == "darwin" {
		// When a file system is HFS+, its filepath is in NFD form.
		return norm.NFC.String(filename)
	}
	return filename
}

func dirEntriesToNames(fis []fs.DirEntry) []string {
	names := make([]string, len(fis))
	for i, d := range fis {
		names[i] = d.Name()
	}
	return names
}

func fromSlash(filenames []string) []string {
	for i, name := range filenames {
		filenames[i] = filepath.FromSlash(name)
	}
	return filenames
}

func sortDirEntries(fis []fs.DirEntry) {
	sort.Slice(fis, func(i, j int) bool {
		fimi, fimj := fis[i].(FileMetaDirEntry), fis[j].(FileMetaDirEntry)
		return fimi.Meta().Filename < fimj.Meta().Filename
	})
}

// dirEntry is an adapter from os.FileInfo to fs.DirEntry
type dirEntry struct {
	fs.FileInfo
}

var _ fs.DirEntry = dirEntry{}

func (d dirEntry) Type() fs.FileMode { return d.FileInfo.Mode().Type() }

func (d dirEntry) Info() (fs.FileInfo, error) { return d.FileInfo, nil }
