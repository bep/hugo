// Copyright 2021 The Hugo Authors. All rights reserved.
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

package paths

import (
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestGetRelativePath(t *testing.T) {
	tests := []struct {
		path   string
		base   string
		expect any
	}{
		{filepath.FromSlash("/a/b"), filepath.FromSlash("/a"), filepath.FromSlash("b")},
		{filepath.FromSlash("/a/b/c/"), filepath.FromSlash("/a"), filepath.FromSlash("b/c/")},
		{filepath.FromSlash("/c"), filepath.FromSlash("/a/b"), filepath.FromSlash("../../c")},
		{filepath.FromSlash("/c"), "", false},
	}
	for i, this := range tests {
		// ultimately a fancy wrapper around filepath.Rel
		result, err := GetRelativePath(this.path, this.base)

		if b, ok := this.expect.(bool); ok && !b {
			if err == nil {
				t.Errorf("[%d] GetRelativePath didn't return an expected error", i)
			}
		} else {
			if err != nil {
				t.Errorf("[%d] GetRelativePath failed: %s", i, err)
				continue
			}
			if result != this.expect {
				t.Errorf("[%d] GetRelativePath got %v but expected %v", i, result, this.expect)
			}
		}

	}
}

func TestExtNoDelimiter(t *testing.T) {
	c := qt.New(t)
	c.Assert(ExtNoDelimiter(filepath.FromSlash("/my/data.json")), qt.Equals, "json")
}

func TestFilename(t *testing.T) {
	type test struct {
		input, expected string
	}
	data := []test{
		{"index.html", "index"},
		{"./index.html", "index"},
		{"/index.html", "index"},
		{"index", "index"},
		{"/tmp/index.html", "index"},
		{"./filename-no-ext", "filename-no-ext"},
		{"/filename-no-ext", "filename-no-ext"},
		{"filename-no-ext", "filename-no-ext"},
		{"directory/", ""}, // no filename case??
		{"directory/.hidden.ext", ".hidden"},
		{"./directory/../~/banana/gold.fish", "gold"},
		{"../directory/banana.man", "banana"},
		{"~/mydir/filename.ext", "filename"},
		{"./directory//tmp/filename.ext", "filename"},
	}

	for i, d := range data {
		output := Filename(filepath.FromSlash(d.input))
		if d.expected != output {
			t.Errorf("Test %d failed. Expected %q got %q", i, d.expected, output)
		}
	}
}

func TestFileAndExt(t *testing.T) {
	type test struct {
		input, expectedFile, expectedExt string
	}
	data := []test{
		{"index.html", "index", ".html"},
		{"./index.html", "index", ".html"},
		{"/index.html", "index", ".html"},
		{"index", "index", ""},
		{"/tmp/index.html", "index", ".html"},
		{"./filename-no-ext", "filename-no-ext", ""},
		{"/filename-no-ext", "filename-no-ext", ""},
		{"filename-no-ext", "filename-no-ext", ""},
		{"directory/", "", ""}, // no filename case??
		{"directory/.hidden.ext", ".hidden", ".ext"},
		{"./directory/../~/banana/gold.fish", "gold", ".fish"},
		{"../directory/banana.man", "banana", ".man"},
		{"~/mydir/filename.ext", "filename", ".ext"},
		{"./directory//tmp/filename.ext", "filename", ".ext"},
	}

	for i, d := range data {
		file, ext := fileAndExt(filepath.FromSlash(d.input), fpb)
		if d.expectedFile != file {
			t.Errorf("Test %d failed. Expected filename %q got %q.", i, d.expectedFile, file)
		}
		if d.expectedExt != ext {
			t.Errorf("Test %d failed. Expected extension %q got %q.", i, d.expectedExt, ext)
		}
	}
}

func TesSanitize(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		input    string
		expected string
	}{
		{"  Foo bar  ", "Foo-bar"},
		{"Foo.Bar/foo_Bar-Foo", "Foo.Bar/foo_Bar-Foo"},
		{"fOO,bar:foobAR", "fOObarfoobAR"},
		{"FOo/BaR.html", "FOo/BaR.html"},
		{"FOo/Ba---R.html", "FOo/Ba-R.html"},
		{"FOo/Ba       R.html", "FOo/Ba-R.html"},
		{"трям/трям", "трям/трям"},
		{"은행", "은행"},
		{"Банковский кассир", "Банковскии-кассир"},
		// Issue #1488
		{"संस्कृत", "संस्कृत"},
		{"a%C3%B1ame", "a%C3%B1ame"},          // Issue #1292
		{"this+is+a+test", "sthis+is+a+test"}, // Issue #1290
		{"~foo", "~foo"},                      // Issue #2177

	}

	for _, test := range tests {
		c.Assert(Sanitize(test.input), qt.Equals, test.expected)
	}
}

func BenchmarkSanitize(b *testing.B) {
	const (
		allAlowedPath = "foo/bar"
		spacePath     = "foo bar"
	)

	// This should not allocate any memory.
	b.Run("All allowed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			got := Sanitize(allAlowedPath)
			if got != allAlowedPath {
				b.Fatal(got)
			}
		}
	})

	// This will allocate some memory.
	b.Run("Spaces", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			got := Sanitize(spacePath)
			if got != "foo-bar" {
				b.Fatal(got)
			}
		}
	})
}

func TestIsOnSameLevel(t *testing.T) {
	c := qt.New(t)
	c.Assert(IsOnSameLevel("/a/b/c/d", "/a/b/c/d"), qt.Equals, true)
	c.Assert(IsOnSameLevel("", ""), qt.Equals, true)
	c.Assert(IsOnSameLevel("/", "/"), qt.Equals, true)
	c.Assert(IsOnSameLevel("/a/b/c", "/a/b/c/d"), qt.Equals, false)
	c.Assert(IsOnSameLevel("/a/b/c/d", "/a/b/c"), qt.Equals, false)
}

func TestDir(t *testing.T) {
	c := qt.New(t)
	c.Assert(Dir("/a/b/c/d"), qt.Equals, "/a/b/c")
	c.Assert(Dir("/a"), qt.Equals, "/")
	c.Assert(Dir("/"), qt.Equals, "/")
	c.Assert(Dir(""), qt.Equals, "")
}
