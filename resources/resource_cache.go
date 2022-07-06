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
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/gohugoio/hugo/cache/memcache"

	"github.com/gohugoio/hugo/resources/resource"

	"github.com/gohugoio/hugo/cache/filecache"
)

type ResourceCache struct {
	rs *Spec

	sync.RWMutex

	cacheResource               *memcache.Partition[string, resource.Resource]
	cacheResources              *memcache.Partition[string, resource.Resources]
	cacheResourceTransformation *memcache.Partition[string, *resourceAdapterInner]

	fileCache *filecache.Cache
}

func newResourceCache(rs *Spec, memCache *memcache.Cache) *ResourceCache {
	return &ResourceCache{
		rs:        rs,
		fileCache: rs.FileCaches.AssetsCache(),
		cacheResource: memcache.GetOrCreatePartition[string, resource.Resource](
			memCache,
			"resource",
			memcache.OptionsPartition{ClearWhen: memcache.ClearOnChange, Weight: 40},
		),
		cacheResources: memcache.GetOrCreatePartition[string, resource.Resources](
			memCache,
			"resources",
			memcache.OptionsPartition{ClearWhen: memcache.ClearOnChange, Weight: 40},
		),
		cacheResourceTransformation: memcache.GetOrCreatePartition[string, *resourceAdapterInner](
			memCache,
			"resourceTransformation",
			memcache.OptionsPartition{ClearWhen: memcache.ClearOnChange, Weight: 40},
		),
	}
}

func (c *ResourceCache) Get(ctx context.Context, key string) resource.Resource {
	v, _ := c.cacheResource.Get(ctx, key)
	return v
}

func (c *ResourceCache) GetOrCreate(ctx context.Context, key string, f func() (resource.Resource, error)) (resource.Resource, error) {
	return c.cacheResource.GetOrCreate(ctx, key, func(key string) (resource.Resource, error) {
		return f()
	})
}

func (c *ResourceCache) GetOrCreateResources(ctx context.Context, key string, f func() (resource.Resources, error)) (resource.Resources, error) {
	return c.cacheResources.GetOrCreate(ctx, key, func(key string) (resource.Resources, error) {
		return f()
	})
}

func (c *ResourceCache) getFilenames(key string) (string, string) {
	filenameMeta := key + ".json"
	filenameContent := key + ".content"

	return filenameMeta, filenameContent
}

func (c *ResourceCache) getFromFile(key string) (filecache.ItemInfo, io.ReadCloser, transformedResourceMetadata, bool) {
	c.RLock()
	defer c.RUnlock()

	var meta transformedResourceMetadata
	filenameMeta, filenameContent := c.getFilenames(key)

	_, jsonContent, _ := c.fileCache.GetBytes(filenameMeta)
	if jsonContent == nil {
		return filecache.ItemInfo{}, nil, meta, false
	}

	if err := json.Unmarshal(jsonContent, &meta); err != nil {
		return filecache.ItemInfo{}, nil, meta, false
	}

	fi, rc, _ := c.fileCache.Get(filenameContent)

	return fi, rc, meta, rc != nil
}

// writeMeta writes the metadata to file and returns a writer for the content part.
func (c *ResourceCache) writeMeta(key string, meta transformedResourceMetadata) (filecache.ItemInfo, io.WriteCloser, error) {
	filenameMeta, filenameContent := c.getFilenames(key)
	raw, err := json.Marshal(meta)
	if err != nil {
		return filecache.ItemInfo{}, nil, err
	}

	_, fm, err := c.fileCache.WriteCloser(filenameMeta)
	if err != nil {
		return filecache.ItemInfo{}, nil, err
	}
	defer fm.Close()

	if _, err := fm.Write(raw); err != nil {
		return filecache.ItemInfo{}, nil, err
	}

	fi, fc, err := c.fileCache.WriteCloser(filenameContent)

	return fi, fc, err
}
