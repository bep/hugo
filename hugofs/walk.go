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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/hugofs/files"

	"github.com/spf13/afero"
)

type (
	WalkFunc func(path string, info FileMetaDirEntry, err error) error
	WalkHook func(dir FileMetaDirEntry, path string, readdir []FileMetaDirEntry) ([]FileMetaDirEntry, error)
)

type Walkway struct {
	fs        afero.Fs
	root      string
	basePath  string
	component string

	logger loggers.Logger

	// May be pre-set
	fi         FileMetaDirEntry
	dirEntries []FileMetaDirEntry

	walkFn WalkFunc
	walked bool

	// We may traverse symbolic links and bite ourself.
	seen map[string]bool

	// Optional hooks
	hookPre  WalkHook
	hookPost WalkHook
}

type WalkwayConfig struct {
	Fs   afero.Fs
	Root string

	// TODO1 check if we can remove.
	BasePath string

	Logger loggers.Logger

	// One or both of these may be pre-set.
	Info       FileMetaDirEntry
	DirEntries []FileMetaDirEntry

	// E.g. layouts, content etc.
	// Will be extraced from the above if not set.
	Component string

	WalkFn   WalkFunc
	HookPre  WalkHook
	HookPost WalkHook
}

func NewWalkway(cfg WalkwayConfig) *Walkway {
	if cfg.Fs == nil {
		panic("Fs must be set")
	}

	basePath := cfg.BasePath
	if basePath != "" && !strings.HasSuffix(basePath, filepathSeparator) {
		basePath += filepathSeparator
	}

	component := cfg.Component
	if component == "" {
		if cfg.Info != nil {
			component = cfg.Info.Meta().Component
		}
		if component == "" && len(cfg.DirEntries) > 0 {
			component = cfg.DirEntries[0].Meta().Component
		}

		if component == "" {
			component = files.ComponentFolderAssets
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = loggers.NewWarningLogger()
	}

	return &Walkway{
		fs:         cfg.Fs,
		root:       cfg.Root,
		basePath:   basePath,
		component:  component,
		fi:         cfg.Info,
		dirEntries: cfg.DirEntries,
		walkFn:     cfg.WalkFn,
		hookPre:    cfg.HookPre,
		hookPost:   cfg.HookPost,
		logger:     logger,
		seen:       make(map[string]bool),
	}
}

func (w *Walkway) Walk() error {
	if w.walked {
		panic("this walkway is already walked")
	}
	w.walked = true

	if w.fs == NoOpFs {
		return nil
	}

	var fi FileMetaDirEntry
	if w.fi != nil {
		fi = w.fi
		if fi.Meta().Component == "" {
			//return w.walkFn(w.root, nil, fmt.Errorf("FileMetaDirEntry: missing metadata for %q", fi.Name()))
		}
	} else {
		info, _, err := lstatIfPossible(w.fs, w.root)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}

			if w.checkErr(w.root, err) {
				return nil
			}
			return w.walkFn(w.root, nil, fmt.Errorf("walk: %q: %w", w.root, err))
		}
		fi = info.(FileMetaDirEntry)
	}

	if !fi.IsDir() {
		panic(fmt.Sprintf("%q is not a directory", fi.Name()))
	}

	return w.walk(w.root, fi, w.dirEntries, w.walkFn)
}

// if the filesystem supports it, use Lstat, else use fs.Stat
func lstatIfPossible(fs afero.Fs, path string) (os.FileInfo, bool, error) {
	if lfs, ok := fs.(afero.Lstater); ok {
		fi, b, err := lfs.LstatIfPossible(path)
		return fi, b, err
	}
	fi, err := fs.Stat(path)
	return fi, false, err
}

// checkErr returns true if the error is handled.
func (w *Walkway) checkErr(filename string, err error) bool {
	if err == ErrPermissionSymlink {
		logUnsupportedSymlink(filename, w.logger)
		return true
	}

	if os.IsNotExist(err) {
		// TODO1
		if true {
			return false
		}
		// The file may be removed in process.
		// This may be a ERROR situation, but it is not possible
		// to determine as a general case.
		w.logger.Warnf("File %q not found, skipping.", filename)
		return true
	}

	return false
}

func logUnsupportedSymlink(filename string, logger loggers.Logger) {
	logger.Warnf("Unsupported symlink found in %q, skipping.", filename)
}

// walk recursively descends path, calling walkFn.
// It follow symlinks if supported by the filesystem, but only the same path once.
func (w *Walkway) walk(path string, info FileMetaDirEntry, dirEntries []FileMetaDirEntry, walkFn WalkFunc) error {
	err := walkFn(path, info, nil)
	if err != nil {
		if info.IsDir() && err == filepath.SkipDir {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	meta := info.Meta()
	filename := meta.Filename

	if dirEntries == nil {
		f, err := w.fs.Open(path)

		if err != nil {
			if w.checkErr(path, err) {
				return nil
			}
			return walkFn(path, info, fmt.Errorf("walk: open %q (%q): %w", path, w.root, err))
		}

		fis, err := f.(fs.ReadDirFile).ReadDir(-1)

		f.Close()
		if err != nil {
			if w.checkErr(filename, err) {
				return nil
			}
			return walkFn(path, info, fmt.Errorf("walk: Readdir: %w", err))
		}

		dirEntries = DirEntriesToFileMetaDirEntries(fis)

		if !meta.IsOrdered {
			sort.Slice(dirEntries, func(i, j int) bool {
				fii := dirEntries[i]
				fij := dirEntries[j]

				fim, fjm := fii.Meta(), fij.Meta()

				// Pull bundle headers to the top.
				ficlass, fjclass := fim.Classifier, fjm.Classifier
				if ficlass != fjclass {
					return ficlass < fjclass
				}

				// With multiple content dirs with different languages,
				// there can be duplicate files, and a weight will be added
				// to the closest one.
				fiw, fjw := fim.Weight, fjm.Weight
				if fiw != fjw {

					return fiw > fjw
				}

				// When we walk into a symlink, we keep the reference to
				// the original name.
				fin, fjn := fim.Name, fjm.Name
				if fin != "" && fjn != "" {
					return fin < fjn
				}

				return fii.Name() < fij.Name()
			})
		}
	}

	// First add some metadata to the dir entries
	for _, fi := range dirEntries {
		fim := fi.(FileMetaDirEntry)

		meta := fim.Meta()

		// Note that we use the original Name even if it's a symlink.
		name := meta.Name
		if name == "" {
			name = fim.Name()
		}

		if name == "" {
			panic(fmt.Sprintf("[%s] no name set in %v", path, meta))
		}

		pathn := filepath.Join(path, name)

		pathMeta := meta.Path
		if pathMeta == "" {
			pathMeta = pathn
			if w.basePath != "" {
				pathMeta = strings.TrimPrefix(pathn, w.basePath)
			}
			pathMeta = normalizeFilename(pathMeta)
			meta.Path = pathMeta
		}

		if meta.Component == "" {
			meta.Component = w.component
		}

		meta.PathInfo = paths.Parse(pathMeta, paths.ForComponent(meta.Component))
		meta.PathWalk = pathn

		if meta.PathInfo.Lang() != "" {
			meta.Lang = meta.PathInfo.Lang()
		}

		if fim.IsDir() && w.isSeen(meta.Filename) {
			// TODO1
			// Prevent infinite recursion
			// Possible cyclic reference
			//meta.SkipDir = true
		}
	}

	if w.hookPre != nil {
		dirEntries, err = w.hookPre(info, path, dirEntries)
		if err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}

	for _, fi := range dirEntries {
		fim := fi.(FileMetaDirEntry)
		meta := fim.Meta()

		if meta.SkipDir {
			continue
		}

		err := w.walk(meta.PathWalk, fim, nil, walkFn)
		if err != nil {
			if !fi.IsDir() || err != filepath.SkipDir {
				return err
			}
		}
	}

	if w.hookPost != nil {
		dirEntries, err = w.hookPost(info, path, dirEntries)
		if err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}
	return nil
}

func (w *Walkway) isSeen(filename string) bool {
	if filename == "" {
		return false
	}

	if w.seen[filename] {
		return true
	}

	w.seen[filename] = true
	return false
}
