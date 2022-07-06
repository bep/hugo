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
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// FilePathSeparator as defined by os.Separator.
const FilePathSeparator = string(filepath.Separator)

// filepathPathBridge is a bridge for common functionality in filepath vs path
type filepathPathBridge interface {
	Base(in string) string
	Ext(in string) string
	Separator() string
}

type filepathBridge struct{}

func (filepathBridge) Base(in string) string {
	return filepath.Base(in)
}

func (filepathBridge) Ext(in string) string {
	return filepath.Ext(in)
}

func (filepathBridge) Separator() string {
	return FilePathSeparator
}

var fpb filepathBridge

// Should be good enough for Hugo.
var isFileRe = regexp.MustCompile(`.*\..{1,6}$`)

// Dir behaves like path.Dir without the path.Clean step.
//
//	The returned path ends in a slash only if it is the root "/".
func Dir(s string) string {
	dir, _ := path.Split(s)
	if len(dir) > 1 && dir[len(dir)-1] == '/' {
		return dir[:len(dir)-1]
	}
	return dir
}

// AddTrailingSlash adds a trailing '/' if not already there.
func AddTrailingSlash(s string) string {
	if s == "" || s[len(s)-1] != '/' {
		return s + "/"
	}
	return s
}

// CommonDir returns the common directory of the given paths.
func CommonDir(path1, path2 string) string {
	if path1 == "" || path2 == "" {
		return ""
	}

	p1 := strings.Split(path1, "/")
	p2 := strings.Split(path2, "/")

	var common []string

	for i := 0; i < len(p1) && i < len(p2); i++ {
		if p1[i] == p2[i] {
			common = append(common, p1[i])
		} else {
			break
		}
	}

	return strings.Join(common, "/")
}

func IsOnSameLevel(path1, path2 string) bool {
	return strings.Count(path1, "/") == strings.Count(path2, "/")
}

// ExtNoDelimiter takes a path and returns the extension, excluding the delimiter, i.e. "md".
func ExtNoDelimiter(in string) string {
	return strings.TrimPrefix(Ext(in), ".")
}

// Ext takes a path and returns the extension, including the delimiter, i.e. ".md".
func Ext(in string) string {
	_, ext := fileAndExt(in, fpb)
	return ext
}

// PathAndExt is the same as FileAndExt, but it uses the path package.
func PathAndExt(in string) (string, string) {
	return fileAndExt(in, pb)
}

// FileAndExt takes a path and returns the file and extension separated,
// the extension including the delimiter, i.e. ".md".
func FileAndExt(in string) (string, string) {
	return fileAndExt(in, fpb)
}

// FileAndExtNoDelimiter takes a path and returns the file and extension separated,
// the extension excluding the delimiter, e.g "md".
func FileAndExtNoDelimiter(in string) (string, string) {
	file, ext := fileAndExt(in, fpb)
	return file, strings.TrimPrefix(ext, ".")
}

// Filename takes a file path, strips out the extension,
// and returns the name of the file.
func Filename(in string) (name string) {
	name, _ = fileAndExt(in, fpb)
	return
}

// FileAndExt returns the filename and any extension of a file path as
// two separate strings.
//
// If the path, in, contains a directory name ending in a slash,
// then both name and ext will be empty strings.
//
// If the path, in, is either the current directory, the parent
// directory or the root directory, or an empty string,
// then both name and ext will be empty strings.
//
// If the path, in, represents the path of a file without an extension,
// then name will be the name of the file and ext will be an empty string.
//
// If the path, in, represents a filename with an extension,
// then name will be the filename minus any extension - including the dot
// and ext will contain the extension - minus the dot.
func fileAndExt(in string, b filepathPathBridge) (name string, ext string) {
	ext = b.Ext(in)
	base := b.Base(in)

	return extractFilename(in, ext, base, b.Separator()), ext
}

func extractFilename(in, ext, base, pathSeparator string) (name string) {
	// No file name cases. These are defined as:
	// 1. any "in" path that ends in a pathSeparator
	// 2. any "base" consisting of just an pathSeparator
	// 3. any "base" consisting of just an empty string
	// 4. any "base" consisting of just the current directory i.e. "."
	// 5. any "base" consisting of just the parent directory i.e. ".."
	if (strings.LastIndex(in, pathSeparator) == len(in)-1) || base == "" || base == "." || base == ".." || base == pathSeparator {
		name = "" // there is NO filename
	} else if ext != "" { // there was an Extension
		// return the filename minus the extension (and the ".")
		name = base[:strings.LastIndex(base, ".")]
	} else {
		// no extension case so just return base, which willi
		// be the filename
		name = base
	}
	return
}

// AbsPathify creates an absolute path if given a working dir and a relative path.
// If already absolute, the path is just cleaned.
func AbsPathify(workingDir, inPath string) string {
	if filepath.IsAbs(inPath) {
		return filepath.Clean(inPath)
	}
	return filepath.Join(workingDir, inPath)
}

// GetRelativePath returns the relative path of a given path.
func GetRelativePath(path, base string) (final string, err error) {
	if filepath.IsAbs(path) && base == "" {
		return "", errors.New("source: missing base directory")
	}
	name := filepath.Clean(path)
	base = filepath.Clean(base)

	name, err = filepath.Rel(base, name)
	if err != nil {
		return "", err
	}

	if strings.HasSuffix(filepath.FromSlash(path), FilePathSeparator) && !strings.HasSuffix(name, FilePathSeparator) {
		name += FilePathSeparator
	}
	return name, nil
}

var slashFunc = func(r rune) bool {
	return r == '/'
}

// FieldsSlash cuts s into fields separated with '/'.
// TODO1 add some tests, consider leading/trailing slashes.
func FieldsSlash(s string) []string {
	f := strings.FieldsFunc(s, slashFunc)
	return f
}

type NamedSlice struct {
	Name  string
	Slice []string
}

func (n NamedSlice) String() string {
	if len(n.Slice) == 0 {
		return n.Name
	}
	return fmt.Sprintf("%s%s{%s}", n.Name, FilePathSeparator, strings.Join(n.Slice, ","))
}

// PathEscape escapes unicode letters in pth.
// Use URLEscape to escape full URLs including scheme, query etc.
// This is slightly faster for the common case.
// Note, there is a url.PathEscape function, but that also
// escapes /.
func PathEscape(pth string) string {
	u, err := url.Parse(pth)
	if err != nil {
		panic(err)
	}
	return u.EscapedPath()
}

// Sanitize sanitizes string to be used in Hugo's file paths and URLs, allowing only
// a predefined set of special Unicode characters.
//
// Spaces will be replaced with a single hyphen, and sequential hyphens will be reduced to one.
//
// This function is the core function used to normalize paths in Hugo.
//
// This function is used for key creation in Hugo's content map, which needs to be very fast.
// This key is also used as a base for URL/file path creation, so  this should always be truthful:
//
//	helpers.PathSpec.MakePathSanitized(anyPath) == helpers.PathSpec.MakePathSanitized(Sanitize(anyPath))
//
// Even if the user has stricter rules defined for the final paths (e.g. removePathAccents=true).
func Sanitize(s string) string {
	var willChange bool
	for i, r := range s {
		willChange = !isAllowedPathCharacter(s, i, r)
		if willChange {
			break
		}
	}

	if !willChange {
		// Prevent allocation when nothing changes.
		return s
	}

	target := make([]rune, 0, len(s))
	var (
		prependHyphen bool
		wasHyphen     bool
	)

	for i, r := range s {
		isAllowed := isAllowedPathCharacter(s, i, r)

		if isAllowed {
			// track explicit hyphen in input; no need to add a new hyphen if
			// we just saw one.
			wasHyphen = r == '-'

			if prependHyphen {
				// if currently have a hyphen, don't prepend an extra one
				if !wasHyphen {
					target = append(target, '-')
				}
				prependHyphen = false
			}
			target = append(target, r)
		} else if len(target) > 0 && !wasHyphen && unicode.IsSpace(r) {
			prependHyphen = true
		}
	}

	return string(target)
}

func isAllowedPathCharacter(s string, i int, r rune) bool {
	if r == ' ' {
		return false
	}
	// Check for the most likely first (faster).
	isAllowed := unicode.IsLetter(r) || unicode.IsDigit(r)
	isAllowed = isAllowed || r == '.' || r == '/' || r == '\\' || r == '_' || r == '#' || r == '+' || r == '~' || r == '-'
	isAllowed = isAllowed || unicode.IsMark(r)
	isAllowed = isAllowed || (r == '%' && i+2 < len(s) && ishex(s[i+1]) && ishex(s[i+2]))
	return isAllowed
}

// From https://golang.org/src/net/url/url.go
func ishex(c byte) bool {
	switch {
	case '0' <= c && c <= '9':
		return true
	case 'a' <= c && c <= 'f':
		return true
	case 'A' <= c && c <= 'F':
		return true
	}
	return false
}
