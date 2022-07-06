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
	"io"
	"io/fs"
	iofs "io/fs"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

var (
	_ afero.Fs      = (*FilterFs)(nil)
	_ afero.Lstater = (*FilterFs)(nil)
	_ afero.File    = (*filterDir)(nil)
)

func NewFilterFs(fs afero.Fs) (afero.Fs, error) {
	applyMeta := func(fs *FilterFs, name string, fis []iofs.DirEntry) {
		for i, fi := range fis {
			if fi.IsDir() {
				fis[i] = decorateFileInfo(fi, fs, fs.getOpener(fi.(MetaProvider).Meta().Filename), "", "", nil).(iofs.DirEntry)
			}
		}
	}

	ffs := &FilterFs{
		fs:             fs,
		applyPerSource: applyMeta,
	}

	return ffs, nil
}

var (
	_ FilesystemUnwrapper = (*FilterFs)(nil)
)

// FilterFs is an ordered composite filesystem.
type FilterFs struct {
	fs afero.Fs

	applyPerSource func(fs *FilterFs, name string, fis []fs.DirEntry)
	applyAll       func(fis []fs.DirEntry)
}

func (fs *FilterFs) Chmod(n string, m os.FileMode) error {
	return syscall.EPERM
}

func (fs *FilterFs) Chtimes(n string, a, m time.Time) error {
	return syscall.EPERM
}

func (fs *FilterFs) Chown(n string, uid, gid int) error {
	return syscall.EPERM
}

func (fs *FilterFs) UnwrapFilesystem() afero.Fs {
	return fs.fs
}

func (fs *FilterFs) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	fi, b, err := lstatIfPossible(fs.fs, name)
	if err != nil {
		return nil, false, err
	}

	if fi.IsDir() {
		return decorateFileInfo(fi, fs, fs.getOpener(name), "", "", nil), false, nil
	}

	// TODO1?
	//parent := filepath.Dir(name)
	//fs.applyFilters(parent, -1, fi)

	return fi, b, nil
}

func (fs *FilterFs) Mkdir(n string, p os.FileMode) error {
	return syscall.EPERM
}

func (fs *FilterFs) MkdirAll(n string, p os.FileMode) error {
	return syscall.EPERM
}

func (fs *FilterFs) Name() string {
	return "WeightedFileSystem"
}

func (fs *FilterFs) Open(name string) (afero.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}

	return &filterDir{
		File: f,
		ffs:  fs,
	}, nil
}

func (fs *FilterFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return fs.fs.Open(name)
}

func (fs *FilterFs) Remove(n string) error {
	return syscall.EPERM
}

func (fs *FilterFs) RemoveAll(p string) error {
	return syscall.EPERM
}

func (fs *FilterFs) Rename(o, n string) error {
	return syscall.EPERM
}

func (fs *FilterFs) Stat(name string) (os.FileInfo, error) {
	fi, _, err := fs.LstatIfPossible(name)
	return fi, err
}

func (fs *FilterFs) Create(n string) (afero.File, error) {
	return nil, syscall.EPERM
}

func (fs *FilterFs) getOpener(name string) func() (afero.File, error) {
	return func() (afero.File, error) {
		return fs.Open(name)
	}
}

func (fs *FilterFs) applyFilters(name string, count int, fis ...fs.DirEntry) ([]fs.DirEntry, error) {
	if fs.applyPerSource != nil {
		fs.applyPerSource(fs, name, fis)
	}

	seen := make(map[string]bool)
	var duplicates []int
	for i, dir := range fis {
		if !dir.IsDir() {
			continue
		}
		if seen[dir.Name()] {
			duplicates = append(duplicates, i)
		} else {
			seen[dir.Name()] = true
		}
	}

	// Remove duplicate directories, keep first.
	if len(duplicates) > 0 {
		for i := len(duplicates) - 1; i >= 0; i-- {
			idx := duplicates[i]
			fis = append(fis[:idx], fis[idx+1:]...)
		}
	}

	if fs.applyAll != nil {
		fs.applyAll(fis)
	}

	if count > 0 && len(fis) >= count {
		return fis[:count], nil
	}

	return fis, nil
}

var _ fs.ReadDirFile = (*filterDir)(nil)

type filterDir struct {
	afero.File
	ffs *FilterFs
}

func (f *filterDir) ReadDir(count int) ([]fs.DirEntry, error) {
	fis, err := f.File.(fs.ReadDirFile).ReadDir(-1)
	if err != nil {
		return nil, err
	}
	return f.ffs.applyFilters(f.Name(), count, fis...)
}

func (f *filterDir) Readdir(count int) ([]os.FileInfo, error) {
	panic("not supported: Use ReadDir")
}

func (f *filterDir) Readdirnames(count int) ([]string, error) {
	dirsi, err := f.File.(iofs.ReadDirFile).ReadDir(count)
	if err != nil {
		return nil, err
	}

	dirs := make([]string, len(dirsi))
	for i, d := range dirsi {
		dirs[i] = d.Name()
	}
	return dirs, nil
}

func printFs(fs afero.Fs, path string, w io.Writer) {
	if fs == nil {
		return
	}
	afero.Walk(fs, path, func(path string, info os.FileInfo, err error) error {
		fmt.Println("p:::", path)
		return nil
	})
}

func sortAndremoveStringDuplicates(s []string) []string {
	ss := sort.StringSlice(s)
	ss.Sort()
	i := 0
	for j := 1; j < len(s); j++ {
		if !ss.Less(i, j) {
			continue
		}
		i++
		s[i] = s[j]
	}

	return s[:i+1]
}
