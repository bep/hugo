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

package hugofs

import (
	"fmt"
	"io/fs"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bep/overlayfs"
	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/gohugoio/hugo/hugofs/glob"

	radix "github.com/armon/go-radix"
	"github.com/spf13/afero"
)

// SuffixReverseLookup is used in RootMappingFs.Stat to signal a reverse lookup.
const SuffixReverseLookup = "__reverse_lookup"

var filepathSeparator = string(filepath.Separator)

// NewRootMappingFs creates a new RootMappingFs on top of the provided with
// root mappings with some optional metadata about the root.
// Note that From represents a virtual root that maps to the actual filename in To.
func NewRootMappingFs(fs afero.Fs, rms ...RootMapping) (*RootMappingFs, error) {
	rootMapToReal := radix.New()
	realMapToRoot := radix.New()
	var virtualRoots []RootMapping

	addMapping := func(key string, rm RootMapping, to *radix.Tree) {
		var mappings []RootMapping
		v, found := to.Get(key)
		if found {
			// There may be more than one language pointing to the same root.
			mappings = v.([]RootMapping)
		}
		mappings = append(mappings, rm)
		to.Insert(key, mappings)
	}

	for _, rm := range rms {
		(&rm).clean()

		rm.FromBase = files.ResolveComponentFolder(rm.From)

		if len(rm.To) < 2 {
			panic(fmt.Sprintf("invalid root mapping; from/to: %s/%s", rm.From, rm.To))
		}

		fi, err := fs.Stat(rm.To)
		if err != nil {
			if herrors.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		if rm.Meta == nil {
			rm.Meta = NewFileMeta()
		}

		if !fi.IsDir() {
			// We do allow single file mounts.
			// However, the file system logic will be much simpler with just directories.
			// So, convert this mount into a directory mount with a nameTo filter and renamer.
			dirFrom, nameFrom := filepath.Split(rm.From)
			dirTo, nameTo := filepath.Split(rm.To)
			dirFrom, dirTo = strings.TrimSuffix(dirFrom, filepathSeparator), strings.TrimSuffix(dirTo, filepathSeparator)
			rm.From = dirFrom
			rm.To = dirTo
			rm.Meta.Rename = func(name string, toFrom bool) string {
				if toFrom {
					if name == nameTo {
						return nameFrom
					}
					return name
				}

				if name == nameFrom {
					return nameTo
				}

				return name
			}
			nameToFilename := filepathSeparator + nameTo

			rm.Meta.InclusionFilter = rm.Meta.InclusionFilter.Append(glob.NewFilenameFilterForInclusionFunc(
				func(filename string) bool {
					return strings.HasPrefix(nameToFilename, filename)
				},
			))

			// Refresh the FileInfo object.
			fi, err = fs.Stat(rm.To)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		}

		if rm.FromBase == "" {
			panic(" rm.FromBase is empty")
		}

		// Extract "blog" from "content/blog"
		rm.path = strings.TrimPrefix(strings.TrimPrefix(rm.From, rm.FromBase), filepathSeparator)

		rm.Meta.SourceRoot = fi.(MetaProvider).Meta().Filename
		rm.Meta.BaseDir = rm.ToBase
		rm.Meta.MountRoot = rm.path
		rm.Meta.Module = rm.Module
		rm.Meta.Component = rm.FromBase
		rm.Meta.IsProject = rm.IsProject

		meta := rm.Meta.Copy()

		if !fi.IsDir() {
			_, name := filepath.Split(rm.From)
			meta.Name = name
		}

		rm.fi = NewFileMetaDirEntry(fi, meta)

		addMapping(filepathSeparator+rm.From, rm, rootMapToReal)
		addMapping(filepathSeparator+rm.FromBase+strings.TrimPrefix(rm.To, rm.ToBase), rm, realMapToRoot)

		virtualRoots = append(virtualRoots, rm)
	}

	rootMapToReal.Insert(filepathSeparator, virtualRoots)

	rfs := &RootMappingFs{
		Fs:            fs,
		rootMapToReal: rootMapToReal,
		realMapToRoot: realMapToRoot,
	}

	return rfs, nil
}

func newRootMappingFsFromFromTo(
	baseDir string,
	fs afero.Fs,
	fromTo ...string,
) (*RootMappingFs, error) {
	rms := make([]RootMapping, len(fromTo)/2)
	for i, j := 0, 0; j < len(fromTo); i, j = i+1, j+2 {
		rms[i] = RootMapping{
			From:   fromTo[j],
			To:     fromTo[j+1],
			ToBase: baseDir,
		}
	}

	return NewRootMappingFs(fs, rms...)
}

// RootMapping describes a virtual file or directory mount.
type RootMapping struct {
	From      string    // The virtual mount.
	FromBase  string    // The base directory of the virtual mount.
	To        string    // The source directory or file.
	ToBase    string    // The base of To. May be empty if an absolute path was provided.
	Module    string    // The module path/ID.
	IsProject bool      // Whether this is a mount in the main project.
	Meta      *FileMeta // File metadata (lang etc.)

	fi   FileMetaDirEntry
	path string // The virtual mount point, e.g. "blog".

}

type keyRootMappings struct {
	key   string
	roots []RootMapping
}

func (rm *RootMapping) clean() {
	rm.From = strings.Trim(filepath.Clean(rm.From), filepathSeparator)
	rm.To = filepath.Clean(rm.To)
}

func (r RootMapping) filename(name string) string {
	if name == "" {
		return r.To
	}
	return filepath.Join(r.To, strings.TrimPrefix(name, r.From))
}

func (r RootMapping) trimFrom(name string) string {
	if name == "" {
		return ""
	}
	return strings.TrimPrefix(name, r.From)
}

var (
	_ FilesystemUnwrapper = (*RootMappingFs)(nil)
)

// A RootMappingFs maps several roots into one. Note that the root of this filesystem
// is directories only, and they will be returned in Readdir and Readdirnames
// in the order given.
type RootMappingFs struct {
	afero.Fs
	rootMapToReal *radix.Tree
	realMapToRoot *radix.Tree
}

func (fs *RootMappingFs) Dirs(base string) ([]FileMetaDirEntry, error) {
	base = filepathSeparator + fs.cleanName(base)
	roots := fs.getRootsWithPrefix(base)

	if roots == nil {
		return nil, nil
	}

	fss := make([]FileMetaDirEntry, len(roots))
	for i, r := range roots {
		bfs := afero.NewBasePathFs(fs.Fs, r.To)
		bfs = decoratePath(bfs, func(name string) string {
			p := strings.TrimPrefix(name, r.To)
			if r.path != "" {
				// Make sure it's mounted to a any sub path, e.g. blog
				p = filepath.Join(r.path, p)
			}
			p = strings.TrimLeft(p, filepathSeparator)
			return p
		})

		fs := bfs
		if r.Meta.InclusionFilter != nil {
			fs = newFilenameFilterFs(fs, r.To, r.Meta.InclusionFilter)
		}
		fs = decorateDirs(fs, r.Meta)
		fi, err := fs.Stat("")
		if err != nil {
			return nil, fmt.Errorf("RootMappingFs.Dirs: %w", err)
		}

		if !fi.IsDir() {
			fi.(FileMetaDirEntry).Meta().Merge(r.Meta)
		}

		fss[i] = fi.(FileMetaDirEntry)
	}

	return fss, nil
}

func (fs *RootMappingFs) UnwrapFilesystem() afero.Fs {
	return fs.Fs
}

// Filter creates a copy of this filesystem with only mappings matching a filter.
func (fs RootMappingFs) Filter(f func(m RootMapping) bool) *RootMappingFs {
	rootMapToReal := radix.New()
	fs.rootMapToReal.Walk(func(b string, v any) bool {
		rms := v.([]RootMapping)
		var nrms []RootMapping
		for _, rm := range rms {
			if f(rm) {
				nrms = append(nrms, rm)
			}
		}
		if len(nrms) != 0 {
			rootMapToReal.Insert(b, nrms)
		}
		return false
	})

	fs.rootMapToReal = rootMapToReal

	return &fs
}

// LstatIfPossible returns the os.FileInfo structure describing a given file.
func (fs *RootMappingFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if strings.HasSuffix(name, SuffixReverseLookup) {
		name = strings.TrimSuffix(name, SuffixReverseLookup)
		var err error
		name, err = fs.ReverseLookup(name)
		if err != nil {
			return nil, false, err
		}

		if name == "" {
			return nil, false, os.ErrNotExist
		}

	}

	fis, err := fs.doLstat(name)
	if err != nil {
		return nil, false, err
	}

	return fis[0], false, nil
}

// Open opens the named file for reading.
func (fs *RootMappingFs) Open(name string) (afero.File, error) {
	fis, err := fs.doLstat(name)
	if err != nil {
		return nil, err
	}

	return fs.newUnionFile(fis...)
}

// Stat returns the os.FileInfo structure describing a given file.  If there is
// an error, it will be of type *os.PathError.
func (fs *RootMappingFs) Stat(name string) (os.FileInfo, error) {
	fi, _, err := fs.LstatIfPossible(name)
	return fi, err
}

func (fs *RootMappingFs) ReverseLookup(filename string) (string, error) {
	filename = fs.cleanName(filename)
	key := filepathSeparator + filename

	s, roots := fs.getRootsReverse(key)

	if len(roots) == 0 {
		// TODO1 lang
		return "", nil
	}

	first := roots[0]

	base := strings.TrimPrefix(key, s)
	dir, name := filepath.Split(base)

	if first.Meta.Rename != nil {
		name = first.Meta.Rename(name, true)
	}

	return filepath.Join(first.FromBase, first.path, dir, name), nil

}

func (fs *RootMappingFs) hasPrefix(prefix string) bool {
	hasPrefix := false
	fs.rootMapToReal.WalkPrefix(prefix, func(b string, v any) bool {
		hasPrefix = true
		return true
	})

	return hasPrefix
}

func (fs *RootMappingFs) getRoot(key string) []RootMapping {
	v, found := fs.rootMapToReal.Get(key)
	if !found {
		return nil
	}

	return v.([]RootMapping)
}

func (fs *RootMappingFs) getRoots(key string) (string, []RootMapping) {
	return fs.getRootsIn(key, fs.rootMapToReal)
}

func (fs *RootMappingFs) getRootsReverse(key string) (string, []RootMapping) {
	return fs.getRootsIn(key, fs.realMapToRoot)
}

func (fs *RootMappingFs) getRootsIn(key string, tree *radix.Tree) (string, []RootMapping) {
	s, v, found := tree.LongestPrefix(key)
	if !found || (s == filepathSeparator && key != filepathSeparator) {
		return "", nil
	}
	return s, v.([]RootMapping)
}

func (fs *RootMappingFs) debug() {
	fmt.Println("rootMapToReal:")
	fs.rootMapToReal.Walk(func(s string, v any) bool {
		fmt.Println("Key", s)
		return false
	})

	fmt.Println("realMapToRoot:")
	fs.realMapToRoot.Walk(func(s string, v any) bool {
		fmt.Println("Key", s)
		return false
	})
}

func (fs *RootMappingFs) getRootsWithPrefix(prefix string) []RootMapping {
	var roots []RootMapping
	fs.rootMapToReal.WalkPrefix(prefix, func(b string, v any) bool {
		roots = append(roots, v.([]RootMapping)...)
		return false
	})

	return roots
}

func (fs *RootMappingFs) getAncestors(prefix string) []keyRootMappings {
	var roots []keyRootMappings
	fs.rootMapToReal.WalkPath(prefix, func(s string, v any) bool {
		if strings.HasPrefix(prefix, s+filepathSeparator) {
			roots = append(roots, keyRootMappings{
				key:   s,
				roots: v.([]RootMapping),
			})
		}
		return false
	})

	return roots
}

func (fs *RootMappingFs) newUnionFile(fis ...FileMetaDirEntry) (afero.File, error) {
	if len(fis) == 1 {
		return fis[0].Meta().Open()
	}

	openers := make([]func() (afero.File, error), len(fis))
	for i := len(fis) - 1; i >= 0; i-- {
		fi := fis[i]
		openers[i] = func() (afero.File, error) {
			meta := fi.Meta()
			f, err := meta.Open()
			if err != nil {
				return nil, err
			}
			return &rootMappingDir{File: f, fs: fs, name: meta.Name, meta: meta}, nil
		}
	}

	merge := func(lofi, bofi []iofs.DirEntry) []iofs.DirEntry {
		// Ignore duplicate directory entries
		for _, fi1 := range bofi {
			var found bool
			for _, fi2 := range lofi {
				if !fi2.IsDir() {
					continue
				}
				if fi1.Name() == fi2.Name() {
					found = true
					break
				}
			}
			if !found {
				lofi = append(lofi, fi1)
			}
		}

		return lofi
	}

	return overlayfs.OpenDir(merge, openers...)

	// TODO1
	/*uf.Merger = func(lofi, bofi []os.FileInfo) ([]os.FileInfo, error) {
		// Ignore duplicate directory entries
		seen := make(map[string]bool)
		var result []os.FileInfo

		for _, fis := range [][]os.FileInfo{bofi, lofi} {
			for _, fi := range fis {

				if fi.IsDir() && seen[fi.Name()] {
					continue
				}

				if fi.IsDir() {
					seen[fi.Name()] = true
				}

				result = append(result, fi)
			}
		}

		return result, nil
	}*/

}

func (fs *RootMappingFs) cleanName(name string) string {
	return strings.Trim(filepath.Clean(name), filepathSeparator)
}

func (rfs *RootMappingFs) collectDirEntries(prefix string) ([]fs.DirEntry, error) {
	prefix = filepathSeparator + rfs.cleanName(prefix)

	var fis []fs.DirEntry

	seen := make(map[string]bool) // Prevent duplicate directories
	level := strings.Count(prefix, filepathSeparator)

	collectDir := func(rm RootMapping, fi FileMetaDirEntry) error {
		f, err := fi.Meta().Open()
		if err != nil {
			return err
		}
		direntries, err := f.(fs.ReadDirFile).ReadDir(-1)
		if err != nil {
			f.Close()
			return err
		}

		for _, fi := range direntries {
			meta := fi.(FileMetaDirEntry).Meta()
			meta.Merge(rm.Meta)
			if !rm.Meta.InclusionFilter.Match(strings.TrimPrefix(meta.Filename, meta.SourceRoot), fi.IsDir()) {
				continue
			}

			if fi.IsDir() {
				name := fi.Name()
				if seen[name] {
					continue
				}
				seen[name] = true
				opener := func() (afero.File, error) {
					return rfs.Open(filepath.Join(rm.From, name))
				}
				fi = newDirNameOnlyFileInfo(name, meta, opener)
			} else if rm.Meta.Rename != nil {
				// TODO1 Dirs() and check if we can move it to rm.
				if n := rm.Meta.Rename(fi.Name(), true); n != fi.Name() {
					fi.(MetaProvider).Meta().Name = n
				}
			}
			fis = append(fis, fi)
		}

		f.Close()

		return nil
	}

	// First add any real files/directories.
	rms := rfs.getRoot(prefix)
	for _, rm := range rms {
		if err := collectDir(rm, rm.fi); err != nil {
			return nil, err
		}
	}

	// Next add any file mounts inside the given directory.
	prefixInside := prefix + filepathSeparator
	rfs.rootMapToReal.WalkPrefix(prefixInside, func(s string, v any) bool {
		if (strings.Count(s, filepathSeparator) - level) != 1 {
			// This directory is not part of the current, but we
			// need to include the first name part to make it
			// navigable.
			path := strings.TrimPrefix(s, prefixInside)
			parts := strings.Split(path, filepathSeparator)
			name := parts[0]

			if seen[name] {
				return false
			}
			seen[name] = true
			opener := func() (afero.File, error) {
				return rfs.Open(path)
			}

			fi := newDirNameOnlyFileInfo(name, nil, opener)
			fis = append(fis, fi)

			return false
		}

		rms := v.([]RootMapping)
		for _, rm := range rms {
			if !rm.fi.IsDir() {
				// A single file mount
				fis = append(fis, rm.fi)
				continue
			}
			name := filepath.Base(rm.From)
			if seen[name] {
				continue
			}
			seen[name] = true

			opener := func() (afero.File, error) {
				return rfs.Open(rm.From)
			}

			fi := newDirNameOnlyFileInfo(name, rm.Meta, opener)

			fis = append(fis, fi)

		}

		return false
	})

	// Finally add any ancestor dirs with files in this directory.
	ancestors := rfs.getAncestors(prefix)
	for _, root := range ancestors {
		subdir := strings.TrimPrefix(prefix, root.key)
		for _, rm := range root.roots {
			if rm.fi.IsDir() {
				fi, err := rm.fi.Meta().JoinStat(subdir)
				if err == nil {
					if err := collectDir(rm, fi); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return fis, nil
}

func (fs *RootMappingFs) doLstat(name string) ([]FileMetaDirEntry, error) {
	name = fs.cleanName(name)
	key := filepathSeparator + name

	roots := fs.getRoot(key)

	if roots == nil {
		if fs.hasPrefix(key) {
			// We have directories mounted below this.
			// Make it look like a directory.
			return []FileMetaDirEntry{newDirNameOnlyFileInfo(name, nil, fs.virtualDirOpener(name))}, nil
		}

		// Find any real directories with this key.
		_, roots := fs.getRoots(key)
		if roots == nil {
			return nil, &os.PathError{Op: "LStat", Path: name, Err: os.ErrNotExist}
		}

		var err error
		var fis []FileMetaDirEntry

		for _, rm := range roots {
			var fi FileMetaDirEntry
			fi, _, err = fs.statRoot(rm, name)
			if err == nil {
				fis = append(fis, fi)
			}
		}

		if fis != nil {
			return fis, nil
		}

		if err == nil {
			err = &os.PathError{Op: "LStat", Path: name, Err: err}
		}

		return nil, err
	}

	fileCount := 0
	var wasFiltered bool
	for _, root := range roots {
		meta := root.fi.Meta()
		if !meta.InclusionFilter.Match(strings.TrimPrefix(meta.Filename, meta.SourceRoot), root.fi.IsDir()) {
			wasFiltered = true
			continue
		}

		if !root.fi.IsDir() {
			fileCount++
		}
		if fileCount > 1 {
			break
		}
	}

	if fileCount == 0 {
		if wasFiltered {
			return nil, os.ErrNotExist
		}
		// Dir only.
		return []FileMetaDirEntry{newDirNameOnlyFileInfo(name, roots[0].Meta, fs.virtualDirOpener(name))}, nil
	}

	if fileCount > 1 {
		// Not supported by this filesystem.
		return nil, fmt.Errorf("found multiple files with name %q, use .Readdir or the source filesystem directly", name)
	}

	return []FileMetaDirEntry{roots[0].fi}, nil
}

func (fs *RootMappingFs) statRoot(root RootMapping, filename string) (FileMetaDirEntry, bool, error) {
	dir, name := filepath.Split(filename)
	if root.Meta.Rename != nil {
		if n := root.Meta.Rename(name, false); n != name {
			filename = filepath.Join(dir, n)
		}
	}

	if !root.Meta.InclusionFilter.Match(root.trimFrom(filename), root.fi.IsDir()) {
		return nil, false, os.ErrNotExist
	}

	filename = root.filename(filename)
	fi, b, err := lstatIfPossible(fs.Fs, filename)
	if err != nil {
		return nil, b, err
	}

	var opener func() (afero.File, error)
	if fi.IsDir() {
		// Make sure metadata gets applied in ReadDir.
		opener = fs.realDirOpener(filename, root.Meta)
	} else {
		if root.Meta.Rename != nil {
			if n := root.Meta.Rename(fi.Name(), true); n != fi.Name() {
				meta := fi.(MetaProvider).Meta()

				meta.Name = n

			}
		}

		// Opens the real file directly.
		opener = func() (afero.File, error) {
			return fs.Fs.Open(filename)
		}
	}

	fim := decorateFileInfo(fi, fs.Fs, opener, "", "", root.Meta)
	rel := filepath.Join(strings.TrimPrefix(dir, root.Meta.Component), fi.Name())
	fim.Meta().PathInfo = paths.Parse(filepath.ToSlash(rel))

	return fim, b, nil
}

func (fs *RootMappingFs) virtualDirOpener(name string) func() (afero.File, error) {
	return func() (afero.File, error) { return &rootMappingDir{name: name, fs: fs}, nil }
}

func (fs *RootMappingFs) realDirOpener(name string, meta *FileMeta) func() (afero.File, error) {
	return func() (afero.File, error) {
		f, err := fs.Fs.Open(name)
		if err != nil {
			return nil, err
		}
		return &rootMappingDir{name: name, meta: meta, fs: fs, File: f}, nil
	}
}

var _ fs.ReadDirFile = (*rootMappingDir)(nil)

type rootMappingDir struct {
	afero.File
	fs   *RootMappingFs
	name string
	meta *FileMeta
}

func (f *rootMappingDir) Close() error {
	if f.File == nil {
		return nil
	}
	return f.File.Close()
}

func (f *rootMappingDir) Name() string {
	return f.name
}

func (f *rootMappingDir) ReadDir(count int) ([]fs.DirEntry, error) {
	if f.File != nil {
		fis, err := f.File.(fs.ReadDirFile).ReadDir(count)
		if err != nil {
			return nil, err
		}

		var result []fs.DirEntry
		for _, fi := range fis {
			fim := decorateFileInfo(fi, f.fs, nil, "", "", f.meta)
			meta := fim.Meta()
			if f.meta.InclusionFilter.Match(strings.TrimPrefix(meta.Filename, meta.SourceRoot), fim.IsDir()) {
				result = append(result, fim)
			}
		}
		return result, nil
	}

	return f.fs.collectDirEntries(f.name)
}

func (f *rootMappingDir) Readdir(count int) ([]os.FileInfo, error) {
	panic("not supported: use ReadDir")
}

func (f *rootMappingDir) Readdirnames(count int) ([]string, error) {
	dirs, err := f.ReadDir(count)
	if err != nil {
		return nil, err
	}
	return dirEntriesToNames(dirs), nil
}
