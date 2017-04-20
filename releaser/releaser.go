// Copyright 2017-present The Hugo Authors. All rights reserved.
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

// Package releaser implements a set of utilities and a wrapper around Goreleaser
// to help automate the Hugo release process.
package releaser

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/hugo/helpers"
)

const commitPrefix = "releaser:"

type ReleaseHandler struct {
	patch int
	step  int
}

func (r ReleaseHandler) shouldRelease() bool {
	return r.step < 1 || r.shouldContinue()
}

func (r ReleaseHandler) shouldContinue() bool {
	return r.step == 2
}

func (r ReleaseHandler) shouldPrepare() bool {
	return r.step < 1 || r.step == 1
}

func (r ReleaseHandler) calculateVersions(current helpers.HugoVersion) (helpers.HugoVersion, helpers.HugoVersion) {
	var (
		newVersion   = current
		finalVersion = current
	)

	newVersion.Suffix = ""

	if r.shouldContinue() {
		// The version in the current code base is in the state we want for
		// the release.
		if r.patch == 0 {
			finalVersion = newVersion.Next()
		}
	} else if r.patch > 0 {
		newVersion = helpers.CurrentHugoVersion.NextPatchLevel(r.patch)
	} else {
		finalVersion = newVersion.Next()
	}

	finalVersion.Suffix = "-DEV"

	return newVersion, finalVersion
}

func New(patch, step int) *ReleaseHandler {
	return &ReleaseHandler{patch: patch, step: step}
}

func (r *ReleaseHandler) Run() error {
	if r.shouldRelease() && os.Getenv("GITHUB_TOKEN") == "" {
		return errors.New("GITHUB_TOKEN not set, needed by goreleaser")
	}

	newVersion, finalVersion := r.calculateVersions(helpers.CurrentHugoVersion)

	if true || confirm(fmt.Sprint("Start release of ", newVersion, "?")) {

		tag := "v" + newVersion.String()

		// Exit early if tag already exists
		out, err := git("tag", "-l", tag)

		if err != nil {
			return err
		}

		if strings.Contains(out, tag) {
			return fmt.Errorf("Tag %q already exists", tag)
		}

		var gitCommits gitInfos

		if r.shouldPrepare() || r.shouldRelease() {
			gitCommits, err = getGitInfos(true)
			if err != nil {
				return err
			}
		}

		if r.shouldPrepare() {
			if err := bumpVersions(newVersion); err != nil {
				return err
			}

			if _, err := git("commit", "-a", "-m", fmt.Sprintf("%s Bump versions for release of %s", commitPrefix, newVersion)); err != nil {
				return err
			}

			releaseNotesFile, err := writeReleaseNotesToDocsTemp(tag, gitCommits)
			if err != nil {
				return err
			}

			if _, err := git("add", releaseNotesFile); err != nil {
				return err
			}
			if _, err := git("commit", "-m", fmt.Sprintf("%s Add relase notes draft for release of %s", commitPrefix, newVersion)); err != nil {
				return err
			}
		}

		if !r.shouldRelease() {
			fmt.Println("Skip release ... Use --state=2 to continue.")
			return nil
		}

		releaseNotesFile := getRelaseNotesDocsTempFilename(tag)

		// Write the release notes to the docs site as well.
		docFile, err := writeReleaseNotesToDocs(tag, releaseNotesFile)
		if err != nil {
			return err
		}

		if _, err := git("add", docFile); err != nil {
			return err
		}
		if _, err := git("commit", "-m", fmt.Sprintf("%s Add relase notes to /docs for release of %s", commitPrefix, newVersion)); err != nil {
			return err
		}

		if _, err := git("tag", "-a", tag, "-m", fmt.Sprintf("%s %s", commitPrefix, newVersion)); err != nil {
			return err
		}

		if confirm("Release to GitHub") {
			// TODO(bep) release draft flag
			if err := release(releaseNotesFile); err != nil {
				return err
			}
		}

		if err := bumpVersions(finalVersion); err != nil {
			return err
		}

		if _, err := git("commit", "-a", "-m", fmt.Sprintf("%s Prepare repository for %s", commitPrefix, finalVersion)); err != nil {
			return err
		}

	}

	return nil
}

func confirm(s string) bool {
	r := bufio.NewReader(os.Stdin)
	tries := 10

	for ; tries > 0; tries-- {
		fmt.Printf("%s [y/n]: ", s)

		res, err := r.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		if len(res) < 2 {
			continue
		}

		return strings.ToLower(strings.TrimSpace(res))[0] == 'y'
	}

	return false
}

func release(releaseNotesFile string) error {
	cmd := exec.Command("goreleaser", "--release-notes", releaseNotesFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("goreleaser failed: %s", err)
	}
	return nil
}

func bumpVersions(ver helpers.HugoVersion) error {
	fromDev := ""
	toDev := ""

	if ver.Suffix != "" {
		toDev = "-DEV"
	} else {
		fromDev = "-DEV"
	}

	if err := replaceInFile("helpers/hugo.go",
		`Number:(\s{4,})(.*),`, fmt.Sprintf(`Number:${1}%.2f,`, ver.Number),
		`PatchLevel:(\s*)(.*),`, fmt.Sprintf(`PatchLevel:${1}%d,`, ver.PatchLevel),
		fmt.Sprintf(`Suffix:(\s{4,})"%s",`, fromDev), fmt.Sprintf(`Suffix:${1}"%s",`, toDev)); err != nil {
		return err
	}

	snapcraftGrade := "stable"
	if ver.Suffix != "" {
		snapcraftGrade = "devel"
	}
	if err := replaceInFile("snapcraft.yaml",
		`version: "(.*)"`, fmt.Sprintf(`version: "%s"`, ver),
		`grade: (.*) #`, fmt.Sprintf(`grade: %s #`, snapcraftGrade)); err != nil {
		return err
	}

	var minVersion string
	if ver.Suffix != "" {
		// People use the DEV version in daily use, and we cannot create new themes
		// with the next version before it is released.
		minVersion = ver.Prev().String()
	} else {
		minVersion = ver.String()
	}

	if err := replaceInFile("commands/new.go",
		`min_version = "(.*)"`, fmt.Sprintf(`min_version = "%s"`, minVersion)); err != nil {
		return err
	}

	// docs/config.toml
	if err := replaceInFile("docs/config.toml",
		`release = "(.*)"`, fmt.Sprintf(`release = "%s"`, ver)); err != nil {
		return err
	}

	return nil
}

func replaceInFile(filename string, oldNew ...string) error {
	fullFilename := hugoFilepath(filename)
	fi, err := os.Stat(fullFilename)
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(fullFilename)
	if err != nil {
		return err
	}
	newContent := string(b)

	for i := 0; i < len(oldNew); i += 2 {
		re := regexp.MustCompile(oldNew[i])
		newContent = re.ReplaceAllString(newContent, oldNew[i+1])
	}

	return ioutil.WriteFile(fullFilename, []byte(newContent), fi.Mode())

	return nil
}

func hugoFilepath(filename string) string {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(pwd, filename)
}
