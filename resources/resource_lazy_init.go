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
	"sync"

	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/resources/resource"
)

var _ resource.Resource = (*ResourceLazyInit)(nil)

// ResourceLazyInit is a Resource that is lazy initialized.
// We do that to
// 1) Avoid creating resources that are not used, and
// 2) Delay creation of resources until we know how they should look (e.g. Page Resources).
type ResourceLazyInit struct {
	Path        *paths.Path
	OpenContent resource.OpenReadSeekCloser

	delegate resource.Resource

	init    sync.Once
	isInit  bool
	initErr error
}

// Init will initialize the resource if it is not already initialized.
func (r *ResourceLazyInit) Init(initFunc func(p *paths.Path, openContent resource.OpenReadSeekCloser) (resource.Resource, error)) (resource.Resource, error) {
	r.init.Do(func() {
		r.delegate, r.initErr = initFunc(r.Path, r.OpenContent)
		r.isInit = true
	})
	return r.delegate, r.initErr
}

func (r *ResourceLazyInit) checkInit() {
	if !r.isInit {
		panic("Resource not initialized, must call Init() first")
	}
	if r.initErr != nil {
		panic("Resource initialization failed: " + r.initErr.Error())
	}
}

func (r *ResourceLazyInit) ResourceType() string {
	r.checkInit()
	return r.delegate.ResourceType()
}

func (r *ResourceLazyInit) MediaType() media.Type {
	r.checkInit()
	return r.delegate.MediaType()
}

func (r *ResourceLazyInit) Permalink() string {
	r.checkInit()
	return r.delegate.Permalink()
}

func (r *ResourceLazyInit) RelPermalink() string {
	r.checkInit()
	return r.delegate.RelPermalink()
}

func (r *ResourceLazyInit) Name() string {
	r.checkInit()
	return r.delegate.Name()
}

func (r *ResourceLazyInit) Title() string {
	r.checkInit()
	return r.delegate.Title()
}

func (r *ResourceLazyInit) Params() maps.Params {
	r.checkInit()
	return r.delegate.Params()
}

func (r *ResourceLazyInit) Data() any {
	r.checkInit()
	return r.delegate.Data()
}

func (r *ResourceLazyInit) Err() resource.ResourceError {
	r.checkInit()
	return r.delegate.Err()
}
