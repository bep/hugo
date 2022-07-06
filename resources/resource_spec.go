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

package resources

import (
	"errors"
	"mime"
	"path"
	"sync"

	"github.com/BurntSushi/locker"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/config/allconfig"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/resources/jsconfig"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/hexec"
	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/resources/postpub"

	"github.com/gohugoio/hugo/cache/filecache"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/resources/images"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
)

func NewSpec(
	s *helpers.PathSpec,
<<<<<<< HEAD
	common *SpecCommon, // may be nil
	imageCache *ImageCache, // may be nil
	incr identity.Incrementer,
	logger loggers.Logger,
	errorHandler herrors.ErrorSender,
	execHelper *hexec.Exec) (*Spec, error) {

	fileCaches, err := filecache.NewCaches(s)
=======
	fileCaches filecache.Caches,
	memCache *memcache.Cache,
	incr identity.Incrementer,
	logger loggers.Logger,
	errorHandler herrors.ErrorSender,
	execHelper *hexec.Exec,
	outputFormats output.Formats,
	mimeTypes media.Types) (*Spec, error) {

	imgConfig, err := images.DecodeConfig(s.Cfg.GetStringMap("imaging"))
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
	if err != nil {
		return nil, fmt.Errorf("failed to create file caches from configuration: %w", err)
	}

	conf := s.Cfg.GetConfig().(*allconfig.Config)
	imgConfig := conf.Imaging

	imaging, err := images.NewImageProcessor(imgConfig)
	if err != nil {
		return nil, err
	}

	if incr == nil {
		incr = &identity.IncrementByOne{}
	}

	if logger == nil {
		logger = loggers.NewErrorLogger()
	}

	permalinks, err := page.NewPermalinkExpander(s.URLize, conf.Permalinks)
	if err != nil {
		return nil, err
	}

<<<<<<< HEAD
	if common == nil {
		common = &SpecCommon{
			incr:       incr,
			FileCaches: fileCaches,
			PostBuildAssets: &PostBuildAssets{
				PostProcessResources: make(map[string]postpub.PostPublishedResource),
				JSConfigBuilder:      jsconfig.NewBuilder(),
			},
			ResourceCache: &ResourceCache{
				fileCache: fileCaches.AssetsCache(),
				cache:     make(map[string]any),
				nlocker:   locker.NewLocker(),
			},
		}
	}

	if imageCache == nil {
		imageCache = newImageCache(
			fileCaches.ImageCache(),
			s,
		)
	} else {
		imageCache = imageCache.WithPathSpec(s)

	}

	rs := &Spec{
		PathSpec:    s,
		Logger:      logger,
		ErrorSender: errorHandler,
		imaging:     imaging,
		ImageCache:  imageCache,
		ExecHelper:  execHelper,

		Permalinks: permalinks,

		SpecCommon: common,
	}
=======
	rs := &Spec{
		PathSpec:      s,
		Logger:        logger,
		ErrorSender:   errorHandler,
		imaging:       imaging,
		ExecHelper:    execHelper,
		incr:          incr,
		MediaTypes:    mimeTypes,
		OutputFormats: outputFormats,
		Permalinks:    permalinks,
		BuildConfig:   config.DecodeBuild(s.Cfg),
		FileCaches:    fileCaches,
		PostBuildAssets: &PostBuildAssets{
			PostProcessResources: make(map[string]postpub.PostPublishedResource),
			JSConfigBuilder:      jsconfig.NewBuilder(),
		},
		imageCache: newImageCache(
			fileCaches.ImageCache(),
			memCache,
			s,
		),
	}

	rs.ResourceCache = newResourceCache(rs, memCache)
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)

	return rs, nil
}

type Spec struct {
	*helpers.PathSpec

	Logger      loggers.Logger
	ErrorSender herrors.ErrorSender

<<<<<<< HEAD
	TextTemplates tpl.TemplateParseFinder

	Permalinks page.PermalinkExpander

	ImageCache *ImageCache
=======
	Permalinks  page.PermalinkExpander
	BuildConfig config.Build
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)

	// Holds default filter settings etc.
	imaging *images.ImageProcessor

	ExecHelper *hexec.Exec

	*SpecCommon
}

// The parts of Spec that's comoon for all sites.
type SpecCommon struct {
	incr          identity.Incrementer
	ResourceCache *ResourceCache
	FileCaches    filecache.Caches

	// Assets used after the build is done.
	// This is shared between all sites.
	*PostBuildAssets
}

type PostBuildAssets struct {
	postProcessMu        sync.RWMutex
	PostProcessResources map[string]postpub.PostPublishedResource
	JSConfigBuilder      *jsconfig.Builder
}

func (r *Spec) New(fd ResourceSourceDescriptor) (resource.Resource, error) {
<<<<<<< HEAD
	return r.newResourceFor(fd)
}

func (r *Spec) MediaTypes() media.Types {
	return r.Cfg.GetConfigSection("mediaTypes").(media.Types)
}

func (r *Spec) OutputFormats() output.Formats {
	return r.Cfg.GetConfigSection("outputFormats").(output.Formats)
}

func (r *Spec) BuildConfig() config.BuildConfig {
	return r.Cfg.GetConfigSection("build").(config.BuildConfig)
}

func (r *Spec) CacheStats() string {
	r.ImageCache.mu.RLock()
	defer r.ImageCache.mu.RUnlock()

	s := fmt.Sprintf("Cache entries: %d", len(r.ImageCache.store))

	count := 0
	for k := range r.ImageCache.store {
		if count > 5 {
			break
		}
		s += "\n" + k
		count++
	}

	return s
}

func (r *Spec) ClearCaches() {
	r.ImageCache.clear()
	r.ResourceCache.clear()
}

func (r *Spec) DeleteBySubstring(s string) {
	r.ImageCache.deleteIfContains(s)
}

func (s *Spec) String() string {
	return "spec"
}

// TODO(bep) clean up below
func (r *Spec) newGenericResource(sourceFs afero.Fs,
	targetPathBuilder func() page.TargetPaths,
	osFileInfo os.FileInfo,
	sourceFilename,
	baseFilename string,
	mediaType media.Type) *genericResource {
	return r.newGenericResourceWithBase(
		sourceFs,
		nil,
		nil,
		targetPathBuilder,
		osFileInfo,
		sourceFilename,
		baseFilename,
		mediaType,
		nil,
	)
}

func (r *Spec) newGenericResourceWithBase(
	sourceFs afero.Fs,
	openReadSeekerCloser resource.OpenReadSeekCloser,
	targetPathBaseDirs []string,
	targetPathBuilder func() page.TargetPaths,
	osFileInfo os.FileInfo,
	sourceFilename,
	baseFilename string,
	mediaType media.Type,
	data map[string]any,
) *genericResource {
	if osFileInfo != nil && osFileInfo.IsDir() {
		panic(fmt.Sprintf("dirs not supported resource types: %v", osFileInfo))
=======
	if len(fd.TargetBasePaths) == 0 {
		// If not set, we publish the same resource to all hosts.
		// TODO1
		fd.TargetBasePaths = r.MultihostTargetBasePaths
	}

	if fd.OpenReadSeekCloser == nil {
		return nil, errors.New("OpenReadSeekCloser is nil")
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
	}

	if fd.TargetPath == "" {
		return nil, errors.New("TargetPath is empty")
	}

	if fd.Path == nil {
		fd.Path = paths.Parse(fd.TargetPath)
	}

	if fd.RelPermalink == "" {
		fd.RelPermalink = fd.Path.Path()
	}

<<<<<<< HEAD
	g := &genericResource{
		resourceFileInfo:       gfi,
		resourcePathDescriptor: pathDescriptor,
		mediaType:              mediaType,
		resourceType:           resourceType,
		spec:                   r,
		params:                 make(map[string]any),
		name:                   baseFilename,
		title:                  baseFilename,
		resourceContent:        &resourceContent{},
		data:                   data,
	}

	return g
}

func (r *Spec) newResource(sourceFs afero.Fs, fd ResourceSourceDescriptor) (resource.Resource, error) {
	fi := fd.FileInfo
	var sourceFilename string

	if fd.OpenReadSeekCloser != nil {
	} else if fd.SourceFilename != "" {
		var err error
		fi, err = sourceFs.Stat(fd.SourceFilename)
		if err != nil {
			if herrors.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		sourceFilename = fd.SourceFilename
	} else {
		sourceFilename = fd.SourceFile.Filename()
	}

	if fd.RelTargetFilename == "" {
		fd.RelTargetFilename = sourceFilename
	}

	mimeType := fd.MediaType
	if mimeType.IsZero() {
		ext := strings.ToLower(filepath.Ext(fd.RelTargetFilename))
=======
	if fd.Name == "" {
		fd.Name = fd.TargetPath
	}

	mediaType := fd.MediaType
	if mediaType.IsZero() {
		ext := fd.Path.Ext()
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
		var (
			found      bool
			suffixInfo media.SuffixInfo
		)
<<<<<<< HEAD
		mimeType, suffixInfo, found = r.MediaTypes().GetFirstBySuffix(strings.TrimPrefix(ext, "."))
		// TODO(bep) we need to handle these ambiguous types better, but in this context
		// we most likely want the application/xml type.
		if suffixInfo.Suffix == "xml" && mimeType.SubType == "rss" {
			mimeType, found = r.MediaTypes().GetByType("application/xml")
=======
		mediaType, suffixInfo, found = r.MediaTypes.GetFirstBySuffix(ext)
		// TODO(bep) we need to handle these ambiguous types better, but in this context
		// we most likely want the application/xml type.
		if suffixInfo.Suffix == "xml" && mediaType.SubType == "rss" {
			mediaType, found = r.MediaTypes.GetByType("application/xml")
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
		}

		if !found {
			// A fallback. Note that mime.TypeByExtension is slow by Hugo standards,
			// so we should configure media types to avoid this lookup for most
			// situations.
			mimeStr := mime.TypeByExtension("." + ext)
			if mimeStr != "" {
				mediaType, _ = media.FromStringAndExt(mimeStr, ext)
			}
		}
	}

<<<<<<< HEAD
	gr := r.newGenericResourceWithBase(
		sourceFs,
		fd.OpenReadSeekCloser,
		fd.TargetBasePaths,
		fd.TargetPaths,
		fi,
		sourceFilename,
		fd.RelTargetFilename,
		mimeType,
		fd.Data)
=======
	if fd.DependencyManager == nil {
		fd.DependencyManager = identity.NopManager
	}
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)

	fpath, fname := path.Split(fd.TargetPath)
	lpath, lname := path.Split(fd.RelPermalink)

	gr := &genericResource{
		groupIdentity:     fd.GroupIdentity,
		dependencyManager: fd.DependencyManager,
		mediaType:         mediaType,
		resourceType:      mediaType.MainType,
		openSource:        fd.OpenReadSeekCloser,
		targetPath:        dirFile{dir: fpath, file: fname},
		relPermalink:      dirFile{dir: lpath, file: lname},
		h:                 &resourceHash{},
		spec:              r,
		params:            make(map[string]any),
		name:              fd.Name,
		title:             fd.Name,
		resourceContent:   &resourceContent{},
	}

	if mediaType.MainType == "image" {
		imgFormat, ok := images.ImageFormatFromMediaSubType(mediaType.SubType)
		if ok {
			ir := &imageResource{
				Image:        images.NewImage(imgFormat, r.imaging, nil, gr),
				baseResource: gr,
			}
			ir.root = ir
			return newResourceAdapter(gr.spec, fd.LazyPublish, ir), nil
		}
	}

	/*gr := r.newGenericResourceWithBase(
		fd.GroupIdentity,
		fd.DependencyManager,
		sourceFs,
		fd.OpenReadSeekCloser,
		fd.TargetBasePaths,
		fd.TargetPathsRemoveMe,
		fi,
		sourceFilename,
		fd.RelTargetFilename,
		mimeType,
	)*/

	return newResourceAdapter(gr.spec, fd.LazyPublish, gr), nil
}

func (s *Spec) String() string {
	return "spec"
}
