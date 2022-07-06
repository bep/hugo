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

package doctree

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"

	radix "github.com/armon/go-radix"
)

type LockType int

const (
	LockTypeNone LockType = iota
	LockTypeRead
	LockTypeWrite
)

func New[T any](cfg Config[T]) *Root[T] {
	if cfg.Shifter == nil {
		panic("Shifter is required")
	}

	if len(cfg.Dimensions) == 0 {
		panic("At least one dimension is required")
	}

	return &Root[T]{
		mu:         &sync.RWMutex{},
		dimensions: cfg.Dimensions,
		shifter:    cfg.Shifter,
		tree:       radix.New(),
	}
}

type (
	Config[T any] struct {
		// Dimensions configures the dimensions in the tree (e.g. role, language).
		// It cannot be changed once set.
		Dimensions Dimensions

		// Shifter handles tree transformations.
		Shifter Shifter[T]
	}

	Dimensions []int

	// Shifter handles tree transformations.
	Shifter[T any] interface {
		// Shift shifts T into the given dimensions.
		// It may return a zero value and false.
		Shift(T, []int) (T, bool)

		// All returns all values of T in all dimensions.
		All(n T) []T

		// Dimension gets all values of node n in dimension d.
		Dimension(n T, d int) []T

		// Insert inserts new into the correct dimension.
		// It may replace old.
		// It returns a T (can be the same as old) and a bool indicating if the insert was successful.
		Insert(old, new T) (T, bool)
	}
)

// Dimension holds information about where an item's location is a the tree's dimensions.
type Dimension struct {
	Name      string
	Dimension int
	Index     int
	Size      int
}

func (d Dimension) IsZero() bool {
	return d.Name == ""
}

type Event[T any] struct {
	Name            string
	Path            string
	Source          T
	stopPropagation bool
}

type eventHandlers[T any] map[string][]func(*Event[T])

type WalkContext[T any] struct {
	Context context.Context

	data     *Tree[any]
	dataInit sync.Once

	eventHandlers eventHandlers[T]
	events        []*Event[T]
}

func (ctx *WalkContext[T]) Data() *Tree[any] {
	ctx.dataInit.Do(func() {
		ctx.data = &Tree[any]{
			tree: radix.New(),
		}
	})
	return ctx.data
}

// AddEventListener adds an event listener to the tree.
// Note that the handler func may not add listeners.
func (ctx *WalkContext[T]) AddEventListener(event, path string, handler func(*Event[T])) {
	if ctx.eventHandlers[event] == nil {
		ctx.eventHandlers[event] = make([]func(*Event[T]), 0)
	}

	// We want to match all above the path, so we need to exclude any similar named siblings.
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	ctx.eventHandlers[event] = append(
		ctx.eventHandlers[event], func(e *Event[T]) {
			// Propagate events up the tree only.
			if strings.HasPrefix(e.Path, path) {
				handler(e)
			}
		},
	)
}

func (ctx *WalkContext[T]) SendEvent(event *Event[T]) {
	ctx.events = append(ctx.events, event)
}

func (ctx *WalkContext[T]) handleEvents() {
	for len(ctx.events) > 0 {
		event := ctx.events[0]
		ctx.events = ctx.events[1:]

		// Loop the event handlers in reverse order so
		// that events created by the handlers themselves will
		// be picked up further up the tree.
		for i := len(ctx.eventHandlers[event.Name]) - 1; i >= 0; i-- {
			ctx.eventHandlers[event.Name][i](event)
			if event.stopPropagation {
				break
			}
		}
	}

}

func (e *Event[T]) StopPropagation() {
	e.stopPropagation = true
}

// MutableTree is a tree that can be modified.
type MutableTree interface {
	Delete(key string)
	DeletePrefix(prefix string) int
	Lock(writable bool) (commit func())
}

var _ MutableTrees = MutableTrees{}

type MutableTrees []MutableTree

func (t MutableTrees) Delete(key string) {
	for _, tree := range t {
		tree.Delete(key)
	}
}

func (t MutableTrees) DeletePrefix(prefix string) int {
	var count int
	for _, tree := range t {
		count += tree.DeletePrefix(prefix)
	}
	return count
}

func (t MutableTrees) Lock(writable bool) (commit func()) {
	commits := make([]func(), len(t))
	for i, tree := range t {
		commits[i] = tree.Lock(writable)
	}
	return func() {
		for _, commit := range commits {
			commit()
		}
	}
}

type Root[T any] struct {
	tree *radix.Tree

	// E.g. [language, role].
	dimensions Dimensions
	shifter    Shifter[T]

	mu *sync.RWMutex
}

func (t *Root[T]) String() string {
	return fmt.Sprintf("Root{%v}", t.dimensions)
}

func (t *Root[T]) PrintDebug(prefix string) {
	t.Walk(context.Background(), WalkConfig[T]{
		Prefix: prefix,
		Callback: func(ctx *WalkContext[T], s string, t T) (bool, error) {
			fmt.Println(s)
			return false, nil
		},
	})
}

func (t *Root[T]) Len() int {
	return t.tree.Len()
}

// Shape the tree for dimension d to value v.
func (t *Root[T]) Shape(d, v int) *Root[T] {
	x := t.clone()
	x.dimensions[d] = v
	return x
}

func (t *Root[T]) DeletePrefix(prefix string) int {
	return t.tree.DeletePrefix(prefix)
}

func (t *Root[T]) Delete(key string) {
	t.tree.Delete(key)
}

// Lock locks the data store for read or read/write access until commit is invoked.
// Note that Root is not thread-safe outside of this transaction construct.
func (t *Root[T]) Lock(writable bool) (commit func()) {
	if writable {
		t.mu.Lock()
	} else {
		t.mu.RLock()
	}
	return func() {
		if writable {
			t.mu.Unlock()
		} else {
			t.mu.RUnlock()
		}
	}
}

// Increment the value of dimension d by 1.
func (t *Root[T]) Increment(d int) *Root[T] {
	return t.Shape(d, t.dimensions[d]+1)
}

func (r *Root[T]) InsertWithLock(s string, v T) (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Insert(s, v)
}

func (r *Root[T]) Insert(s string, v T) (T, bool) {
	s = cleanKey(s)
	mustValidateKey(s)
	vv, ok := r.tree.Get(s)

	if ok {
		v, ok = r.shifter.Insert(vv.(T), v)
		if !ok {
			return v, false
		}
	}

	//fmt.Printf("Insert2 %q -> %T\n", s, v)

	r.tree.Insert(s, v)
	return v, true
}

func (r *Root[T]) Has(s string) bool {
	_, ok := r.get(s)
	return ok
}

func (r *Root[T]) Get(s string) T {
	t, _ := r.get(s)
	return t
}

func (r *Root[T]) get(s string) (T, bool) {
	s = cleanKey(s)
	v, ok := r.tree.Get(s)
	if !ok {
		var t T
		return t, false
	}
	t, ok := r.shift(v.(T))
	return t, ok
}

func (r *Root[T]) GetRaw(s string) (T, bool) {
	v, ok := r.tree.Get(s)
	if !ok {
		var t T
		return t, false
	}
	return v.(T), true
}

func (r *Root[T]) GetAll(s string) []T {
	s = cleanKey(s)
	v, ok := r.tree.Get(s)
	if !ok {
		return nil
	}
	return r.shifter.All(v.(T))
}

func (r *Root[T]) GetDimension(s string, d int) []T {
	s = cleanKey(s)
	v, ok := r.tree.Get(s)
	if !ok {
		return nil
	}
	return r.shifter.Dimension(v.(T), d)
}

// LongestPrefix finds the longest prefix of s that exists in the tree that also matches the predicate (if set).
func (r *Root[T]) LongestPrefix(s string, predicate func(v T) bool) (string, T) {
	for {
		longestPrefix, v, found := r.tree.LongestPrefix(s)

		if found {
			if t, ok := r.shift(v.(T)); ok && (predicate == nil || predicate(t)) {
				return longestPrefix, t
			}
		}

		if s == "" || s == "/" {
			var t T
			return "", t
		}

		// Walk up to find a node in the correct dimension.
		s = path.Dir(s)

	}
}

// LongestPrefixAll returns the longest prefix considering all tree dimensions.
func (r *Root[T]) LongestPrefixAll(s string) (string, bool) {
	s, _, found := r.tree.LongestPrefix(s)
	return s, found
}

type WalkConfig[T any] struct {
	// Optional prefix filter.
	Prefix string

	// Callback will be called for each node in the tree.
	// If the callback returns true, the walk will stop.
	Callback func(ctx *WalkContext[T], s string, t T) (bool, error)

	// Enable read or write locking if needed.
	LockType LockType

	// When set, no dimension shifting will be performed.
	NoShift bool

	// Used in development only.
	Debug bool
}

func (r *Root[T]) Walk(ctx context.Context, cfg WalkConfig[T]) error {
	if cfg.LockType > LockTypeNone {
		commit := r.Lock(cfg.LockType == LockTypeWrite)
		defer commit()
	}
	wctx := r.newWalkContext(ctx)

	var err error
	fn := func(s string, v interface{}) bool {
		if cfg.Debug {
			fmt.Println(s, "=>", v)
		}

		var t T

		if cfg.NoShift {
			t = v.(T)
		} else {
			var ok bool
			t, ok = r.shift(v.(T))
			if !ok {
				return false
			}
		}

		var terminate bool
		terminate, err = cfg.Callback(wctx, s, t)
		return terminate || err != nil
	}

	if cfg.Prefix != "" {
		r.tree.WalkPrefix(cfg.Prefix, fn)
	} else {
		r.tree.Walk(fn)
	}

	if err != nil {
		return err
	}

	wctx.handleEvents()

	return nil
}

func (r *Root[T]) newWalkContext(ctx context.Context) *WalkContext[T] {
	return &WalkContext[T]{
		eventHandlers: make(eventHandlers[T]),
		Context:       ctx,
	}
}

func (t Root[T]) clone() *Root[T] {
	dimensions := make(Dimensions, len(t.dimensions))
	copy(dimensions, t.dimensions)
	t.dimensions = dimensions

	return &t
}

func (r *Root[T]) shift(t T) (T, bool) {
	return r.shifter.Shift(t, r.dimensions)
}

func cleanKey(key string) string {
	if key == "/" {
		// The path to the home page is logically "/",
		// but for technical reasons, it's stored as "".
		// This allows us to treat the home page as a section,
		// and a prefix search for "/" will return the home page's descendants.
		return ""
	}
	return key
}

func mustValidateKey(key string) {
	if err := ValidateKey(key); err != nil {
		panic(err)
	}
}

// ValidateKey returns an error if the key is not valid.
func ValidateKey(key string) error {
	if key == "" {
		// Root node.
		return nil
	}

	if len(key) < 2 {
		return fmt.Errorf("too short key: %q", key)
	}

	if key[0] != '/' {
		return fmt.Errorf("key must start with '/': %q", key)
	}

	if key[len(key)-1] == '/' {
		return fmt.Errorf("key must not end with '/': %q", key)
	}

	return nil
}

type Tree[T any] struct {
	mu   sync.RWMutex
	tree *radix.Tree
}

func (tree *Tree[T]) Get(s string) T {
	tree.mu.RLock()
	defer tree.mu.RUnlock()

	if v, ok := tree.tree.Get(s); ok {
		return v.(T)
	}
	var t T
	return t
}

func (tree *Tree[T]) LongestPrefix(s string) (string, T) {
	tree.mu.RLock()
	defer tree.mu.RUnlock()

	if s, v, ok := tree.tree.LongestPrefix(s); ok {
		return s, v.(T)
	}
	var t T
	return "", t
}

func (tree *Tree[T]) Insert(s string, v T) T {
	tree.mu.Lock()
	defer tree.mu.Unlock()

	tree.tree.Insert(s, v)
	return v
}
