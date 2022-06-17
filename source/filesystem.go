// Copyright 2016 The Hugo Authors. All rights reserved.
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
	"fmt"
	"path/filepath"
	"sync"

	"github.com/gohugoio/hugo/hugofs"
)

// Filesystem represents a source filesystem.
type Filesystem struct {
	files        []*File
	filesInit    sync.Once
	filesInitErr error

	Base string

	fi hugofs.FileMetaInfo

	SourceSpec
}

// NewFilesystem returns a new filesytem for a given source spec.
func (sp SourceSpec) NewFilesystem(base string) *Filesystem {
	return &Filesystem{SourceSpec: sp, Base: base}
}

func (sp SourceSpec) NewFilesystemFromFileMetaInfo(fi hugofs.FileMetaInfo) *Filesystem {
	return &Filesystem{SourceSpec: sp, fi: fi}
}

func (f *Filesystem) Walk(adder func(*File) error) error {
	walker := func(path string, fi hugofs.FileMetaInfo, err error) error {
		if err != nil {
<<<<<<< HEAD
			f.filesInitErr = fmt.Errorf("capture files: %w", err)
=======
			return err
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}

		if fi.IsDir() {
			return nil
		}

		meta := fi.Meta()
		filename := meta.Filename

		b, err := f.shouldRead(filename, fi)
		if err != nil {
			return err
		}

		file, err := NewFileInfo(fi)
		if err != nil {
			return err
		}

		if b {
			if err = adder(file); err != nil {
				return err
			}
		}

		return err
	}

	w := hugofs.NewWalkway(hugofs.WalkwayConfig{
		Fs:     f.SourceFs,
		Info:   f.fi,
		Root:   f.Base,
		WalkFn: walker,
	})

	return w.Walk()
}

func (f *Filesystem) _captureFiles() error {
	walker := func(path string, fi hugofs.FileMetaInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		meta := fi.Meta()
		filename := meta.Filename

		b, err := f.shouldRead(filename, fi)
		if err != nil {
			return err
		}

		if b {
			// err = f.add(fi)
		}

		return err
	}

	w := hugofs.NewWalkway(hugofs.WalkwayConfig{
		Fs:     f.SourceFs,
		Info:   f.fi,
		Root:   f.Base,
		WalkFn: walker,
	})

	return w.Walk()
}

func (f *Filesystem) shouldRead(filename string, fi hugofs.FileMetaInfo) (bool, error) {
	ignore := f.SourceSpec.IgnoreFile(fi.Meta().Filename)

	if fi.IsDir() {
		if ignore {
			return false, filepath.SkipDir
		}
		return false, nil
	}

	if ignore {
		return false, nil
	}

	return true, nil
}
