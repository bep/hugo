// Copyright 2018 The Hugo Authors. All rights reserved.
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

package pcache

import (
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPersistentCache(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	cache, _ := createCache(t, true)

	create1, state1 := createTestObjectCreate("1", "ABC", true)
	create2, state2 := createTestObjectCreate("1", "CDE", true)

	to1v, err := cache.GetOrCreate(state1.vID, create1)
	assert.NoError(err)
	to1 := to1v.(*testObject)
	assert.Equal(state1.vID.ID, to1._ID())
	assert.True(state1.created)
	assert.IsType(&testObject{}, to1v)

	state1.created = false
	to1v_2, err := cache.GetOrCreate(state1.vID, create1)
	assert.NoError(err)
	assert.Equal(to1v, to1v_2)
	assert.False(state1.created)

	to2v, err := cache.GetOrCreate(state2.vID, create2)
	assert.NoError(err)
	to2 := to2v.(*testObject)
	assert.Equal(state2.vID.ID, to2._ID())
	assert.True(state2.created)

	assert.NoError(cache.Close())

	cache2, cleanup := createCacheFrom(t, cache, true)
	defer cleanup()

	state1.created = false
	to1v_3, err := cache2.GetOrCreate(state1.vID, create1)
	assert.NoError(err)
	assert.Equal(to1v, to1v_3)
	assert.False(state1.created)

	state2.created = false
	to2v_2, err := cache2.GetOrCreate(state2.vID, create1)
	assert.NoError(err)
	assert.Equal(to2v, to2v_2)
	assert.False(state2.created)

}

func TestPersistentCacheValueType(t *testing.T) {
	t.Parallel()
	assert := require.New(t)

	cache, _ := createCache(t, false)

	create1, state1 := createTestObjectCreate("1", "ABC", false)

	to1v, err := cache.GetOrCreate(state1.vID, create1)
	assert.NoError(err)
	to1 := to1v.(testObject)
	assert.Equal(state1.vID.ID, to1._ID())
	assert.True(state1.created)
	assert.IsType(testObject{}, to1v)

	assert.NoError(cache.Close())

	cache2, cleanup := createCacheFrom(t, cache, false)
	defer cleanup()

	state1.created = false
	to1v_2, err := cache2.GetOrCreate(state1.vID, create1)
	assert.NoError(err)
	assert.Equal(to1v, to1v_2)
	assert.False(state1.created)
}

func createCacheFrom(t *testing.T, from *persistentCache, pointer bool) (*persistentCache, func()) {
	var entity interface{}
	if pointer {
		entity = &testObject{}
	} else {
		entity = testObject{}
	}

	c := New(from.filename, entity)
	err := c.Open()
	if err != nil {
		t.Fatal(err)
	}

	return c.(*persistentCache), func() {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(filepath.Dir(from.filename))
	}
}

func createTestObjectCreate(version, ID string, pointer bool) (func() (Identifier, error), *cacheTestsState) {
	state := &cacheTestsState{}
	state.vID = NewVersionedID(version, ID)

	timestamp, _ := time.Parse(time.RFC3339, "2018-01-02T15:04:05Z07:00")

	return func() (Identifier, error) {
		// We do round-trip testing of Go struct => JSON => Go struct, so add
		// any special types to this testObject.
		top := testObject{
			ID:        state.vID.ID,
			MyString:  "hi",
			MyRat:     big.NewRat(1, 100),
			MyInt64:   int64(64),
			MyFloat64: float64(3.14159264),
			MyDate:    timestamp,
		}

		state.created = true

		if pointer {
			return &top, nil
		}

		return top, nil
	}, state
}

type cacheTestsState struct {
	vID     VersionedID
	created bool
}

type testObject struct {
	ID
	MyString  string
	MyRat     *big.Rat
	MyInt64   int64
	MyFloat64 float64
	MyDate    time.Time
}

func (t *testObject) String() string {
	return string(t.ID)
}

func createCache(t *testing.T, pointer bool) (*persistentCache, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "hugodbcache")
	if err != nil {
		t.Fatal(err)
	}

	var entity interface{}
	if pointer {
		entity = &testObject{}
	} else {
		entity = testObject{}
	}

	c := New(filepath.Join(dir, "hugocache.json"), entity)
	err = c.Open()
	if err != nil {
		t.Fatal(err)
	}

	return c.(*persistentCache), func() {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(dir)
	}
}
