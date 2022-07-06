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
	"sync"
)

type NodeGetter[N any] interface {
	GetNode() N
}

type LazySlice[S comparable, N any] struct {
	items []lazySliceUnit[S, N]
}

type lazySliceUnit[S, N any] struct {
	source S
	value  N
	init   sync.Once
}

func (l lazySliceUnit[S, N]) GetNode() N {
	return l.value
}

func NewLazySlice[S comparable, N any](size int) *LazySlice[S, N] {
	return &LazySlice[S, N]{
		items: make([]lazySliceUnit[S, N], size),
	}
}

// TODO1 check this NodeGetter construct.
func (s *LazySlice[S, N]) GetNode() N {
	var n N
	return n
}

func (s *LazySlice[S, N]) HasSource(idx int) bool {
	var zeros S
	return s.items[idx].source != zeros
}

func (s *LazySlice[S, N]) GetSource(idx int) (S, bool) {
	var zeros S
	item := s.items[idx]
	return item.source, item.source != zeros
}

func (s *LazySlice[S, N]) SetSource(idx int, source S) {
	s.items[idx].source = source
}

func (s *LazySlice[S, N]) GetOrCreate(sourceIdx, targetIdx int, create func(S) (N, error)) (NodeGetter[N], error) {
	var initErr error
	sourceUnit := &s.items[sourceIdx]
	targetUnit := &s.items[targetIdx]
	targetUnit.init.Do(func() {
		var zeros S
		source := sourceUnit.source
		if source == zeros {
			source = s.items[0].source
		}

		if source == zeros {
			return
		}

		targetUnit.value, initErr = create(source)

	})
	return targetUnit, initErr
}
