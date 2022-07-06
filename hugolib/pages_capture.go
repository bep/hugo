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
	"fmt"
	pth "path"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/parser/pageparser"
	"github.com/spf13/afero"

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
		fs:      sp.BaseFs.Content.Fs,
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

	fs afero.Fs

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
		collectErr = c.collectDir(nil, nil)
	} else {
		for _, s := range c.h.Sites {
			s.pageMap.cfg.isRebuild = true
		}

		for _, id := range c.ids {
			if id.IsLeafBundle() {
				collectErr = c.collectDir(
					id.Path,
					func(fim hugofs.FileMetaDirEntry) bool {
						return true
					},
				)
			} else if id.IsBranchBundle() {
				isCascadingEdit := c.isCascadingEdit(id.Path)
				// bookmark cascade
				collectErr = c.collectDir(
					id.Path,
					func(fim hugofs.FileMetaDirEntry) bool {
						if isCascadingEdit {
							// Re-read all files below.
							return true
						}

						// TODO1 PathInfo for dirs.
						return strings.HasPrefix(id.Path.Path(), fim.Meta().PathInfo.Path())

					},
				)
			} else {
				// We always start from a directory.
				collectErr = c.collectDir(id.Path, func(fim hugofs.FileMetaDirEntry) bool {
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

func (c *pagesCollector) collectDir(dirPath *paths.Path, inFilter func(fim hugofs.FileMetaDirEntry) bool) error {
	var dpath string
	if dirPath != nil {
		dpath = filepath.FromSlash(dirPath.Dir())
	}
<<<<<<< HEAD
	return c.sp.Cfg.DefaultContentLanguage()
}

func (c *pagesCollector) addToBundle(info hugofs.FileMetaInfo, btyp bundleDirType, bundles pageBundles) error {
	getBundle := func(lang string) *fileinfoBundle {
		return bundles[lang]
	}

	cloneBundle := func(lang string) *fileinfoBundle {
		// Every bundled content file needs a content file header.
		// Use the default content language if found, else just
		// pick one.
		var (
			source *fileinfoBundle
			found  bool
		)

		source, found = bundles[c.sp.Cfg.DefaultContentLanguage()]
		if !found {
			for _, b := range bundles {
				source = b
				break
			}
		}

		if source == nil {
			panic(fmt.Sprintf("no source found, %d", len(bundles)))
		}

		clone := c.cloneFileInfo(source.header)
		clone.Meta().Lang = lang

		return &fileinfoBundle{
			header: clone,
		}
	}

	lang := c.getLang(info)
	bundle := getBundle(lang)
	isBundleHeader := c.isBundleHeader(info)
	if bundle != nil && isBundleHeader {
		// index.md file inside a bundle, see issue 6208.
		info.Meta().Classifier = files.ContentClassContent
		isBundleHeader = false
	}
	classifier := info.Meta().Classifier
	isContent := classifier == files.ContentClassContent
	if bundle == nil {
		if isBundleHeader {
			bundle = &fileinfoBundle{header: info}
			bundles[lang] = bundle
		} else {
			if btyp == bundleBranch {
				// No special logic for branch bundles.
				// Every language needs its own _index.md file.
				// Also, we only clone bundle headers for lonesome, bundled,
				// content files.
				return c.handleFiles(info)
			}

			if isContent {
				bundle = cloneBundle(lang)
				bundles[lang] = bundle
			}
		}
	}

	if !isBundleHeader && bundle != nil {
		bundle.resources = append(bundle.resources, info)
	}

	if classifier == files.ContentClassFile {
		translations := info.Meta().Translations

		for lang, b := range bundles {
			if !stringSliceContains(lang, translations...) && !b.containsResource(info.Name()) {

				// Clone and add it to the bundle.
				clone := c.cloneFileInfo(info)
				clone.Meta().Lang = lang
				b.resources = append(b.resources, clone)
			}
		}
	}

	return nil
}

func (c *pagesCollector) cloneFileInfo(fi hugofs.FileMetaInfo) hugofs.FileMetaInfo {
	return hugofs.NewFileMetaInfo(fi, hugofs.NewFileMeta())
}

func (c *pagesCollector) collectDir(dirname string, partial bool, inFilter func(fim hugofs.FileMetaInfo) bool) error {
	fi, err := c.fs.Stat(dirname)
	if err != nil {
		if herrors.IsNotExist(err) {
			// May have been deleted.
=======

	root, err := c.fs.Stat(dpath)
	if err != nil {
		if os.IsNotExist(err) {
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
			return nil
		}
		return err
	}

	if err := c.collectDirDir(dpath, root.(hugofs.FileMetaDirEntry), inFilter); err != nil {
		return err
	}

	return nil
}

func (c *pagesCollector) collectDirDir(path string, root hugofs.FileMetaDirEntry, inFilter func(fim hugofs.FileMetaDirEntry) bool) error {

	filter := func(fim hugofs.FileMetaDirEntry) bool {
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

	preHook := func(dir hugofs.FileMetaDirEntry, path string, readdir []hugofs.FileMetaDirEntry) ([]hugofs.FileMetaDirEntry, error) {
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

		rootMeta := dir.Meta()
		walkRoot := rootMeta.IsRootFile
		readdir = filtered

		var (
			// We merge language directories, so there can be duplicates, but they
			// will be ordered, most important first.
			// TODO1 reverse order so most important comes last.
			//duplicates        []int
			//seen              = make(map[string]bool)
			bundleFileCounters = make(map[string]int)
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
				bundleFileCounters[meta.Lang]++
			}

			// Folders with both index.md and _index.md type of files have
			// undefined behaviour and can never work.
			// The branch variant will win because of sort order, but log
			// a warning about it.
			if bundleFileCounters[meta.Lang] > 1 {
				c.logger.Warnf("Content directory %q have both index.* and _index.* files, pick one.", dir.Meta().Filename)
				// Reclassify it so it will be handled as a content file inside the
				// section, which is in line with the <= 0.55 behaviour.
				// TODO1 create issue, we now make it a bundle. meta.Classifier = files.ContentClassContent
			}
		}

		/*
			TODO1
			if btype > paths.BundleTypeNone && c.tracker != nil {
				c.tracker.add(path, btype)
			}*/

		switch btype {
		case paths.PathTypeBranch:
			if err := c.handleBundleBranch(readdir); err != nil {
				return nil, err
			}
		case paths.PathTypeLeaf:
			if err := c.handleBundleLeaf(dir, path, readdir); err != nil {
				return nil, err
			}
		default:
			if err := c.handleFiles(readdir...); err != nil {
				return nil, err
			}

		}

		if btype == paths.PathTypeLeaf { // YODO1 || partial {
			return nil, filepath.SkipDir
		}

		// Keep walking.
		return readdir, nil
	}

	var postHook hugofs.WalkHook
	if c.tracker != nil {
		// TODO1 remove
		postHook = func(dir hugofs.FileMetaDirEntry, path string, readdir []hugofs.FileMetaDirEntry) ([]hugofs.FileMetaDirEntry, error) {
			if c.tracker == nil {
				// Nothing to do.
				return readdir, nil
			}

			return readdir, nil
		}
	}

	wfn := func(path string, fi hugofs.FileMetaDirEntry, err error) error {
		if err != nil {
			return err
		}

		return nil
	}

	// Make sure the pages in this directory gets re-rendered,
	// even in fast render mode.
	// TODO1
	root.Meta().IsRootFile = true

	w := hugofs.NewWalkway(hugofs.WalkwayConfig{
		Logger:   c.logger,
		Root:     path,
		Info:     root,
		Fs:       c.fs,
		HookPre:  preHook,
		HookPost: postHook,
		WalkFn:   wfn,
	})

	return w.Walk()
}

func (c *pagesCollector) handleBundleBranch(readdir []hugofs.FileMetaDirEntry) error {
	for _, fim := range readdir {
		c.proc.Process(fim, pageProcessFiTypeBranch)
	}
	return nil
}

func (c *pagesCollector) handleBundleLeaf(dir hugofs.FileMetaDirEntry, path string, readdir []hugofs.FileMetaDirEntry) error {
	walk := func(path string, info hugofs.FileMetaDirEntry, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		pi := info.Meta().PathInfo

		if !pi.IsLeafBundle() {
			// Everything inside a leaf bundle is a Resource,
			// even the content pages.
			paths.ModifyPathBundleTypeResource(pi)
		}

		c.proc.Process(info, pageProcessFiTypeLeaf)

		return nil
	}

	// Start a new walker from the given path.
	w := hugofs.NewWalkway(
		hugofs.WalkwayConfig{
			Root:       path,
			Fs:         c.fs,
			Logger:     c.logger,
			Info:       dir,
			DirEntries: readdir,
			WalkFn:     walk,
		})

	return w.Walk()

}

func (c *pagesCollector) handleFiles(fis ...hugofs.FileMetaDirEntry) error {
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

// isCascadingEdit returns whether the dir represents a cascading edit.
// That is, if a front matter cascade section is removed, added or edited.
// If this is the case we must re-evaluate its descendants.
func (c *pagesCollector) isCascadingEdit(dir *paths.Path) bool {
	p := c.h.getPageFirstDimension(dir.Base())

	if p == nil {
		return false
	}

	if p.File() == nil {
		return false
	}

	f, err := p.File().FileInfo().Meta().Open()
	if err != nil {
		// File may have been removed, assume a cascading edit.
		// Some false positives is not too bad.
		return true
	}

	pf, err := pageparser.ParseFrontMatterAndContent(f)
	f.Close()
	if err != nil {
		return true
	}

	maps.PrepareParams(pf.FrontMatter)
	cascade1, ok := pf.FrontMatter["cascade"]
	hasCascade := p.m.cascade != nil
	if !ok {
		return hasCascade
	}
	if !hasCascade {
		return true
	}

	for _, v := range p.m.cascade {
		isCascade := !reflect.DeepEqual(cascade1, v)
		if isCascade {
			return true
		}
	}

	return false

}
