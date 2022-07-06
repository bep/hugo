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
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestCache(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	cache := New(Options{})
	opts := OptionsPartition{Weight: 30}

	c.Assert(cache, qt.Not(qt.IsNil))

	p1 := GetOrCreatePartition[string, testItem](cache, "a", opts)
	c.Assert(p1, qt.Not(qt.IsNil))

	p2 := GetOrCreatePartition[string, testItem](cache, "a", opts)
	c.Assert(p2, qt.Equals, p1)

	p3 := GetOrCreatePartition[string, testItem](cache, "b", opts)
	c.Assert(p3, qt.Not(qt.IsNil))
	c.Assert(p3, qt.Not(qt.Equals), p1)

}

type testItem struct {
}

func TestCalculateMaxSizePerPartition(t *testing.T) {
	t.Parallel()
	c := qt.New(t)

	c.Assert(calculateMaxSizePerPartition(1000, 500, 5), qt.Equals, 200)
	c.Assert(calculateMaxSizePerPartition(1000, 250, 5), qt.Equals, 400)

}
