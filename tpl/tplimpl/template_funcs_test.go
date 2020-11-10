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

package tplimpl_test

import (
<<<<<<< HEAD
=======
	"bytes"
	"context"
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"fmt"
	"strings"
	"testing"

	"github.com/gohugoio/hugo/hugolib"

	"github.com/gohugoio/hugo/tpl/internal"
)

func TestTemplateFuncsExamples(t *testing.T) {
	t.Parallel()

	files := `
-- config.toml --
disableKinds=["home", "section", "taxonomy", "term", "sitemap", "robotsTXT"]
ignoreErrors = ["my-err-id"]
[outputs]
home=["HTML"]
-- layouts/partials/header.html --
<title>Hugo Rocks!</title>
-- files/README.txt --
Hugo Rocks!
-- content/blog/hugo-rocks.md --
--- 
title: "**BatMan**"
---
`

	b := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
			NeedsOsFS:   true,
		},
	).Build()

	d := b.H.Sites[0].Deps

	var (
		templates []string
		expected  []string
	)

	for _, nsf := range internal.TemplateFuncsNamespaceRegistry {
		ns := nsf(d)
		for _, mm := range ns.MethodMappings {
			for _, example := range mm.Examples {
				if strings.Contains(example[0], "errorf") {
					// This will fail the build, so skip for now.
					continue
				}
				templates = append(templates, example[0])
				expected = append(expected, example[1])
			}
		}
	}
<<<<<<< HEAD

	files += fmt.Sprintf("-- layouts/_default/single.html --\n%s\n", strings.Join(templates, "\n"))
	b = hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:           t,
			TxtarString: files,
			NeedsOsFS:   true,
		},
	).Build()

	b.AssertFileContent("public/blog/hugo-rocks/index.html", expected...)
=======
}

// TODO(bep) it would be dandy to put this one into the partials package, but
// we have some package cycle issues to solve first.
func TestPartialCached(t *testing.T) {
	t.Parallel()

	c := qt.New(t)

	partial := `Now: {{ now.UnixNano }}`
	name := "testing"

	var data struct {
	}

	v := newTestConfig()

	config := newDepsConfig(v)

	config.WithTemplate = func(templ tpl.TemplateManager) error {
		err := templ.AddTemplate("partials/"+name, partial)
		if err != nil {
			return err
		}

		return nil
	}

	de, err := deps.New(config)
	c.Assert(err, qt.IsNil)
	defer de.Close()
	c.Assert(de.LoadResources(), qt.IsNil)

	ns := partials.New(de)

	res1, err := ns.IncludeCached(context.Background(), name, &data)
	c.Assert(err, qt.IsNil)

	for j := 0; j < 10; j++ {
		time.Sleep(2 * time.Nanosecond)
		res2, err := ns.IncludeCached(context.Background(), name, &data)
		c.Assert(err, qt.IsNil)

		if !reflect.DeepEqual(res1, res2) {
			t.Fatalf("cache mismatch")
		}

		res3, err := ns.IncludeCached(context.Background(), name, &data, fmt.Sprintf("variant%d", j))
		c.Assert(err, qt.IsNil)

		if reflect.DeepEqual(res1, res3) {
			t.Fatalf("cache mismatch")
		}
	}
}

func BenchmarkPartial(b *testing.B) {
	ctx := context.Background()
	doBenchmarkPartial(b, func(ns *partials.Namespace) error {
		_, err := ns.Include(ctx, "bench1")
		return err
	})
}

func BenchmarkPartialCached(b *testing.B) {
	ctx := context.Background()
	doBenchmarkPartial(b, func(ns *partials.Namespace) error {
		_, err := ns.IncludeCached(ctx, "bench1", nil)
		return err
	})
}

func doBenchmarkPartial(b *testing.B, f func(ns *partials.Namespace) error) {
	c := qt.New(b)
	config := newDepsConfig(config.New())
	config.WithTemplate = func(templ tpl.TemplateManager) error {
		err := templ.AddTemplate("partials/bench1", `{{ shuffle (seq 1 10) }}`)
		if err != nil {
			return err
		}

		return nil
	}

	de, err := deps.New(config)
	c.Assert(err, qt.IsNil)
	defer de.Close()
	c.Assert(de.LoadResources(), qt.IsNil)

	ns := partials.New(de)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := f(ns); err != nil {
				b.Fatalf("error executing template: %s", err)
			}
		}
	})
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}
