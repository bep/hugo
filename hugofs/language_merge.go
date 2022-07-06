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

package hugofs

import (
	"io/fs"
)

// LanguageDirsMerger implements the overlayfs.DirsMerger func, which is used
// to merge two directories.
var LanguageDirsMerger = func(lofi, bofi []fs.DirEntry) []fs.DirEntry {
	for _, fi1 := range bofi {
		fim1 := fi1.(FileMetaDirEntry)
		var found bool
		for _, fi2 := range lofi {
			fim2 := fi2.(FileMetaDirEntry)
			if fi1.Name() == fi2.Name() && fim1.Meta().Lang == fim2.Meta().Lang {
				found = true
				break
			}
		}
		if !found {
			lofi = append(lofi, fi1)
		}
	}

	return lofi
}
