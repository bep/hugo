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

package memcache

import (
	"context"
	"math"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bep/lazycache"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/resources/resource"
)

const minMaxSize = 10

type Options struct {
	CheckInterval time.Duration
	MaxSize       int
	MinMaxSize    int
	Running       bool
}

type OptionsPartition struct {
	ClearWhen ClearWhen

	// Weight is a number between 1 and 100 that indicates how, in general, how big this partition may get.
	Weight int
}

func (o OptionsPartition) WeightFraction() float64 {
	return float64(o.Weight) / 100
}

func (o OptionsPartition) CalculateMaxSize(maxSizePerPartition int) int {
	return int(math.Floor(float64(maxSizePerPartition) * o.WeightFraction()))
}

type Cache struct {
	mu sync.RWMutex

	partitions map[string]PartitionManager
	opts       Options

	stats    *stats
	stopOnce sync.Once
	stop     func()
}

func (c *Cache) ClearOn(when ClearWhen, changeset ...identity.Identity) {
	if when == 0 {
		panic("invalid ClearWhen")
	}

	for _, g := range c.partitions {
		g.clearOn(when, changeset...)
	}

}

func calculateMaxSizePerPartition(maxItemsTotal, totalWeightQuantity, numPartitions int) int {
	if numPartitions == 0 {
		panic("numPartitions must be > 0")
	}
	if totalWeightQuantity == 0 {
		panic("totalWeightQuantity must be > 0")
	}

	avgWeight := float64(totalWeightQuantity) / float64(numPartitions)
	return int(math.Floor(float64(maxItemsTotal) / float64(numPartitions) * (100.0 / avgWeight)))
}

func (c *Cache) Stop() {
	c.stopOnce.Do(func() {
		c.stop()
	})
}

func (c *Cache) adjustCurrentMaxSize() {
	if len(c.partitions) == 0 {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := c.stats
	s.memstatsCurrent = m
	if s.availableMemory >= s.memstatsCurrent.Alloc {
		if s.adjustmentFactor <= 1.0 {
			s.adjustmentFactor += 0.1
		}
	} else {
		// We're low on memory.
		s.adjustmentFactor -= 0.4
	}

	if s.adjustmentFactor < 0.2 {
		s.adjustmentFactor = 0.2
	}

	if !s.adjustCurrentMaxSize() {
		return
	}

	//fmt.Printf("\n\nAvailable = %v\nAlloc = %v\nTotalAlloc = %v\nSys = %v\nNumGC = %v\nMaxSize = %d\n\n", helpers.FormatByteCount(s.availableMemory), helpers.FormatByteCount(m.Alloc), helpers.FormatByteCount(m.TotalAlloc), helpers.FormatByteCount(m.Sys), m.NumGC, c.stats.currentMaxSize)

	totalWeight := 0
	for _, pm := range c.partitions {
		totalWeight += pm.getOptions().Weight
	}

	maxSizePerPartition := calculateMaxSizePerPartition(c.stats.currentMaxSize, totalWeight, len(c.partitions))

	//fmt.Println("SCALE", s.adjustmentFactor, maxSizePerPartition)

	evicted := 0
	for _, p := range c.partitions {
		evicted += p.adjustMaxSize(p.getOptions().CalculateMaxSize(maxSizePerPartition))
	}

	// TODO1
	//fmt.Println("Evicted", evicted, "items from cache")

}

func (c *Cache) start() func() {
	ticker := time.NewTicker(c.opts.CheckInterval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				c.adjustCurrentMaxSize()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(quit)
	}
}

func GetOrCreatePartition[K comparable, V any](c *Cache, name string, opts OptionsPartition) *Partition[K, V] {
	if c == nil {
		panic("nil Cache")
	}
	if opts.Weight < 1 || opts.Weight > 100 {
		panic("invalid Weight, must be between 1 and 100")
	}

	c.mu.RLock()
	p, found := c.partitions[name]
	c.mu.RUnlock()
	if found {
		return p.(*Partition[K, V])
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check.
	p, found = c.partitions[name]
	if found {
		return p.(*Partition[K, V])
	}

	// At this point, we don't now the the number of partitions or their configuration, but
	// this will be re-adjusted later.
	const numberOfPartitionsEstimate = 10
	maxSize := opts.CalculateMaxSize(c.opts.MaxSize / numberOfPartitionsEstimate)

	// Create a new partition and cache it.
	partition := &Partition[K, V]{
		c:       lazycache.New[K, V](lazycache.Options{MaxEntries: maxSize}),
		maxSize: maxSize,
		opts:    opts,
	}
	c.partitions[name] = partition

	return partition
}

func New(opts Options) *Cache {
	if opts.CheckInterval == 0 {
		opts.CheckInterval = time.Second * 2
	}

	if opts.MaxSize == 0 {
		opts.MaxSize = 100000
	}

	if opts.MinMaxSize == 0 {
		opts.MinMaxSize = 30
	}

	stats := &stats{
		configuredMaxSize:    opts.MaxSize,
		configuredMinMaxSize: opts.MinMaxSize,
		currentMaxSize:       opts.MaxSize,
		availableMemory:      config.GetMemoryLimit(),
	}

	c := &Cache{
		partitions: make(map[string]PartitionManager),
		opts:       opts,
		stats:      stats,
	}

	c.stop = c.start()

	return c
}

type Partition[K comparable, V any] struct {
	c *lazycache.Cache[K, V]

	opts OptionsPartition

	maxSize int
}

func (p *Partition[K, V]) GetOrCreate(ctx context.Context, key K, create func(key K) (V, error)) (V, error) {
	v, _, err := p.c.GetOrCreate(key, create)
	return v, err
	//g.c.trackDependencyIfRunning(ctx, v)

}

func (p *Partition[K, V]) clearOn(when ClearWhen, changeset ...identity.Identity) {
	opts := p.getOptions()
	if opts.ClearWhen == ClearNever {
		return
	}

	if opts.ClearWhen == when {
		// Clear all.
		p.Clear()
		return
	}

	shouldDelete := func(key K, v V) bool {
		// We always clear elements marked as stale.
		if resource.IsStaleAny(v) {
			return true
		}

		// Now check if this entry has changed based on the changeset
		// based on filesystem events.
		if len(changeset) == 0 {
			// Nothing changed.
			return false
		}

		var probablyDependent bool
		identity.WalkIdentities(v, false, func(level int, id2 identity.Identity) bool {
			for _, id := range changeset {
				if !identity.IsNotDependent(id2, id) {
					// It's probably dependent, evict from cache.
					probablyDependent = true
					return true
				}
			}
			return false
		})

		return probablyDependent
	}

	// First pass.
	p.c.DeleteFunc(func(key K, v V) bool {
		if shouldDelete(key, v) {
			resource.MarkStale(v)
			return true
		}
		return false
	})

	// Second pass: Clear all entries marked as stale in the first.
	p.c.DeleteFunc(func(key K, v V) bool {
		return resource.IsStaleAny(v)
	})

}

// adjustMaxSize adjusts the max size of the and returns the number of items evicted.
func (p *Partition[K, V]) adjustMaxSize(newMaxSize int) int {
	if newMaxSize < minMaxSize {
		newMaxSize = minMaxSize
	}
	p.maxSize = newMaxSize
	//fmt.Println("Adjusting max size of partition from", oldMaxSize, "to", newMaxSize)
	return p.c.Resize(newMaxSize)
}

func (p *Partition[K, V]) getMaxSize() int {
	return p.maxSize
}

func (p *Partition[K, V]) getOptions() OptionsPartition {
	return p.opts
}

func (p *Partition[K, V]) Clear() {
	p.c.DeleteFunc(func(key K, v V) bool {
		return true
	})
}

func (p *Partition[K, V]) Get(ctx context.Context, key K) (V, bool) {
	return p.c.Get(key)
	// 	g.c.trackDependencyIfRunning(ctx, v)
}

type PartitionManager interface {
	adjustMaxSize(addend int) int
	getMaxSize() int
	getOptions() OptionsPartition
	clearOn(when ClearWhen, changeset ...identity.Identity)
}

const (
	ClearOnRebuild ClearWhen = iota + 1
	ClearOnChange
	ClearNever
)

type ClearWhen int

type stats struct {
	memstatsCurrent      runtime.MemStats
	configuredMaxSize    int
	configuredMinMaxSize int
	currentMaxSize       int
	availableMemory      uint64

	adjustmentFactor float64
}

func (s *stats) adjustCurrentMaxSize() bool {
	newCurrentMaxSize := int(math.Floor(float64(s.configuredMaxSize) * s.adjustmentFactor))

	if newCurrentMaxSize < s.configuredMinMaxSize {
		newCurrentMaxSize = int(s.configuredMinMaxSize)
	}
	changed := newCurrentMaxSize != s.currentMaxSize
	s.currentMaxSize = newCurrentMaxSize
	return changed

}

const (
	cacheVirtualRoot = "_root/"
)

const unknownExtension = "unkn"

// CleanKey turns s into a format suitable for a cache key for this package.
// The key will be a Unix-styled path without any leading slash.
// If the input string does not contain any slash, a root will be prepended.
// If the input string does not contain any ".", a dummy file suffix will be appended.
// These are to make sure that they can effectively partake in the "cache cleaning"
// strategy used in server mode.
func CleanKey(s string) string {
	s = path.Clean(helpers.ToSlashTrimLeading(s))
	if !strings.ContainsRune(s, '/') {
		s = cacheVirtualRoot + s
	}
	if !strings.ContainsRune(s, '.') {
		s += "." + unknownExtension
	}

	return s
}

// TODO1: Usage.
var (

	// Consider a change in files matching this expression a "JS change".
	isJSFileRe = regexp.MustCompile(`\.(js|ts|jsx|tsx)`)

	// Consider a change in files matching this expression a "CSS change".
	isCSSFileRe = regexp.MustCompile(`\.(css|scss|sass)`)

	// These config files are tightly related to CSS editing, so consider
	// a change to any of them a "CSS change".
	isCSSConfigRe = regexp.MustCompile(`(postcss|tailwind)\.config\.js`)
)

// Helpers to help eviction of related media types.
func isCSSType(m media.Type) bool {
	tp := m.Type()
	return tp == media.CSSType.Type() || tp == media.SASSType.Type() || tp == media.SCSSType.Type()
}

func isJSType(m media.Type) bool {
	tp := m.Type()
	return tp == media.JavascriptType.Type() || tp == media.TypeScriptType.Type() || tp == media.JSXType.Type() || tp == media.TSXType.Type()
}
