// Copyright 2024 The Hugo Authors. All rights reserved.
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
	"os"
	"path/filepath"
	"strings"

	"github.com/gohugoio/hugo/common/paths"
	"github.com/spf13/afero"
)

type MountFs interface {
	afero.Fs
	ReverseLookupProvider
	Mounts(base string) ([]FileMetaInfo, error)
	Filter(f func(m RootMapping) bool) MountFs
	Component() string
}

type mountDirFs struct {
	afero.Fs
	rm RootMapping
}

func (fs *mountDirFs) Component() string {
	return fs.rm.FromBase
}

func (fs *mountDirFs) Stat(name string) (os.FileInfo, error) {
	fi, _, err := fs.doStat(name)
	return fi, err
}

func (fs *mountDirFs) Open(name string) (afero.File, error) {
	_, filename, err := fs.doStat(name)
	if err != nil {
		return nil, err
	}
	return fs.Fs.Open(filename)
}

func (fs *mountDirFs) doStat(name string) (os.FileInfo, string, error) {
	name = fs.rm.cleanFrom(name)

	if name == fs.rm.From {
		return fs.rm.fi, fs.rm.To, nil
	}

	if strings.HasPrefix(name, fs.rm.From) {
		base := fs.rm.trimFrom(name)
		filename := filepath.Join(fs.rm.To, base)
		fi, err := fs.Fs.Stat(filename)

		// TODO1 meta.
		return fi, filename, err
	}
	return nil, "", os.ErrNotExist
}

// TODO1 change return value to 1.
func (fs *mountDirFs) ReverseLookup(filename string, checkExists bool) ([]ComponentPath, error) {
	filename = fs.rm.cleanName(filename)
	if !strings.HasPrefix(filename, fs.rm.To) {
		return nil, nil
	}

	if checkExists {
		_, err := fs.Fs.Stat(filename)
		if err != nil {
			return nil, err
		}
	}
	filename = filepath.Join(fs.rm.path, strings.TrimPrefix(filename, fs.rm.To))
	return []ComponentPath{{Component: fs.Component(), Path: paths.ToSlashTrimLeading(filename), Lang: fs.rm.Meta.Lang}}, nil
}

func (fs *mountDirFs) ReverseLookupComponent(component, filename string, checkExists bool) ([]ComponentPath, error) {
	if component != fs.Component() {
		return nil, nil
	}
	return fs.ReverseLookup(filename, checkExists)
}

// TODO1 remove this (or replace with something else)
func (fs *mountDirFs) Filter(f func(m RootMapping) bool) MountFs {
	panic("not implemented")
}

func (fs *mountDirFs) Mounts(base string) ([]FileMetaInfo, error) {
	panic("not implemented")
}

type mountFileFs struct {
	afero.Fs
	rm RootMapping
}

func (fs *mountFileFs) Component() string {
	return fs.rm.FromBase
}

func (fs *mountFileFs) Stat(name string) (os.FileInfo, error) {
	_, isDir, err := fs.doStat(name)
	if err != nil {
		return nil, err
	}
	if isDir {
		return newDirNameOnlyFileInfo(filepath.Base(fs.rm.From), fs.rm.Meta, nil), nil
	}
	return fs.rm.fi, nil
}

func (fs *mountFileFs) Open(name string) (afero.File, error) {
	name, isDir, err := fs.doStat(name)
	if err != nil {
		return nil, err
	}
	if isDir {
		return &oneFileDir{name: name, fi: fs.rm.fi}, nil
	}
	return fs.Fs.Open(name)
}

func (fs *mountFileFs) doStat(name string) (string, bool, error) {
	if name == fs.rm.From {
		return fs.rm.To, false, nil
	}
	dir := filepath.Dir(fs.rm.From)
	if name == dir {
		return dir, true, nil
	}

	return "", false, os.ErrNotExist
}

func (fs *mountFileFs) Filter(f func(m RootMapping) bool) MountFs {
	panic("not implemented")
}

func (fs *mountFileFs) Mounts(base string) ([]FileMetaInfo, error) {
	panic("not implemented")
}

func (fs *mountFileFs) ReverseLookup(filename string, checkExists bool) ([]ComponentPath, error) {
	panic("not implemented")
}

func (fs *mountFileFs) ReverseLookupComponent(component, filename string, checkExists bool) ([]ComponentPath, error) {
	if component != fs.Component() {
		return nil, nil
	}

	filename = fs.rm.cleanName(filename)
	if filename != fs.rm.To {
		return nil, nil
	}

	if checkExists {
		_, err := fs.Fs.Stat(filename)
		if err != nil {
			return nil, err
		}
	}

	filename = paths.ToSlashTrimLeading(fs.rm.path)
	return []ComponentPath{{Component: fs.Component(), Path: filename, Lang: fs.rm.Meta.Lang}}, nil
}

func (fs *mountFileFs) UnwrapFilesystem() afero.Fs {
	return fs.Fs
}
