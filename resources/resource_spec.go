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
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/resources/jsconfig"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/hexec"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/resources/postpub"

	"github.com/gohugoio/hugo/cache/filecache"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources/images"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
	"github.com/spf13/afero"
)

func NewSpec(
	s *helpers.PathSpec,
	fileCaches filecache.Caches,
	memCache *memcache.Cache,
	incr identity.Incrementer,
	logger loggers.Logger,
	errorHandler herrors.ErrorSender,
	execHelper *hexec.Exec,
	outputFormats output.Formats,
	mimeTypes media.Types) (*Spec, error) {

	imgConfig, err := images.DecodeConfig(s.Cfg.GetStringMap("imaging"))
	if err != nil {
		return nil, err
	}

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

	permalinks, err := page.NewPermalinkExpander(s)
	if err != nil {
		return nil, err
	}

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

	return rs, nil
}

type Spec struct {
	*helpers.PathSpec

	MediaTypes    media.Types
	OutputFormats output.Formats

	Logger      loggers.Logger
	ErrorSender herrors.ErrorSender

	Permalinks  page.PermalinkExpander
	BuildConfig config.Build

	// Holds default filter settings etc.
	imaging *images.ImageProcessor

	ExecHelper *hexec.Exec

	incr          identity.Incrementer
	imageCache    *imageCache
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
	return r.newResourceFor(fd)
}

func (s *Spec) String() string {
	return "spec"
}

// TODO(bep) clean up below
func (r *Spec) newGenericResourceWithBase(
	groupIdentity identity.Identity,
	dependencyManager identity.Manager,
	sourceFs afero.Fs,
	openReadSeekerCloser resource.OpenReadSeekCloser,
	targetPathBaseDirs []string,
	targetPathBuilder func() page.TargetPaths,
	osFileInfo os.FileInfo,
	sourceFilename,
	baseFilename string,
	mediaType media.Type) *genericResource {

	if osFileInfo != nil && osFileInfo.IsDir() {
		panic(fmt.Sprintf("dirs not supported resource types: %v", osFileInfo))
	}

	// This value is used both to construct URLs and file paths, but start
	// with a Unix-styled path.
	baseFilename = helpers.ToSlashTrimLeading(baseFilename)
	fpath, fname := path.Split(baseFilename)

	resourceType := mediaType.MainType

	pathDescriptor := &resourcePathDescriptor{
		baseTargetPathDirs: helpers.UniqueStringsReuse(targetPathBaseDirs),
		targetPathBuilder:  targetPathBuilder,
		relTargetDirFile:   dirFile{dir: fpath, file: fname},
	}

	var fim hugofs.FileMetaDirEntry
	if osFileInfo != nil {
		fim = osFileInfo.(hugofs.FileMetaDirEntry)
	}

	gfi := &resourceFileInfo{
		fi:                   fim,
		openReadSeekerCloser: openReadSeekerCloser,
		sourceFs:             sourceFs,
		sourceFilename:       sourceFilename,
		h:                    &resourceHash{},
	}

	g := &genericResource{
		groupIdentity:          groupIdentity,
		dependencyManager:      dependencyManager,
		resourceFileInfo:       gfi,
		resourcePathDescriptor: pathDescriptor,
		mediaType:              mediaType,
		resourceType:           resourceType,
		spec:                   r,
		params:                 make(map[string]any),
		name:                   baseFilename,
		title:                  baseFilename,
		resourceContent:        &resourceContent{},
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
			if os.IsNotExist(err) {
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
		var (
			found      bool
			suffixInfo media.SuffixInfo
		)
		mimeType, suffixInfo, found = r.MediaTypes.GetFirstBySuffix(strings.TrimPrefix(ext, "."))
		// TODO(bep) we need to handle these ambiguous types better, but in this context
		// we most likely want the application/xml type.
		if suffixInfo.Suffix == "xml" && mimeType.SubType == "rss" {
			mimeType, found = r.MediaTypes.GetByType("application/xml")
		}

		if !found {
			// A fallback. Note that mime.TypeByExtension is slow by Hugo standards,
			// so we should configure media types to avoid this lookup for most
			// situations.
			mimeStr := mime.TypeByExtension(ext)
			if mimeStr != "" {
				mimeType, _ = media.FromStringAndExt(mimeStr, ext)
			}
		}
	}

	if fd.GroupIdentity == nil {
		// TODO1
		fd.GroupIdentity = identity.StringIdentity("/" + memcache.CleanKey(fd.RelTargetFilename))
	}

	if fd.DependencyManager == nil {
		fd.DependencyManager = identity.NopManager
	}

	gr := r.newGenericResourceWithBase(
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
	)

	if mimeType.MainType == "image" {
		imgFormat, ok := images.ImageFormatFromMediaSubType(mimeType.SubType)
		if ok {
			ir := &imageResource{
				Image:        images.NewImage(imgFormat, r.imaging, nil, gr),
				baseResource: gr,
			}
			ir.root = ir
			return newResourceAdapter(gr.spec, fd.LazyPublish, ir), nil
		}

	}

	return newResourceAdapter(gr.spec, fd.LazyPublish, gr), nil
}

func (r *Spec) newResourceFor(fd ResourceSourceDescriptor) (resource.Resource, error) {
	if fd.OpenReadSeekCloser == nil {
		if fd.SourceFile != nil && fd.SourceFilename != "" {
			return nil, errors.New("both SourceFile and AbsSourceFilename provided")
		} else if fd.SourceFile == nil && fd.SourceFilename == "" {
			return nil, errors.New("either SourceFile or AbsSourceFilename must be provided")
		}
	}

	if fd.RelTargetFilename == "" {
		fd.RelTargetFilename = fd.Filename()
	}

	if len(fd.TargetBasePaths) == 0 {
		// If not set, we publish the same resource to all hosts.
		fd.TargetBasePaths = r.MultihostTargetBasePaths
	}

	return r.newResource(fd.Fs, fd)
}
