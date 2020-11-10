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

<<<<<<< HEAD
	// Either resource.Resource or resource.Resources.
	cache map[string]any
=======
	// Memory cache with either
	// resource.Resource or resource.Resources.
	cache memcache.Getter
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

	fileCache *filecache.Cache
}

func newResourceCache(rs *Spec, memCache *memcache.Cache) *ResourceCache {
	return &ResourceCache{
		rs:        rs,
		fileCache: rs.FileCaches.AssetsCache(),
<<<<<<< HEAD
		cache:     make(map[string]any),
		nlocker:   locker.NewLocker(),
	}
}

func (c *ResourceCache) clear() {
	c.Lock()
	defer c.Unlock()

	c.cache = make(map[string]any)
	c.nlocker = locker.NewLocker()
}

func (c *ResourceCache) Contains(key string) bool {
	key = c.cleanKey(filepath.ToSlash(key))
	_, found := c.get(key)
	return found
}

func (c *ResourceCache) cleanKey(key string) string {
	return strings.TrimPrefix(path.Clean(strings.ToLower(key)), "/")
}

func (c *ResourceCache) get(key string) (any, bool) {
	c.RLock()
	defer c.RUnlock()
	r, found := c.cache[key]
	return r, found
}

func (c *ResourceCache) GetOrCreate(key string, f func() (resource.Resource, error)) (resource.Resource, error) {
	r, err := c.getOrCreate(key, func() (any, error) { return f() })
=======
		cache:     memCache.GetOrCreatePartition("resources", memcache.ClearOnChange),
	}
}

func (c *ResourceCache) Get(ctx context.Context, key string) (resource.Resource, error) {
	// TODO, maybe also look in resources and rename it to something ala Find?
	v, err := c.cache.Get(ctx, key)
	if v == nil || err != nil {
		return nil, err
	}
	return v.(resource.Resource), nil
}

func (c *ResourceCache) GetOrCreate(ctx context.Context, key string, clearWhen memcache.ClearWhen, f func() (resource.Resource, error)) (resource.Resource, error) {
	r, err := c.cache.GetOrCreate(ctx, key, func() memcache.Entry {
		r, err := f()
		return memcache.Entry{
			Value:     r,
			Err:       err,
			ClearWhen: clearWhen,
		}
	})
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	if r == nil || err != nil {
		return nil, err
	}
	return r.(resource.Resource), nil
}

func (c *ResourceCache) GetOrCreateResources(key string, f func() (resource.Resources, error)) (resource.Resources, error) {
<<<<<<< HEAD
	r, err := c.getOrCreate(key, func() (any, error) { return f() })
=======
	r, err := c.cache.GetOrCreate(context.TODO(), key, func() memcache.Entry {
		r, err := f()
		return memcache.Entry{
			Value:     r,
			Err:       err,
			ClearWhen: memcache.ClearOnChange,
		}
	})
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	if r == nil || err != nil {
		return nil, err
	}
	return r.(resource.Resources), nil
}

<<<<<<< HEAD
func (c *ResourceCache) getOrCreate(key string, f func() (any, error)) (any, error) {
	key = c.cleanKey(key)
	// First check in-memory cache.
	r, found := c.get(key)
	if found {
		return r, nil
	}
	// This is a potentially long running operation, so get a named lock.
	c.nlocker.Lock(key)

	// Double check in-memory cache.
	r, found = c.get(key)
	if found {
		c.nlocker.Unlock(key)
		return r, nil
	}

	defer c.nlocker.Unlock(key)

	r, err := f()
	if err != nil {
		return nil, err
	}

	c.set(key, r)

	return r, nil
}

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
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
<<<<<<< HEAD

func (c *ResourceCache) set(key string, r any) {
	c.Lock()
	defer c.Unlock()
	c.cache[key] = r
}

func (c *ResourceCache) DeletePartitions(partitions ...string) {
	partitionsSet := map[string]bool{
		// Always clear out the resources not matching any partition.
		"other": true,
	}
	for _, p := range partitions {
		partitionsSet[p] = true
	}

	if partitionsSet[CACHE_CLEAR_ALL] {
		c.clear()
		return
	}

	c.Lock()
	defer c.Unlock()

	for k := range c.cache {
		clear := false
		for p := range partitionsSet {
			if strings.Contains(k, p) {
				// There will be some false positive, but that's fine.
				clear = true
				break
			}
		}

		if clear {
			delete(c.cache, k)
		}
	}
}

func (c *ResourceCache) DeleteMatches(re *regexp.Regexp) {
	c.Lock()
	defer c.Unlock()

	for k := range c.cache {
		if re.MatchString(k) {
			delete(c.cache, k)
		}
	}
}
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
