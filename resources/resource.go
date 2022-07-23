// Copyright 2022 The Hugo Authors. All rights reserved.
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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/gohugoio/hugo/identity"

	"github.com/gohugoio/hugo/resources/internal"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/paths"

	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/source"

	"errors"

	"github.com/gohugoio/hugo/common/hugio"
	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/resources/resource"
	"github.com/spf13/afero"

	"github.com/gohugoio/hugo/helpers"
)

var (
	_ resource.ContentResource           = (*genericResource)(nil)
	_ resource.ReadSeekCloserResource    = (*genericResource)(nil)
	_ resource.Resource                  = (*genericResource)(nil)
	_ identity.DependencyManagerProvider = (*genericResource)(nil)
	_ resource.Source                    = (*genericResource)(nil)
	_ resource.Cloner                    = (*genericResource)(nil)
	_ resource.ResourcesLanguageMerger   = (*resource.Resources)(nil)
	_ permalinker                        = (*genericResource)(nil)
	_ types.Identifier                   = (*genericResource)(nil)
	_ fileInfo                           = (*genericResource)(nil)
)

type ResourceSourceDescriptor struct {

	// Need one of these to load the resource content.
	SourceFile *source.File

	// Keep
	OpenReadSeekCloser resource.OpenReadSeekCloser

	Path       *paths.Path
	TargetPath string
	// Any base paths prepended to the target path. This will also typically be the
	// language code, but setting it here means that it should not have any effect on
	// the permalink.
	// This may be several values. In multihost mode we may publish the same resources to
	// multiple targets.
	TargetBasePaths []string
	RelPermalink    string
	// Keep

	// Delay publishing until either Permalink or RelPermalink is called. Maybe never.
	LazyPublish bool

	FileInfo os.FileInfo

	// TargetPathsRemoveMe is a callback to fetch paths's relative to its owner.
	// TODO1
	TargetPathsRemoveMe func() page.TargetPaths

	// The relative target filename without any language code.
	RelTargetFilename string

	// If OpenReadSeekerCloser is not set, we use this to open the file.
	SourceFilename string

	Fs afero.Fs

	// Set when its known up front, else it's resolved from the target filename.
	MediaType media.Type

	// Used to track depenencies (e.g. imports). May be nil if that's of no concern.
	DependencyManager identity.Manager

	// A shared identity for this resource and all its clones.
	// If this is not set, an Identity is created.
	GroupIdentity identity.Identity
}

func (r ResourceSourceDescriptor) Filename() string {
	if r.SourceFile != nil {
		return r.SourceFile.Filename()
	}
	return r.SourceFilename
}

type ResourceTransformer interface {
	resource.Resource
	Transformer
}

type Transformer interface {
	Transform(...ResourceTransformation) (ResourceTransformer, error)
}

func NewFeatureNotAvailableTransformer(key string, elements ...any) ResourceTransformation {
	return transformerNotAvailable{
		key: internal.NewResourceTransformationKey(key, elements...),
	}
}

type transformerNotAvailable struct {
	key internal.ResourceTransformationKey
}

func (t transformerNotAvailable) Transform(ctx *ResourceTransformationCtx) error {
	return herrors.ErrFeatureNotAvailable
}

func (t transformerNotAvailable) Key() internal.ResourceTransformationKey {
	return t.key
}

// resourceCopier is for internal use.
type resourceCopier interface {
	cloneTo(targetPath string) resource.Resource
}

// Copy copies r to the targetPath given.
func Copy(r resource.Resource, targetPath string) resource.Resource {
	if r.Err() != nil {
		panic(fmt.Sprintf("Resource has an .Err: %s", r.Err()))
	}
	return r.(resourceCopier).cloneTo(targetPath)
}

type baseResourceResource interface {
	resource.Cloner
	resourceCopier
	resource.ContentProvider
	resource.Resource
	types.Identifier
	identity.IdentityGroupProvider
	identity.DependencyManagerProvider
}

type baseResourceInternal interface {
	resource.Source

	fileInfo
	metaAssigner
	targetPather

	ReadSeekCloser() (hugio.ReadSeekCloser, error)

	// Internal
	cloneWithUpdates(*transformationUpdate) (baseResource, error)
	tryTransformedFileCache(key string, u *transformationUpdate) io.ReadCloser

	getTargetPathDirFile() dirFile

	specProvider
	getTargetFilenames() []string
	openDestinationsForWriting() (io.WriteCloser, error)
	openPublishFileForWriting(relTargetPath string) (io.WriteCloser, error)

	relTargetPathForRel(rel string, addBaseTargetPath, isAbs, isURL bool) string
}

type specProvider interface {
	getSpec() *Spec
}

type baseResource interface {
	baseResourceResource
	baseResourceInternal
}

type commonResource struct{}

// Slice is for internal use.
// for the template functions. See collections.Slice.
func (commonResource) Slice(in any) (any, error) {
	switch items := in.(type) {
	case resource.Resources:
		return items, nil
	case []any:
		groups := make(resource.Resources, len(items))
		for i, v := range items {
			g, ok := v.(resource.Resource)
			if !ok {
				return nil, fmt.Errorf("type %T is not a Resource", v)
			}
			groups[i] = g

		}
		return groups, nil
	default:
		return nil, fmt.Errorf("invalid slice type %T", items)
	}
}

type dirFile struct {
	// This is the directory component with Unix-style slashes.
	dir string
	// This is the file component.
	file string
}

func (d dirFile) path() string {
	return path.Join(d.dir, d.file)
}

type fileInfo interface {
	setOpenSource(resource.OpenReadSeekCloser)
	setTargetPath(dirFile)
	size() int
	hashProvider
}

type hashProvider interface {
	hash() string
}

type staler struct {
	stale uint32
}

func (s *staler) MarkStale() {
	atomic.StoreUint32(&s.stale, 1)
}

func (s *staler) IsStale() bool {
	return atomic.LoadUint32(&(s.stale)) > 0
}

// genericResource represents a generic linkable resource.
type genericResource struct {
	*resourceContent //

	openSource   resource.OpenReadSeekCloser
	targetPath   dirFile
	relPermalink dirFile

	// A hash of the source content. Is only calculated in caching situations.
	h *resourceHash

	groupIdentity     identity.Identity
	dependencyManager identity.Manager

	spec *Spec

	title  string
	name   string
	params map[string]any
	data   map[string]any

	resourceType string
	mediaType    media.Type
}

func (l *genericResource) GetIdentityGroup() identity.Identity {
	return l.groupIdentity
}

func (l *genericResource) GetDependencyManager() identity.Manager {
	return l.dependencyManager
}

func (l *genericResource) ReadSeekCloser() (hugio.ReadSeekCloser, error) {
	return l.openSource()
}

func (l *genericResource) size() int {
	l.hash()
	return l.h.size
}

func (l *genericResource) hash() string {
	l.h.init.Do(func() {
		var hash string
		var size int
		var f hugio.ReadSeekCloser
		f, err := l.ReadSeekCloser()
		if err != nil {
			err = fmt.Errorf("failed to open source: %w", err)
			return
		}
		defer f.Close()

		hash, size, err = helpers.MD5FromReaderFast(f)
		if err != nil {
			return
		}
		l.h.value = hash
		l.h.size = size
	})

	return l.h.value
}

func (l *genericResource) setOpenSource(openSource resource.OpenReadSeekCloser) {
	l.openSource = openSource
}

func (l *genericResource) setTargetPath(d dirFile) {
	// TODO1 add a setTargetName
	l.targetPath = d
	l.relPermalink = d
}

func (l *genericResource) Clone() resource.Resource {
	return l.clone()
}

func (l *genericResource) cloneTo(targetPath string) resource.Resource {
	c := l.clone()

	targetPath = helpers.ToSlashTrimLeading(targetPath)
	dir, file := path.Split(targetPath)
	c.targetPath = dirFile{dir: dir, file: file}

	return c

}

func (l *genericResource) Content() (any, error) {
	if err := l.initContent(); err != nil {
		return nil, err
	}

	return l.content, nil
}

func (r *genericResource) Err() resource.ResourceError {
	return nil
}

func (l *genericResource) Data() any {
	return l.data
}

func (l *genericResource) Key() string {
	// TODO1 consider repeating the section in the path segment.

	/*if l.fi != nil {
		// Create a key that at least shares the base folder with the source,
		// to facilitate effective cache busting on changes.
		meta := l.fi.Meta()
		p := meta.Path
		if p != "" {
			d, _ := filepath.Split(p)
			p = path.Join(d, l.targetPath.file)
			key := memcache.CleanKey(p)
			key = memcache.InsertKeyPathElements(key, meta.Lang)

			return key
		}
	}*/

	return memcache.CleanKey(l.RelPermalink())
}

func (l *genericResource) MediaType() media.Type {
	return l.mediaType
}

func (l *genericResource) setMediaType(mediaType media.Type) {
	l.mediaType = mediaType
}

func (l *genericResource) Name() string {
	return l.name
}

func (l *genericResource) Params() maps.Params {
	return l.params
}

func (l *genericResource) Permalink() string {
	return l.spec.PermalinkForBaseURL(l.relPermalink.path(), l.spec.BaseURL.HostURL())
}

func (l *genericResource) Publish() error {
	var err error
	l.publishInit.Do(func() {
		var fr hugio.ReadSeekCloser
		fr, err = l.ReadSeekCloser()
		if err != nil {
			return
		}
		defer fr.Close()

		var fw io.WriteCloser
		fw, err = helpers.OpenFilesForWriting(l.spec.BaseFs.PublishFs, l.targetPath.path()) // TODO1 multiple.
		if err != nil {
			return
		}
		defer fw.Close()

		_, err = io.Copy(fw, fr)
	})

	return err
}

func (l *genericResource) RelPermalink() string {
	return l.relPermalink.path()
}

func (l *genericResource) ResourceType() string {
	return l.resourceType
}

func (l *genericResource) String() string {
	return fmt.Sprintf("Resource(%s: %s)", l.resourceType, l.name)
}

// Path is stored with Unix style slashes.
func (l *genericResource) TargetPath() string {
	return l.targetPath.path()
}

func (l *genericResource) Title() string {
	return l.title
}

func (l *genericResource) createBasePath(rel string, isURL bool) string {
	return "TODO1"
	/*if isURL {
		return path.Join(tp.SubResourceBaseLink, rel)
	}

	// TODO(bep) path
	return path.Join(filepath.ToSlash(tp.SubResourceBaseTarget), rel)
	*/
}

func (l *genericResource) initContent() error {
	var err error
	l.contentInit.Do(func() {
		var r hugio.ReadSeekCloser
		r, err = l.ReadSeekCloser()
		if err != nil {
			return
		}
		defer r.Close()

		var b []byte
		b, err = ioutil.ReadAll(r)
		if err != nil {
			return
		}

		l.content = string(b)
	})

	return err
}

func (l *genericResource) setName(name string) {
	l.name = name
}

func (l *genericResource) getSpec() *Spec {
	return l.spec
}

func (l *genericResource) getTargetFilenames() []string {
	paths := l.relTargetPaths()
	for i, p := range paths {
		paths[i] = filepath.Clean(p)
	}
	return paths
}

func (l *genericResource) getTargetPathDirFile() dirFile {
	return l.targetPath
}

func (l *genericResource) setTitle(title string) {
	l.title = title
}

func (r *genericResource) tryTransformedFileCache(key string, u *transformationUpdate) io.ReadCloser {
	fi, f, meta, found := r.spec.ResourceCache.getFromFile(key)
	if !found {
		return nil
	}
	u.sourceFilename = &fi.Name
	mt, _ := r.spec.MediaTypes.GetByType(meta.MediaTypeV)
	u.mediaType = mt
	u.data = meta.MetaData
	u.targetPath = meta.Target
	return f
}

func (r *genericResource) mergeData(in map[string]any) {
	if len(in) == 0 {
		return
	}
	if r.data == nil {
		r.data = make(map[string]any)
	}
	for k, v := range in {
		if _, found := r.data[k]; !found {
			r.data[k] = v
		}
	}
}

func (rc *genericResource) cloneWithUpdates(u *transformationUpdate) (baseResource, error) {
	r := rc.clone()

	if u.content != nil {
		r.contentInit.Do(func() {
			r.content = *u.content
			r.openSource = func() (hugio.ReadSeekCloser, error) {
				return hugio.NewReadSeekerNoOpCloserFromString(r.content), nil
			}
		})
	}

	r.mediaType = u.mediaType

	if u.sourceFilename != nil {
		if u.sourceFs == nil {
			return nil, errors.New("sourceFs is nil")
		}
		r.setOpenSource(func() (hugio.ReadSeekCloser, error) {
			return u.sourceFs.Open(*u.sourceFilename)
		})
	} else if u.sourceFs != nil {
		return nil, errors.New("sourceFs is set without sourceFilename")
	}

	if u.targetPath == "" {
		return nil, errors.New("missing targetPath")
	}

	fpath, fname := path.Split(u.targetPath)
	r.setTargetPath(dirFile{dir: fpath, file: fname})

	r.mergeData(u.data)

	return r, nil
}

func (l genericResource) clone() *genericResource {
	l.resourceContent = &resourceContent{}
	return &l
}

// returns an opened file or nil if nothing to write (it may already be published).
func (l *genericResource) openDestinationsForWriting() (w io.WriteCloser, err error) {
	l.publishInit.Do(func() {
		targetFilenames := l.getTargetFilenames()
		var changedFilenames []string

		// Fast path:
		// This is a processed version of the original;
		// check if it already exists at the destination.
		for _, targetFilename := range targetFilenames {
			if _, err := l.getSpec().BaseFs.PublishFs.Stat(targetFilename); err == nil {
				continue
			}

			changedFilenames = append(changedFilenames, targetFilename)
		}

		if len(changedFilenames) == 0 {
			return
		}

		w, err = helpers.OpenFilesForWriting(l.getSpec().BaseFs.PublishFs, changedFilenames...)
	})

	return
}

func (r *genericResource) openPublishFileForWriting(relTargetPath string) (io.WriteCloser, error) {
	return helpers.OpenFilesForWriting(r.spec.BaseFs.PublishFs, r.relTargetPathsFor(relTargetPath)...)
}

func (l *genericResource) permalinkFor(target string) string {
	return l.spec.PermalinkForBaseURL(l.relPermalinkForRel(target, true), l.spec.BaseURL.HostURL())
}

func (l *genericResource) relPermalinkFor(target string) string {
	return l.relPermalinkForRel(target, false)
}

func (l *genericResource) relPermalinkForRel(rel string, isAbs bool) string {
	return l.spec.PathSpec.URLizeFilename(l.relTargetPathForRel(rel, false, isAbs, true))
}

func (l *genericResource) relTargetPathForRel(rel string, addBaseTargetPath, isAbs, isURL bool) string {
	panic("TODO1 remove this")
}

func (l *genericResource) relTargetPathForRelAndBasePath(rel, basePath string, isAbs, isURL bool) string {
	panic("TODO1 remove this")
}

func (l *genericResource) relTargetPaths() []string {
	return l.relTargetPathsForRel(l.TargetPath())
}

func (l *genericResource) relTargetPathsFor(target string) []string {
	return l.relTargetPathsForRel(target)
}

func (l *genericResource) relTargetPathsForRel(rel string) []string {
	panic("TODO1 remove this")
}

func (l *genericResource) updateParams(params map[string]any) {
	if l.params == nil {
		l.params = params
		return
	}

	// Sets the params not already set
	for k, v := range params {
		if _, found := l.params[k]; !found {
			l.params[k] = v
		}
	}
}

type targetPather interface {
	TargetPath() string
}

type permalinker interface {
	targetPather
	permalinkFor(target string) string
	relPermalinkFor(target string) string
	relTargetPaths() []string
	relTargetPathsFor(target string) []string
}

type resourceContent struct {
	content     string
	contentInit sync.Once

	publishInit sync.Once
}

type resourceHash struct {
	value string
	size  int
	init  sync.Once
}
