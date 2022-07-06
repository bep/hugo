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

package hugolib

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/source"

	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/hugofs"
)

func newPagesCollector(
	h *HugoSites,
	sp *source.SourceSpec,
	logger loggers.Logger,
	contentTracker *contentChangeMap,
	proc *pagesProcessor,
	ids paths.PathInfos) *pagesCollector {
	return &pagesCollector{
		h:       h,
		dirs:    sp.BaseFs.Content.Dirs,
		proc:    proc,
		sp:      sp,
		logger:  logger,
		ids:     ids,
		tracker: contentTracker,
	}
}

type pagesCollector struct {
	h      *HugoSites
	sp     *source.SourceSpec
	logger loggers.Logger

	dirs []hugofs.FileMetaInfo

	// Ordered list (bundle headers first) used in partial builds.
	// TODO1 check order
	ids paths.PathInfos

	// Content files tracker used in partial builds.
	tracker *contentChangeMap

	proc *pagesProcessor
}

// Collect collects content by walking the file system and storing
// it in the content tree.
// It may be restricted by filenames set on the collector (partial build).
func (c *pagesCollector) Collect() (collectErr error) {
	c.proc.Start(context.Background())
	defer func() {
		err := c.proc.Wait()
		if collectErr == nil {
			collectErr = err
		}
	}()

	if c.ids == nil {
		// Collect everything.
		collectErr = c.collectDir(nil, false, nil)
	} else {
		for _, s := range c.h.Sites {
			s.pageMap.cfg.isRebuild = true
		}

		for _, id := range c.ids {
			if id.IsLeafBundle() {
				collectErr = c.collectDir(id.Path, true, nil)
			} else if id.IsBranchBundle() {
				collectErr = c.collectDir(id.Path, true, nil)
			} else {
				// We always start from a directory.
				collectErr = c.collectDir(id.Path, true, func(fim hugofs.FileMetaInfo) bool {
					return id.Filename() == fim.Meta().Filename
				})
			}

			if collectErr != nil {
				break
			}
		}

	}

	return
}

func (c *pagesCollector) collectDir(dirPath *paths.Path, partial bool, inFilter func(fim hugofs.FileMetaInfo) bool) error {
	var dpath string
	if dirPath != nil {
		dpath = filepath.FromSlash(dirPath.Dir())
	}
	for _, dir := range c.dirs {
		if err := c.collectDirDir(dir, dpath, partial, inFilter); err != nil {
			return err
		}
	}

	return nil
}

func (c *pagesCollector) collectDirDir(rootDir hugofs.FileMetaInfo, dpath string, partial bool, inFilter func(fim hugofs.FileMetaInfo) bool) error {
	rootMeta := rootDir.Meta()
	if rootMeta.Lang == "" {
		rootMeta.Lang = c.sp.DefaultContentLanguage
	}
	fs := rootMeta.Fs

	_, err := fs.Stat(dpath)
	if err != nil {
		if os.IsNotExist(err) {
			// Dir has been deleted.
			return nil
		}
		return err
	}

	handleDir := func(
		btype paths.PathType,
		dir hugofs.FileMetaInfo,
		path string,
		readdir []hugofs.FileMetaInfo) error {

		/*
			TODO1
			if btype > paths.BundleTypeNone && c.tracker != nil {
				c.tracker.add(path, btype)
			}*/

		if btype == paths.PathTypeBranch {
			if err := c.handleBundleBranch(readdir); err != nil {
				return err
			}
			return nil
		} else if btype == paths.PathTypeLeaf {
			if err := c.handleBundleLeaf(dir, path, readdir); err != nil {
				return err
			}
			return nil
		}

		if err := c.handleFiles(readdir...); err != nil {
			return err
		}

		return nil
	}

	/*applyMetaDefaults := func(meta *hugofs.FileMeta) {
		// Make sure language is set.
		if meta.Lang == "" {
			if meta.PathInfo.Lang() != "" {
				meta.Lang = meta.PathInfo.Lang()
			} else {
				meta.Lang = rootDir.Meta().Lang
			}
		}
	}*/

	filter := func(fim hugofs.FileMetaInfo) bool {
		if fim.Meta().SkipDir {
			return false
		}

		if c.sp.IgnoreFile(fim.Meta().Filename) {
			return false
		}

		if inFilter != nil {
			return inFilter(fim)
		}
		return true
	}

	preHook := func(dir hugofs.FileMetaInfo, path string, readdir []hugofs.FileMetaInfo) ([]hugofs.FileMetaInfo, error) {
		var btype paths.PathType

		filtered := readdir[:0]
		for _, fi := range readdir {
			if filter(fi) {
				filtered = append(filtered, fi)

				if c.tracker != nil {
					// Track symlinks.
					c.tracker.addSymbolicLinkMapping(fi)
				}
			}
		}
		walkRoot := dir.Meta().IsRootFile
		readdir = filtered

		var (
			// We merge language directories, so there can be duplicates, but they
			// will be ordered, most important first.
			// TODO1 reverse order so most important comes last.
			//duplicates        []int
			//seen              = make(map[string]bool)
			bundleFileCounter int
		)

		for _, fi := range readdir {
			if fi.IsDir() {
				continue
			}

			// TODO1 PathInfo vs BundleType vs HTML with not front matter.
			meta := fi.Meta()
			pi := meta.PathInfo

			if meta.Lang == "" {
				meta.Lang = rootMeta.Lang
			}

			meta.IsRootFile = walkRoot
			// TODO1 remove the classifier class := meta.Classifier

			if pi.IsBundle() {
				btype = pi.BundleType()
				bundleFileCounter++
			}

			// Folders with both index.md and _index.md type of files have
			// undefined behaviour and can never work.
			// The branch variant will win because of sort order, but log
			// a warning about it.
			if bundleFileCounter > 1 {
				c.logger.Warnf("Content directory %q have both index.* and _index.* files, pick one.", dir.Meta().Filename)
				// Reclassify it so it will be handled as a content file inside the
				// section, which is in line with the <= 0.55 behaviour.
				// TODO1 create issue, we now make it a bundle. meta.Classifier = files.ContentClassContent
			}

		}

		err := handleDir(btype, dir, path, readdir)
		if err != nil {
			return nil, err
		}

		if btype == paths.PathTypeLeaf || partial {
			return nil, filepath.SkipDir
		}

		// Keep walking.
		return readdir, nil
	}

	var postHook hugofs.WalkHook
	if c.tracker != nil {
		// TODO1 remove
		postHook = func(dir hugofs.FileMetaInfo, path string, readdir []hugofs.FileMetaInfo) ([]hugofs.FileMetaInfo, error) {
			if c.tracker == nil {
				// Nothing to do.
				return readdir, nil
			}

			return readdir, nil
		}
	}

	wfn := func(path string, info hugofs.FileMetaInfo, err error) error {
		if err != nil {
			return err
		}

		return nil
	}

	// Make sure the pages in this directory gets re-rendered,
	// even in fast render mode.
	// TODO1
	rootDir.Meta().IsRootFile = true

	w := hugofs.NewWalkway(hugofs.WalkwayConfig{
		Logger:   c.logger,
		Root:     dpath,
		Info:     rootDir,
		HookPre:  preHook,
		HookPost: postHook,
		WalkFn:   wfn,
	})

	return w.Walk()
}

func (c *pagesCollector) handleBundleBranch(readdir []hugofs.FileMetaInfo) error {
	for _, fim := range readdir {
		c.proc.Process(fim, pageProcessFiTypeBranch)
	}
	return nil
}

func (c *pagesCollector) handleBundleLeaf(dir hugofs.FileMetaInfo, path string, readdir []hugofs.FileMetaInfo) error {
	walk := func(path string, info hugofs.FileMetaInfo, err error) error {
		if err != nil {
			return err
		}

		pathInfo := info.Meta().PathInfo
		if !pathInfo.IsLeafBundle() {
			// Everything inside a leaf bundle is a Resource,
			// even the content pages.
			paths.ModifyPathBundleTypeResource(pathInfo)
		}

		c.proc.Process(info, pageProcessFiTypeLeaf)

		return nil
	}

	// Start a new walker from the given path.
	w := hugofs.NewWalkway(
		hugofs.WalkwayConfig{
			Root:       path,
			Logger:     c.logger,
			Info:       dir,
			DirEntries: readdir,
			WalkFn:     walk,
		})

	return w.Walk()

}

func (c *pagesCollector) handleFiles(fis ...hugofs.FileMetaInfo) error {
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		typ := pageProcessFiTypeLeaf
		if fi.Meta().PathInfo.BundleType() < paths.PathTypeContentResource {
			typ = pageProcessFiTypeStaticFile
		}

		c.proc.Process(fi, typ)
	}
	return nil
}
