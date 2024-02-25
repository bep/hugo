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
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/bep/overlayfs"
	"github.com/gohugoio/hugo/hugofs/files"

	radix "github.com/armon/go-radix"
	"github.com/spf13/afero"
)

var filepathSeparator = string(filepath.Separator)

var _ ReverseLookupProvider = (*rootMappingFs)(nil)

func FilterMounts(fss ...MountFs) []afero.Fs {
	var fs []afero.Fs
	for _, f := range fss {
		fs = append(fs, f)
	}
	return fs
}

// NewRootMappingFs creates a new slice of rootMappingFs on top of the provided with
// root mappings with some optional metadata about the root.
// Note that From represents a virtual root that maps to the actual filename in To.
func NewRootMappingFs(fs afero.Fs, rms ...RootMapping) ([]MountFs, error) {
	rootMapToReal := radix.New()
	// realMapToRoot := radix.New()
	var virtualRoots []RootMapping
	var fss []MountFs

	/*addMapping := func(key string, rm RootMapping, to *radix.Tree) {
		var mappings []RootMapping
		v, found := to.Get(key)
		if found {
			// There may be more than one language pointing to the same root.
			mappings = v.([]RootMapping)
		}
		mappings = append(mappings, rm)
		to.Insert(key, mappings)
	}*/

	for _, rm := range rms {
		(&rm).clean()

		rm.FromBase = files.ResolveComponentFolder(rm.From)

		if len(rm.To) < 2 {
			panic(fmt.Sprintf("invalid root mapping; from/to: %s/%s", rm.From, rm.To))
		}

		fi, err := fs.Stat(rm.To)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		if rm.Meta == nil {
			rm.Meta = NewFileMeta()
		}

		if rm.FromBase == "" {
			panic(" rm.FromBase is empty")
		}

		// Extract "blog" from "content/blog"
		rm.path = strings.TrimPrefix(strings.TrimPrefix(rm.From, rm.FromBase), filepathSeparator)

		rm.Meta.SourceRoot = fi.(MetaProvider).Meta().Filename
		rm.Meta.BaseDir = rm.ToBase
		rm.Meta.Module = rm.Module
		rm.Meta.ModuleOrdinal = rm.ModuleOrdinal
		rm.Meta.Component = rm.FromBase
		rm.Meta.IsProject = rm.IsProject

		meta := rm.Meta.Copy()

		rm.fi = NewFileMetaInfo(fi, meta)

		if fi.IsDir() {
			fss = append(fss, &mountDirFs{
				Fs: fs,
				rm: rm,
			})
		} else {
			_, name := filepath.Split(rm.From)
			meta.Name = name
			fss = append(fss, &mountFileFs{
				Fs: fs,
				rm: rm,
			})
			continue
		}

		// addMapping(filepathSeparator+rm.From, rm, rootMapToReal)
		rev := rm.To
		if !strings.HasPrefix(rev, filepathSeparator) {
			rev = filepathSeparator + rev
		}

		// addMapping(rev, rm, realMapToRoot)

		// virtualRoots = append(virtualRoots, rm)
	}

	rootMapToReal.Insert(filepathSeparator, virtualRoots)

	/*rfs := &rootMappingFs{
		Fs:            fs,
		rootMapToReal: rootMapToReal,
		realMapToRoot: realMapToRoot,
	}*/

	// fss = append(fss, rfs)

	return fss, nil
}

func newRootMappingFsFromFromTo(
	baseDir string,
	fs afero.Fs,
	fromTo ...string,
) ([]MountFs, error) {
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
	From          string    // The virtual mount.
	FromBase      string    // The base directory of the virtual mount.
	To            string    // The source directory or file.
	ToBase        string    // The base of To. May be empty if an absolute path was provided.
	Module        string    // The module path/ID.
	ModuleOrdinal int       // The module ordinal starting with 0 which is the project.
	IsProject     bool      // Whether this is a mount in the main project.
	Meta          *FileMeta // File metadata (lang etc.)

	fi           FileMetaInfo
	fiSingleFile FileMetaInfo // TODO1 remove this. // Also set when this mounts represents a single file with a rename func.
	path         string       // The virtual mount point, e.g. "blog".
}

type keyRootMappings struct {
	key   string
	roots []RootMapping
}

func (rm *RootMapping) isDir() bool {
	return rm.fiSingleFile == nil && rm.fi.IsDir()
}

func (rm *RootMapping) clean() {
	rm.From = rm.cleanFrom(rm.From)
	rm.To = rm.cleanTo(rm.To)
}

func (rm *RootMapping) cleanFrom(s string) string {
	return strings.Trim(filepath.Clean(s), filepathSeparator)
}

func (rm *RootMapping) cleanTo(s string) string {
	return filepath.Clean(s)
}

func (rm *RootMapping) cleanName(name string) string {
	name = strings.Trim(filepath.Clean(name), filepathSeparator)
	if name == "." {
		name = ""
	}
	return filepathSeparator + name
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

var _ FilesystemUnwrapper = (*rootMappingFs)(nil)

// A rootMappingFs maps several roots into one. Note that the root of this filesystem
// is directories only, and they will be returned in Readdir and Readdirnames
// in the order given.
type rootMappingFs struct {
	afero.Fs
	rootMapToReal *radix.Tree
	realMapToRoot *radix.Tree
}

func (fs *rootMappingFs) Mounts(base string) ([]FileMetaInfo, error) {
	base = filepathSeparator + fs.cleanName(base)
	roots := fs.getRootsWithPrefix(base)

	if roots == nil {
		return nil, nil
	}

	fss := make([]FileMetaInfo, len(roots))
	for i, r := range roots {

		if r.fiSingleFile != nil {
			// A single file mount.
			fss[i] = r.fiSingleFile
			continue
		}

		bfs := NewBasePathFs(fs.Fs, r.To)
		fs := bfs
		if r.Meta.InclusionFilter != nil {
			fs = newFilenameFilterFs(fs, r.To, r.Meta.InclusionFilter)
		}
		fs = decorateDirs(fs, r.Meta)
		fi, err := fs.Stat("")
		if err != nil {
			return nil, fmt.Errorf("rootMappingFs.Dirs: %w", err)
		}

		if !fi.IsDir() {
			fi.(FileMetaInfo).Meta().Merge(r.Meta)
		}

		fss[i] = fi.(FileMetaInfo)
	}

	return fss, nil
}

func (fs *rootMappingFs) UnwrapFilesystem() afero.Fs {
	return fs.Fs
}

func (fs rootMappingFs) Component() string {
	panic("don't call me")
}

// Filter creates a copy of this filesystem with only mappings matching a filter.
func (fs rootMappingFs) Filter(f func(m RootMapping) bool) MountFs {
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

// Open opens the named file for reading.
func (fs *rootMappingFs) Open(name string) (afero.File, error) {
	fis, err := fs.doStat(name)
	if err != nil {
		return nil, err
	}

	return fs.newUnionFile(fis...)
}

// Stat returns the os.FileInfo structure describing a given file.  If there is
// an error, it will be of type *os.PathError.
func (fs *rootMappingFs) Stat(name string) (os.FileInfo, error) {
	fis, err := fs.doStat(name)
	if err != nil {
		return nil, err
	}
	return fis[0], nil
}

type ComponentPath struct {
	Component string
	Path      string
	Lang      string
}

func (c ComponentPath) ComponentPathJoined() string {
	return path.Join(c.Component, c.Path)
}

type ReverseLookupProvider interface {
	ReverseLookup(filename string, checkExists bool) ([]ComponentPath, error)
	ReverseLookupComponent(component, filename string, checkExists bool) ([]ComponentPath, error)
}

// func (fs *rootMappingFs) ReverseStat(filename string) ([]FileMetaInfo, error)
func (fs *rootMappingFs) ReverseLookup(filename string, checkExists bool) ([]ComponentPath, error) {
	return fs.ReverseLookupComponent("", filename, checkExists)
}

func (fs *rootMappingFs) ReverseLookupComponent(component, filename string, checkExists bool) ([]ComponentPath, error) {
	filename = fs.cleanName(filename)
	key := filepathSeparator + filename

	s, roots := fs.getRootsReverse(key)

	if len(roots) == 0 {
		return nil, nil
	}

	var cps []ComponentPath

	base := strings.TrimPrefix(key, s)
	dir, name := filepath.Split(base)

	for _, first := range roots {
		if component != "" && first.FromBase != component {
			continue
		}
		if first.Meta.Rename != nil {
			name = first.Meta.Rename(name, true)
		}

		// Now we know that this file _could_ be in this fs.
		filename := filepathSeparator + filepath.Join(first.path, dir, name)

		if checkExists {
			// Confirm that it exists.
			_, err := fs.Stat(first.FromBase + filename)
			if err != nil {
				continue
			}
		}

		cps = append(cps, ComponentPath{
			Component: first.FromBase,
			Path:      paths.ToSlashTrimLeading(filename),
			Lang:      first.Meta.Lang,
		})
	}

	return cps, nil
}

func (fs *rootMappingFs) hasPrefix(prefix string) bool {
	hasPrefix := false
	fs.rootMapToReal.WalkPrefix(prefix, func(b string, v any) bool {
		hasPrefix = true
		return true
	})

	return hasPrefix
}

func (fs *rootMappingFs) getRoot(key string) []RootMapping {
	v, found := fs.rootMapToReal.Get(key)
	if !found {
		return nil
	}

	return v.([]RootMapping)
}

func (fs *rootMappingFs) getRoots(key string) (string, []RootMapping) {
	tree := fs.rootMapToReal
	levels := strings.Count(key, filepathSeparator)
	seen := make(map[RootMapping]bool)

	var roots []RootMapping
	var s string

	for {
		var found bool
		ss, vv, found := tree.LongestPrefix(key)
		if !found || (levels < 2 && ss == key) {
			break
		}

		for _, rm := range vv.([]RootMapping) {
			if !seen[rm] {
				seen[rm] = true
				roots = append(roots, rm)
			}
		}
		s = ss

		// We may have more than one root for this key, so walk up.
		oldKey := key
		key = filepath.Dir(key)
		if key == oldKey {
			break
		}
	}

	return s, roots
}

func (fs *rootMappingFs) getRootsReverse(key string) (string, []RootMapping) {
	tree := fs.realMapToRoot
	s, v, found := tree.LongestPrefix(key)
	if !found {
		return "", nil
	}
	return s, v.([]RootMapping)
}

func (fs *rootMappingFs) getRootsWithPrefix(prefix string) []RootMapping {
	var roots []RootMapping
	fs.rootMapToReal.WalkPrefix(prefix, func(b string, v any) bool {
		roots = append(roots, v.([]RootMapping)...)
		return false
	})

	return roots
}

func (fs *rootMappingFs) getAncestors(prefix string) []keyRootMappings {
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

func (fs *rootMappingFs) newUnionFile(fis ...FileMetaInfo) (afero.File, error) {
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
			return &rootMappingDir{DirOnlyOps: f, fs: fs, name: meta.Name, meta: meta}, nil
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

	info := func() (os.FileInfo, error) {
		return fis[0], nil
	}

	return overlayfs.OpenDir(merge, info, openers...)
}

func (fs *rootMappingFs) cleanName(name string) string {
	name = strings.Trim(filepath.Clean(name), filepathSeparator)
	if name == "." {
		name = ""
	}
	return name
}

func (rfs *rootMappingFs) collectDirEntries(prefix string) ([]iofs.DirEntry, error) {
	prefix = filepathSeparator + rfs.cleanName(prefix)

	var fis []iofs.DirEntry

	seen := make(map[string]bool) // Prevent duplicate directories
	level := strings.Count(prefix, filepathSeparator)

	collectDir := func(rm RootMapping, fi FileMetaInfo) error {
		f, err := fi.Meta().Open()
		if err != nil {
			return err
		}
		direntries, err := f.(iofs.ReadDirFile).ReadDir(-1)
		if err != nil {
			f.Close()
			return err
		}

		for _, fi := range direntries {
			meta := fi.(FileMetaInfo).Meta()
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

func (fs *rootMappingFs) doStat(name string) ([]FileMetaInfo, error) {
	name = fs.cleanName(name)
	key := filepathSeparator + name

	roots := fs.getRoot(key)

	if roots == nil {
		if fs.hasPrefix(key) {
			// We have directories mounted below this.
			// Make it look like a directory.
			return []FileMetaInfo{newDirNameOnlyFileInfo(name, nil, fs.virtualDirOpener(name))}, nil
		}

		// Find any real directories with this key.
		_, roots := fs.getRoots(key)
		if roots == nil {
			return nil, &os.PathError{Op: "LStat", Path: name, Err: os.ErrNotExist}
		}

		var err error
		var fis []FileMetaInfo

		for _, rm := range roots {
			var fi FileMetaInfo
			fi, err = fs.statRoot(rm, name)
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
		if !meta.InclusionFilter.Match(strings.TrimPrefix(meta.Filename, meta.SourceRoot), root.isDir()) {
			wasFiltered = true
			continue
		}
		if root.fiSingleFile != nil {
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
		return []FileMetaInfo{newDirNameOnlyFileInfo(name, roots[0].Meta, fs.virtualDirOpener(name))}, nil
	}

	if fileCount > 1 {
		// Not supported by this filesystem.
		return nil, fmt.Errorf("found multiple files with name %q, use .Readdir or the source filesystem directly", name)
	}

	return []FileMetaInfo{roots[0].fi}, nil
}

func (fs *rootMappingFs) statRoot(root RootMapping, filename string) (FileMetaInfo, error) {
	dir, name := filepath.Split(filename)
	if root.Meta.Rename != nil {
		// TODO1 true vs false.
		if n := root.Meta.Rename(name, true); n != name {
			filename = filepath.Join(dir, n)
		}
	}

	if !root.Meta.InclusionFilter.Match(root.trimFrom(filename), root.isDir()) {
		return nil, os.ErrNotExist
	}

	filename = root.filename(filename)
	fi, err := fs.Fs.Stat(filename)
	if err != nil {
		return nil, err
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

	fim := decorateFileInfo(fi, opener, "", root.Meta)

	return fim, nil
}

func (fs *rootMappingFs) virtualDirOpener(name string) func() (afero.File, error) {
	return func() (afero.File, error) { return &rootMappingDir{name: name, fs: fs}, nil }
}

func (fs *rootMappingFs) realDirOpener(name string, meta *FileMeta) func() (afero.File, error) {
	return func() (afero.File, error) {
		f, err := fs.Fs.Open(name)
		if err != nil {
			return nil, err
		}
		return &rootMappingDir{name: name, meta: meta, fs: fs, DirOnlyOps: f}, nil
	}
}

var _ iofs.ReadDirFile = (*rootMappingDir)(nil)

type oneFileDir struct {
	*noOpRegularFileOps
	name string
	fi   FileMetaInfo
}

func (f *oneFileDir) Close() error {
	return nil
}

func (f *oneFileDir) Name() string {
	return f.name
}

func (f *oneFileDir) Stat() (iofs.FileInfo, error) {
	return nil, errIsDir
}

func (f *oneFileDir) ReadDir(count int) ([]iofs.DirEntry, error) {
	return []iofs.DirEntry{f.fi}, nil
}

func (f *oneFileDir) Readdirnames(count int) ([]string, error) {
	return []string{f.fi.Name()}, nil
}

func (f *oneFileDir) Readdir(count int) ([]os.FileInfo, error) {
	panic("not supported: use ReadDir")
}

type rootMappingDir struct {
	*noOpRegularFileOps
	DirOnlyOps
	fs   *rootMappingFs
	name string
	meta *FileMeta
}

func (f *rootMappingDir) Close() error {
	if f.DirOnlyOps == nil {
		return nil
	}
	return f.DirOnlyOps.Close()
}

func (f *rootMappingDir) Name() string {
	return f.name
}

func (f *rootMappingDir) ReadDir(count int) ([]iofs.DirEntry, error) {
	if f.DirOnlyOps != nil {
		fis, err := f.DirOnlyOps.(iofs.ReadDirFile).ReadDir(count)
		if err != nil {
			return nil, err
		}

		var result []iofs.DirEntry
		for _, fi := range fis {
			fim := decorateFileInfo(fi, nil, "", f.meta)
			meta := fim.Meta()
			if f.meta.InclusionFilter.Match(strings.TrimPrefix(meta.Filename, meta.SourceRoot), fim.IsDir()) {
				result = append(result, fim)
			}
		}
		return result, nil
	}

	return f.fs.collectDirEntries(f.name)
}

// Sentinal error to signal that a file is a directory.
var errIsDir = errors.New("isDir")

func (f *rootMappingDir) Stat() (iofs.FileInfo, error) {
	return nil, errIsDir
}

func (f *rootMappingDir) Readdir(count int) ([]os.FileInfo, error) {
	panic("not supported: use ReadDir")
}

// Note that Readdirnames preserves the order of the underlying filesystem(s),
// which is usually directory order.
func (f *rootMappingDir) Readdirnames(count int) ([]string, error) {
	dirs, err := f.ReadDir(count)
	if err != nil {
		return nil, err
	}
	return dirEntriesToNames(dirs), nil
}

func dirEntriesToNames(fis []iofs.DirEntry) []string {
	names := make([]string, len(fis))
	for i, d := range fis {
		names[i] = d.Name()
	}
	return names
}
