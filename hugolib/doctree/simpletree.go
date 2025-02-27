// Copyright 2024 The Hugo Authors. All rights reserved.
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
	"sync"

	radix "github.com/armon/go-radix"
)

// Tree is a radix tree that holds T.
type Tree[T any] interface {
	Get(s string) T
	LongestPrefix(s string) (string, T)
	Insert(s string, v T) T
	WalkPrefix(lockType LockType, s string, f func(s string, v T) (bool, error)) error
	WalkPath(lockType LockType, s string, f func(s string, v T) (bool, error)) error
}

// NewSimpleTree creates a new SimpleTree.
func NewSimpleTree[T any]() *SimpleTree[T] {
	return &SimpleTree[T]{tree: radix.New(), mu: new(sync.RWMutex)}
}

// SimpleTree is a thread safe radix tree that holds T.
type SimpleTree[T any] struct {
	mu     *sync.RWMutex
	noLock bool
	tree   *radix.Tree
	zero   T
}

func (tree *SimpleTree[T]) readLock() func() {
	if tree.noLock {
		return func() {}
	}
	tree.mu.RLock()
	return tree.mu.RUnlock
}

func (tree *SimpleTree[T]) writeLock() func() {
	if tree.noLock {
		return func() {}
	}
	tree.mu.Lock()
	return tree.mu.Unlock
}

func (tree *SimpleTree[T]) Get(s string) T {
	unlock := tree.readLock()
	defer unlock()

	if v, ok := tree.tree.Get(s); ok {
		return v.(T)
	}
	return tree.zero
}

func (tree *SimpleTree[T]) LongestPrefix(s string) (string, T) {
	unlock := tree.readLock()
	defer unlock()

	if s, v, ok := tree.tree.LongestPrefix(s); ok {
		return s, v.(T)
	}
	return "", tree.zero
}

func (tree *SimpleTree[T]) Insert(s string, v T) T {
	unlock := tree.writeLock()
	defer unlock()

	tree.tree.Insert(s, v)
	return v
}

func (tree *SimpleTree[T]) Lock(lockType LockType) func() {
	switch lockType {
	case LockTypeNone:
		return func() {}
	case LockTypeRead:
		tree.mu.RLock()
		return tree.mu.RUnlock
	case LockTypeWrite:
		tree.mu.Lock()
		return tree.mu.Unlock
	}
	return func() {}
}

func (tree SimpleTree[T]) LockTree(lockType LockType) (Tree[T], func()) {
	unlock := tree.Lock(lockType)
	tree.noLock = true
	return &tree, unlock // create a copy of tree with the noLock flag set to true.
}

func (tree *SimpleTree[T]) WalkPrefix(lockType LockType, s string, f func(s string, v T) (bool, error)) error {
	commit := tree.Lock(lockType)
	defer commit()
	var err error
	tree.tree.WalkPrefix(s, func(s string, v any) bool {
		var b bool
		b, err = f(s, v.(T))
		if err != nil {
			return true
		}
		return b
	})

	return err
}

func (tree *SimpleTree[T]) WalkPath(lockType LockType, s string, f func(s string, v T) (bool, error)) error {
	commit := tree.Lock(lockType)
	defer commit()
	var err error
	tree.tree.WalkPath(s, func(s string, v any) bool {
		var b bool
		b, err = f(s, v.(T))
		if err != nil {
			return true
		}
		return b
	})

	return err
}
