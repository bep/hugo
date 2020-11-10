<<<<<<< HEAD:tpl/diagrams/init.go
// Copyright 2022 The Hugo Authors. All rights reserved.
=======
// Copyright 2021 The Hugo Authors. All rights reserved.
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution):tpl/transform/init_test.go
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

package diagrams

import (
<<<<<<< HEAD:tpl/diagrams/init.go
=======
	"testing"

	"github.com/gohugoio/hugo/cache/memcache"

	qt "github.com/frankban/quicktest"
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution):tpl/transform/init_test.go
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/tpl/internal"
)

const name = "diagrams"

<<<<<<< HEAD:tpl/diagrams/init.go
func init() {
	f := func(d *deps.Deps) *internal.TemplateFuncsNamespace {
		ctx := &Diagrams{
			d: d,
=======
	for _, nsf := range internal.TemplateFuncsNamespaceRegistry {
		ns = nsf(&deps.Deps{
			MemCache: memcache.New(memcache.Config{}),
		})
		if ns.Name == name {
			found = true
			break
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution):tpl/transform/init_test.go
		}

		ns := &internal.TemplateFuncsNamespace{
			Name:    name,
			Context: func(args ...any) (any, error) { return ctx, nil },
		}

		return ns
	}

	internal.AddTemplateFuncsNamespace(f)
}
