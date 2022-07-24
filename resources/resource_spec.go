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

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/resources/jsconfig"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/hexec"
	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/resources/postpub"

	"github.com/gohugoio/hugo/cache/filecache"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources/images"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
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
	if len(fd.TargetBasePaths) == 0 {
		// If not set, we publish the same resource to all hosts.
		// TODO1
		fd.TargetBasePaths = r.MultihostTargetBasePaths
	}

	if fd.OpenReadSeekCloser == nil {
		return nil, errors.New("OpenReadSeekCloser is nil")
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

	if fd.Name == "" {
		fd.Name = fd.TargetPath
	}

	mediaType := fd.MediaType
	if mediaType.IsZero() {
		ext := fd.Path.Ext()
		var (
			found      bool
			suffixInfo media.SuffixInfo
		)
		mediaType, suffixInfo, found = r.MediaTypes.GetFirstBySuffix(ext)
		// TODO(bep) we need to handle these ambiguous types better, but in this context
		// we most likely want the application/xml type.
		if suffixInfo.Suffix == "xml" && mediaType.SubType == "rss" {
			mediaType, found = r.MediaTypes.GetByType("application/xml")
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

	if fd.DependencyManager == nil {
		fd.DependencyManager = identity.NopManager
	}

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
