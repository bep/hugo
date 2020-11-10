package hugolib

import (
	"bytes"
<<<<<<< HEAD
	"encoding/base64"
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"fmt"
	"io"
	"os"
	"path/filepath"
<<<<<<< HEAD
	"regexp"
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"strings"
	"sync"
	"testing"

	jww "github.com/spf13/jwalterweatherman"

	qt "github.com/frankban/quicktest"
	"github.com/fsnotify/fsnotify"
	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/hexec"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/config/security"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/htesting"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/spf13/afero"
	"golang.org/x/tools/txtar"
)

func NewIntegrationTestBuilder(conf IntegrationTestConfig) *IntegrationTestBuilder {
<<<<<<< HEAD
	// Code fences.
	conf.TxtarString = strings.ReplaceAll(conf.TxtarString, "§§§", "```")

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	data := txtar.Parse([]byte(conf.TxtarString))

	c, ok := conf.T.(*qt.C)
	if !ok {
		c = qt.New(conf.T)
	}

	if conf.NeedsOsFS {
<<<<<<< HEAD
		if !filepath.IsAbs(conf.WorkingDir) {
			tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
			c.Assert(err, qt.IsNil)
			conf.WorkingDir = filepath.Join(tempDir, conf.WorkingDir)
			if !conf.PrintAndKeepTempDir {
				c.Cleanup(clean)
			} else {
				fmt.Println("\nUsing WorkingDir dir:", conf.WorkingDir)
			}
		}
	} else if conf.WorkingDir == "" {
		conf.WorkingDir = helpers.FilePathSeparator
=======
		doClean := true
		tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
		c.Assert(err, qt.IsNil)
		conf.WorkingDir = filepath.Join(tempDir, conf.WorkingDir)
		if doClean {
			c.Cleanup(clean)
		}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	}

	return &IntegrationTestBuilder{
		Cfg:  conf,
		C:    c,
		data: data,
	}
}

// IntegrationTestBuilder is a (partial) rewrite of sitesBuilder.
// The main problem with the "old" one was that it was that the test data was often a little hidden,
// so it became hard to look at a test and determine what it should do, especially coming back to the
// test after a year or so.
type IntegrationTestBuilder struct {
	*qt.C

	data *txtar.Archive

	fs *hugofs.Fs
	H  *HugoSites

	Cfg IntegrationTestConfig

	changedFiles []string
	createdFiles []string
	removedFiles []string
	renamedFiles []string

	buildCount int
	counters   *testCounters
	logBuff    lockingBuffer

	builderInit sync.Once
}

type lockingBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *lockingBuffer) Write(p []byte) (n int, err error) {
	b.Lock()
	n, err = b.Buffer.Write(p)
	b.Unlock()
	return
}

func (s *IntegrationTestBuilder) AssertLogContains(text string) {
	s.Helper()
	s.Assert(s.logBuff.String(), qt.Contains, text)
}

<<<<<<< HEAD
func (s *IntegrationTestBuilder) AssertLogMatches(expression string) {
	s.Helper()
	re := regexp.MustCompile(expression)
	s.Assert(re.MatchString(s.logBuff.String()), qt.IsTrue, qt.Commentf(s.logBuff.String()))
}

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
func (s *IntegrationTestBuilder) AssertBuildCountData(count int) {
	s.Helper()
	s.Assert(s.H.init.data.InitCount(), qt.Equals, count)
}

func (s *IntegrationTestBuilder) AssertBuildCountGitInfo(count int) {
	s.Helper()
	s.Assert(s.H.init.gitInfo.InitCount(), qt.Equals, count)
}

func (s *IntegrationTestBuilder) AssertBuildCountLayouts(count int) {
	s.Helper()
	s.Assert(s.H.init.layouts.InitCount(), qt.Equals, count)
}

func (s *IntegrationTestBuilder) AssertBuildCountTranslations(count int) {
	s.Helper()
	s.Assert(s.H.init.translations.InitCount(), qt.Equals, count)
}

func (s *IntegrationTestBuilder) AssertFileContent(filename string, matches ...string) {
	s.Helper()
	content := strings.TrimSpace(s.FileContent(filename))
	for _, m := range matches {
		lines := strings.Split(m, "\n")
		for _, match := range lines {
			match = strings.TrimSpace(match)
			if match == "" || strings.HasPrefix(match, "#") {
				continue
			}
<<<<<<< HEAD
			s.Assert(content, qt.Contains, match, qt.Commentf(m))
=======
			s.Assert(content, qt.Contains, match, qt.Commentf(content))
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}
	}
}

<<<<<<< HEAD
func (s *IntegrationTestBuilder) AssertFileContentExact(filename string, matches ...string) {
	s.Helper()
	content := s.FileContent(filename)
	for _, m := range matches {
		s.Assert(content, qt.Contains, m, qt.Commentf(m))
	}
}

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
func (s *IntegrationTestBuilder) AssertDestinationExists(filename string, b bool) {
	checker := qt.IsTrue
	if !b {
		checker = qt.IsFalse
	}
	s.Assert(s.destinationExists(filepath.Clean(filename)), checker)
}

func (s *IntegrationTestBuilder) destinationExists(filename string) bool {
<<<<<<< HEAD
	b, err := helpers.Exists(filename, s.fs.PublishDir)
=======
	b, err := helpers.Exists(filename, s.fs.Destination)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	if err != nil {
		panic(err)
	}
	return b
}

<<<<<<< HEAD
func (s *IntegrationTestBuilder) AssertIsFileError(err error) herrors.FileError {
	s.Assert(err, qt.ErrorAs, new(herrors.FileError))
	return herrors.UnwrapFileError(err)
=======
func (s *IntegrationTestBuilder) AssertIsFileError(err error) {
	var ferr *herrors.ErrorWithFileContext
	s.Assert(err, qt.ErrorAs, &ferr)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func (s *IntegrationTestBuilder) AssertRenderCountContent(count int) {
	s.Helper()
	s.Assert(s.counters.contentRenderCounter, qt.Equals, uint64(count))
}

func (s *IntegrationTestBuilder) AssertRenderCountPage(count int) {
	s.Helper()
	s.Assert(s.counters.pageRenderCounter, qt.Equals, uint64(count))
}

func (s *IntegrationTestBuilder) Build() *IntegrationTestBuilder {
	s.Helper()
	_, err := s.BuildE()
<<<<<<< HEAD
	if s.Cfg.Verbose || err != nil {
=======
	if s.Cfg.Verbose {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		fmt.Println(s.logBuff.String())
	}
	s.Assert(err, qt.IsNil)
	return s
}

func (s *IntegrationTestBuilder) BuildE() (*IntegrationTestBuilder, error) {
	s.Helper()
<<<<<<< HEAD
	if err := s.initBuilder(); err != nil {
		return s, err
	}

=======
	s.initBuilder()
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	err := s.build(BuildCfg{})
	return s, err
}

type IntegrationTestDebugConfig struct {
	Out io.Writer

	PrintDestinationFs bool
	PrintPagemap       bool

	PrefixDestinationFs string
	PrefixPagemap       string
}

<<<<<<< HEAD
=======
func (s *IntegrationTestBuilder) Debug(cfg IntegrationTestDebugConfig) {
	if cfg.PrefixDestinationFs == "" {
		cfg.PrefixDestinationFs = "public"
	}

	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}

	if cfg.PrintPagemap {
		for _, site := range s.H.Sites {
			fmt.Printf("Page Map %s:\n", site.Lang())
			site.pageMap.Debug(cfg.PrefixPagemap, out)
		}
	}

	if cfg.PrintDestinationFs {
		fmt.Printf("Print destination Fs using prefix %s:\n", cfg.PrefixDestinationFs)
		helpers.PrintFs(s.fs.Destination, cfg.PrefixDestinationFs, out)
	}
}

>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
func (s *IntegrationTestBuilder) EditFileReplace(filename string, replacementFunc func(s string) string) *IntegrationTestBuilder {
	absFilename := s.absFilename(filename)
	b, err := afero.ReadFile(s.fs.Source, absFilename)
	s.Assert(err, qt.IsNil)
	s.changedFiles = append(s.changedFiles, absFilename)
	oldContent := string(b)
	s.writeSource(absFilename, replacementFunc(oldContent))
	return s
}

func (s *IntegrationTestBuilder) EditFiles(filenameContent ...string) *IntegrationTestBuilder {
	for i := 0; i < len(filenameContent); i += 2 {
		filename, content := filepath.FromSlash(filenameContent[i]), filenameContent[i+1]
		absFilename := s.absFilename(filename)
		s.changedFiles = append(s.changedFiles, absFilename)
		s.writeSource(absFilename, content)
	}
	return s
}

func (s *IntegrationTestBuilder) AddFiles(filenameContent ...string) *IntegrationTestBuilder {
	for i := 0; i < len(filenameContent); i += 2 {
		filename, content := filepath.FromSlash(filenameContent[i]), filenameContent[i+1]
		absFilename := s.absFilename(filename)
		s.createdFiles = append(s.createdFiles, absFilename)
		s.writeSource(absFilename, content)
	}
	return s
}

func (s *IntegrationTestBuilder) RemoveFiles(filenames ...string) *IntegrationTestBuilder {
	for _, filename := range filenames {
		absFilename := s.absFilename(filename)
		s.removedFiles = append(s.removedFiles, absFilename)
		s.Assert(s.fs.Source.Remove(absFilename), qt.IsNil)

	}

	return s
}

func (s *IntegrationTestBuilder) RenameFile(old, new string) *IntegrationTestBuilder {
	absOldFilename := s.absFilename(old)
	absNewFilename := s.absFilename(new)
	s.renamedFiles = append(s.renamedFiles, absOldFilename)
	s.createdFiles = append(s.createdFiles, absNewFilename)
	s.Assert(s.fs.Source.Rename(absOldFilename, absNewFilename), qt.IsNil)
	return s
}

func (s *IntegrationTestBuilder) FileContent(filename string) string {
	s.Helper()
<<<<<<< HEAD
	return s.readWorkingDir(s, s.fs, filepath.FromSlash(filename))
}

func (s *IntegrationTestBuilder) initBuilder() error {
	var initErr error
=======
	filename = filepath.FromSlash(filename)
	if !strings.HasPrefix(filename, s.Cfg.WorkingDir) {
		filename = filepath.Join(s.Cfg.WorkingDir, filename)
	}
	return s.readDestination(s, s.fs, filename)
}

func (s *IntegrationTestBuilder) initBuilder() {
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	s.builderInit.Do(func() {
		var afs afero.Fs
		if s.Cfg.NeedsOsFS {
			afs = afero.NewOsFs()
		} else {
			afs = afero.NewMemMapFs()
		}

		if s.Cfg.LogLevel == 0 {
			s.Cfg.LogLevel = jww.LevelWarn
		}

		logger := loggers.NewBasicLoggerForWriter(s.Cfg.LogLevel, &s.logBuff)

<<<<<<< HEAD
		isBinaryRe := regexp.MustCompile(`^(.*)(\.png|\.jpg)$`)

		for _, f := range s.data.Files {
			filename := filepath.Join(s.Cfg.WorkingDir, f.Name)
			data := bytes.TrimSuffix(f.Data, []byte("\n"))
			if isBinaryRe.MatchString(filename) {
				var err error
				data, err = base64.StdEncoding.DecodeString(string(data))
				s.Assert(err, qt.IsNil)

			}
			s.Assert(afs.MkdirAll(filepath.Dir(filename), 0777), qt.IsNil)
			s.Assert(afero.WriteFile(afs, filename, data, 0666), qt.IsNil)
=======
		fs := hugofs.NewFrom(afs, config.New())

		for _, f := range s.data.Files {
			filename := filepath.Join(s.Cfg.WorkingDir, f.Name)
			s.Assert(afs.MkdirAll(filepath.Dir(filename), 0777), qt.IsNil)
			s.Assert(afero.WriteFile(afs, filename, bytes.TrimSuffix(f.Data, []byte("\n")), 0666), qt.IsNil)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
		}

		cfg, _, err := LoadConfig(
			ConfigSourceDescriptor{
				WorkingDir: s.Cfg.WorkingDir,
				Fs:         afs,
				Logger:     logger,
				Environ:    []string{},
				Filename:   "config.toml",
			},
			func(cfg config.Provider) error {
				return nil
			},
		)

		s.Assert(err, qt.IsNil)

		cfg.Set("workingDir", s.Cfg.WorkingDir)

<<<<<<< HEAD
		fs := hugofs.NewFrom(afs, cfg)

		s.Assert(err, qt.IsNil)

		depsCfg := deps.DepsCfg{Cfg: cfg, Fs: fs, Running: s.Cfg.Running, Logger: logger}
		sites, err := NewHugoSites(depsCfg)
		if err != nil {
			initErr = err
			return
		}
=======
		depsCfg := deps.DepsCfg{Cfg: cfg, Fs: fs, Running: s.Cfg.Running, Logger: logger}
		sites, err := NewHugoSites(depsCfg)
		s.Assert(err, qt.IsNil)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)

		s.H = sites
		s.fs = fs

		if s.Cfg.NeedsNpmInstall {
			wd, _ := os.Getwd()
			s.Assert(os.Chdir(s.Cfg.WorkingDir), qt.IsNil)
			s.C.Cleanup(func() { os.Chdir(wd) })
			sc := security.DefaultConfig
			sc.Exec.Allow = security.NewWhitelist("npm")
			ex := hexec.New(sc)
			command, err := ex.New("npm", "install")
			s.Assert(err, qt.IsNil)
			s.Assert(command.Run(), qt.IsNil)

		}
	})
<<<<<<< HEAD

	return initErr
=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func (s *IntegrationTestBuilder) absFilename(filename string) string {
	filename = filepath.FromSlash(filename)
	if filepath.IsAbs(filename) {
		return filename
	}
	if s.Cfg.WorkingDir != "" && !strings.HasPrefix(filename, s.Cfg.WorkingDir) {
		filename = filepath.Join(s.Cfg.WorkingDir, filename)
	}
	return filename
}

func (s *IntegrationTestBuilder) build(cfg BuildCfg) error {
	s.Helper()
	defer func() {
		s.changedFiles = nil
		s.createdFiles = nil
		s.removedFiles = nil
		s.renamedFiles = nil
	}()

	changeEvents := s.changeEvents()
	s.logBuff.Reset()
	s.counters = &testCounters{}
	cfg.testCounters = s.counters

	if s.buildCount > 0 && (len(changeEvents) == 0) {
		return nil
	}

	s.buildCount++

	err := s.H.Build(cfg, changeEvents...)
	if err != nil {
		return err
	}
	logErrorCount := s.H.NumLogErrors()
	if logErrorCount > 0 {
		return fmt.Errorf("logged %d error(s): %s", logErrorCount, s.logBuff.String())
	}

	return nil
}

func (s *IntegrationTestBuilder) changeEvents() []fsnotify.Event {
	var events []fsnotify.Event
	for _, v := range s.removedFiles {
		events = append(events, fsnotify.Event{
			Name: v,
			Op:   fsnotify.Remove,
		})
	}
	for _, v := range s.renamedFiles {
		events = append(events, fsnotify.Event{
			Name: v,
			Op:   fsnotify.Rename,
		})
	}
	for _, v := range s.changedFiles {
		events = append(events, fsnotify.Event{
			Name: v,
			Op:   fsnotify.Write,
		})
	}
	for _, v := range s.createdFiles {
		events = append(events, fsnotify.Event{
			Name: v,
			Op:   fsnotify.Create,
		})
	}

	return events
}

<<<<<<< HEAD
func (s *IntegrationTestBuilder) readWorkingDir(t testing.TB, fs *hugofs.Fs, filename string) string {
	t.Helper()
	return s.readFileFromFs(t, fs.WorkingDirReadOnly, filename)
=======
func (s *IntegrationTestBuilder) readDestination(t testing.TB, fs *hugofs.Fs, filename string) string {
	t.Helper()
	return s.readFileFromFs(t, fs.Destination, filename)
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func (s *IntegrationTestBuilder) readFileFromFs(t testing.TB, fs afero.Fs, filename string) string {
	t.Helper()
	filename = filepath.Clean(filename)
	b, err := afero.ReadFile(fs, filename)
	if err != nil {
		// Print some debug info
		hadSlash := strings.HasPrefix(filename, helpers.FilePathSeparator)
		start := 0
		if hadSlash {
			start = 1
		}
		end := start + 1

		parts := strings.Split(filename, helpers.FilePathSeparator)
		if parts[start] == "work" {
			end++
		}

		s.Assert(err, qt.IsNil)

	}
	return string(b)
}

func (s *IntegrationTestBuilder) writeSource(filename, content string) {
	s.Helper()
	s.writeToFs(s.fs.Source, filename, content)
}

func (s *IntegrationTestBuilder) writeToFs(fs afero.Fs, filename, content string) {
	s.Helper()
	if err := afero.WriteFile(fs, filepath.FromSlash(filename), []byte(content), 0755); err != nil {
		s.Fatalf("Failed to write file: %s", err)
	}
}

type IntegrationTestConfig struct {
	T testing.TB

	// The files to use on txtar format, see
	// https://pkg.go.dev/golang.org/x/exp/cmd/txtar
	TxtarString string

	// Whether to simulate server mode.
	Running bool

	// Will print the log buffer after the build
	Verbose bool

	LogLevel jww.Threshold

	// Whether it needs the real file system (e.g. for js.Build tests).
	NeedsOsFS bool

<<<<<<< HEAD
	// Do not remove the temp dir after the test.
	PrintAndKeepTempDir bool

=======
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	// Whether to run npm install before Build.
	NeedsNpmInstall bool

	WorkingDir string
}
