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

// Package provides ways to identify values in Hugo. Used for dependency tracking etc.
package identity

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestStringPrefixIdentity(t *testing.T) {
	c := qt.New(t)

	sid := StringPrefixIdentity("/a/b/mysuffix")

	c.Assert(IsNotDependent(StringIdentity("/a/b"), sid), qt.IsFalse)
	c.Assert(IsNotDependent(StringIdentity("/a/b/c"), sid), qt.IsTrue)
	c.Assert(IsNotDependent(sid, StringIdentity("/a/b")), qt.IsFalse)
	c.Assert(IsNotDependent(sid, StringIdentity("/a/b/c")), qt.IsTrue)
}
