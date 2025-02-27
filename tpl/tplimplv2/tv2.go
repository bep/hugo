package tplimplv2

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gohugoio/hugo/common/herrors"
	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/common/paths"
	"github.com/gohugoio/hugo/common/types"
	"github.com/gohugoio/hugo/helpers"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/media"
	"github.com/gohugoio/hugo/metrics"
	"github.com/gohugoio/hugo/output"
	"github.com/gohugoio/hugo/resources/kinds"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/tpl"
	htmltemplate "github.com/gohugoio/hugo/tpl/internal/go_templates/htmltemplate"
	template "github.com/gohugoio/hugo/tpl/internal/go_templates/htmltemplate"
	texttemplate "github.com/gohugoio/hugo/tpl/internal/go_templates/texttemplate"
	"github.com/gohugoio/hugo/tpl/internal/go_templates/texttemplate/parse"
	"github.com/spf13/afero"
)

const (
	CategoryLayout Category = iota
	CategoryBaseof
	CategoryMarkup
	CategoryShortcode
	CategoryPartial
	CategoryTemplates
	// Internal categories
	CategoryServer
	CategoryHugo
)

const (
	// TODO1 better name for Sub*
	SubCategoryMain     SubCategory = iota
	SubCategoryInternal             // Internal Hugo templates
	SubCategoryInline               // Inline partials
)

const (
	pagesRoot      = "/pages"
	shortcodesRoot = "/shortcodes"
	partialsRoot   = "/partials"
	templatesRoot  = "/templates"
)

// TODO1 remove me.
var (
	dodebug  = false
	dodebug2 = false
)

const (
	layoutList   = "list"
	layoutSingle = "single"
)

func NewStore(opts StoreOptions, siteOpts SiteOptions) (*TemplateStore, error) {
	html, ok := opts.OutputFormats.GetByName("html")
	if !ok {
		panic("HTML output format not found")
	}
	s := &TemplateStore{
		opts:            opts,
		siteOpts:        siteOpts,
		htmlFormat:      html,
		storeSite:       configureSiteStorage(siteOpts, opts.Watching),
		templatesTree:   doctree.NewSimpleTree[map[TemplateDescriptor]*TemplInfo](),
		templatesByPath: maps.NewCache[string, *TemplInfo](),
		// Note that the funcs passed below is just for name validation.
		tns: newTemplateNamespace(siteOpts.TemplateFuncs),
	}

	if err := s.insertTemplates(nil, false); err != nil {
		return nil, err
	}
	if err := s.insertEmbedded(); err != nil {
		return nil, err
	}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	if err := s.extractInlinePartials(); err != nil {
		return nil, err
	}
	if err := s.transformTemplates(); err != nil {
		return nil, err
	}

	if err := s.tns.createPrototypes(true); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *TemplateStore) timeSpent(what string, start time.Time) {
	fmt.Printf("[timer] %s %s\n", what, time.Since(start))
}

// RefreshFiles refreshes this store for the files matching the given predicate.
func (s *TemplateStore) RefreshFiles(include func(fi hugofs.FileMetaInfo) bool) error {
	if err := s.tns.createPrototypesParse(); err != nil {
		return err
	}
	if err := s.insertTemplates(include, true); err != nil {
		return err
	}
	if err := s.parseTemplates(); err != nil {
		return err
	}
	if err := s.extractInlinePartials(); err != nil {
		return err
	}
	if err := s.transformTemplates(); err != nil {
		return err
	}
	if err := s.tns.createPrototypes(false); err != nil {
		return err
	}

	return nil
}

//go:generate stringer -type Category

type Category int

//go:generate stringer -type SubCategory

type SubCategory int

type SiteOptions struct {
	Site          page.Site
	TemplateFuncs map[string]any
}

type StoreOptions struct {
	// The filesystem to use.
	Fs afero.Fs

	// The path parser to use.
	PathParser *paths.PathParser

	// Set when --enableTemplateMetrics is set.
	Metrics metrics.Provider

	// All configured output formats.
	OutputFormats output.Formats

	// All configured media types.
	MediaTypes media.Types

	// The default content language.
	DefaultContentLanguage string

	// The default output format.
	DefaultOutputFormat string

	// Whether we are in watch or server mode.
	Watching bool
}

var (
	_ identity.IdentityProvider             = (*TemplInfo)(nil)
	_ identity.IsProbablyDependentProvider  = (*TemplInfo)(nil)
	_ identity.IsProbablyDependencyProvider = (*TemplInfo)(nil)
)

type TemplInfo struct {
	// The category of this template.
	Category Category

	SubCategory SubCategory

	// PathInfo info.
	PathInfo *paths.Path

	// Set when backed by a file.
	Fi hugofs.FileMetaInfo

	// The template content with any leading BOM removed.
	Content string

	// The parsed template.
	// Note that any baseof template will be applied later.
	Template tpl.Template

	// If no baseof is needed, this will be set to true.
	// E.g. shortcode templates do not need a baseof.
	// TODO1 make this NeedsBaseOf.
	NoBaseOf bool

	// If NoBaseOf is false, we will look for the final template in this map.
	BaseVariants map[TemplateDescriptor]*TemplWithBaseApplied

	// The template variants that are based on this template.
	Overlays []*TemplInfo

	// The base template used, if any.
	Base *TemplInfo

	// The descriptior that this template represents.
	D TemplateDescriptor

	// Parser state.
	ParseInfo tpl.ParseInfo

	// The execution counter for this template.
	ExecutionCounter atomic.Uint64

	// processing state.
	state processingState
}

func (ti *TemplInfo) Name() string {
	return ti.Template.Name()
}

func (ti *TemplInfo) Prepare() (*texttemplate.Template, error) {
	return ti.Template.Prepare()
}

type processingState int

const (
	processingStateInitial processingState = iota
	processingStateTransformed
)

func (t *TemplInfo) IdentifierBase() string {
	if t.PathInfo == nil {
		return t.Name()
	}
	return t.PathInfo.IdentifierBase()
}

func (t *TemplInfo) GetIdentity() identity.Identity {
	return t
}

func (t *TemplInfo) IsProbablyDependent(other identity.Identity) bool {
	for _, overlay := range t.Overlays {
		if overlay.isProbablyTheSameIDAs(other) {
			return true
		}
	}
	return t.isProbablyTheSameIDAs(other)
}

func (t *TemplInfo) IsProbablyDependency(other identity.Identity) bool {
	return t.isProbablyTheSameIDAs(other)
}

func (t *TemplInfo) isProbablyTheSameIDAs(other identity.Identity) bool {
	if t.IdentifierBase() == other.IdentifierBase() {
		return true
	}

	if t.Fi != nil && t.Fi.Meta().PathInfo != t.PathInfo {
		return other.IdentifierBase() == t.Fi.Meta().PathInfo.IdentifierBase()
	}

	return false
}

type TemplWithBaseApplied struct {
	// The template that's overlaid on top of the base template.
	Overlay *TemplInfo
	// The base template.
	Base *TemplInfo
	// This is the final template that can be used to render a page.
	Template *TemplInfo
}

type TemplateStore struct {
	opts       StoreOptions
	siteOpts   SiteOptions
	htmlFormat output.Format
	// TODO1 unique/intern. Maybe
	templatesTree   *doctree.SimpleTree[map[TemplateDescriptor]*TemplInfo]
	templatesByPath *maps.Cache[string, *TemplInfo]

	// The template namespace.
	tns *templateNamespace

	// Site specific state.
	// All above this is reused.
	storeSite *storeSite

	TxtTmpl tpl.TemplateParseFinder // TODO1
}

// TODO1 rename.
func (t *TemplateStore) TextParse(name, tpl string) (*TemplInfo, error) {
	templ, err := t.tns.standaloneText.New(name).Parse(tpl)
	if err != nil {
		return nil, err
	}
	return &TemplInfo{
		Template: templ,
	}, nil
}

func (t *TemplateStore) TextLookup(name string) *TemplInfo {
	templ := t.tns.standaloneText.Lookup(name)
	if templ == nil {
		return nil
	}
	return &TemplInfo{
		Template: templ,
	}
}

func (t *TemplateStore) ExecuteWithContext(ctx context.Context, ti *TemplInfo, wr io.Writer, data any) error {
	if ti == nil || ti.Template == nil {
		panic("nil template")
	}
	defer func() {
		ti.ExecutionCounter.Add(1)
		if ti.Base != nil {
			ti.Base.ExecutionCounter.Add(1)
		}
	}()
	templ := ti.Template
	if rlocker, ok := templ.(types.RLocker); ok { // TODO1 check when this is implemented in the old setup.
		rlocker.RLock()
		defer rlocker.RUnlock()
	}
	if t.opts.Metrics != nil {
		defer t.opts.Metrics.MeasureSince(templ.Name(), time.Now())
	}

	execErr := t.storeSite.executer.ExecuteWithContext(ctx, ti, wr, data)
	if execErr != nil {
		return t.addFileContext(ti, execErr)
	}
	return nil
}

func (s *TemplateStore) GetIdentity(p string) identity.Identity {
	p = paths.AddLeadingSlash(p)
	v, found := s.templatesByPath.Get(p)
	if !found {
		return nil
	}
	getID := func(v *TemplInfo) identity.Identity {
		id := v.GetIdentity()
		// TODO1
		/*if v.Fi != nil && v.Fi.Meta().PathInfo != v.PathInfo {
			id = identity.Or(id, v.Fi.Meta().PathInfo)
		}*/
		return id
	}

	id := getID(v)

	/*for _, overlay := range v.Overlays {
		id = identity.Or(id, getID(overlay))
	}*/

	return id
}

func (s *TemplateStore) HasTemplate(templatePath string) bool {
	templatePath = paths.AddLeadingSlash(templatePath)
	return s.templatesByPath.Contains(templatePath)
}

// The identifiers may be truncated in the log, e.g.
// "executing "main" at <$scaled.SRelPermalin...>: can't evaluate field SRelPermalink in type *resource.Image"
// We need this to identify position in templates with base templates applied.
var identifiersRe = regexp.MustCompile(`at \<(.*?)(\.{3})?\>:`)

// TODO1 mvoe these private methods.
func (s *TemplateStore) addFileContext(ti *TemplInfo, inerr error) error {
	if ti.Fi == nil {
		return inerr
	}

	identifiers := s.extractIdentifiers(inerr.Error())

	checkFilename := func(fi hugofs.FileMetaInfo, inErr error) (error, bool) {
		lineMatcher := func(m herrors.LineMatcher) int {
			if m.Position.LineNumber != m.LineNumber {
				return -1
			}

			for _, id := range identifiers {
				if strings.Contains(m.Line, id) {
					// We found the line, but return a 0 to signal to
					// use the column from the error message.
					return 0
				}
			}
			return -1
		}

		f, err := fi.Meta().Open()
		if err != nil {
			return inErr, false
		}
		defer f.Close()

		fe := herrors.NewFileErrorFromName(inErr, fi.Meta().Filename)
		fe.UpdateContent(f, lineMatcher)

		if !fe.ErrorContext().Position.IsValid() {
			return inErr, false
		}
		return fe, true
	}

	inerr = fmt.Errorf("execute of template failed: %w", inerr)

	if err, ok := checkFilename(ti.Fi, inerr); ok {
		return err
	}

	return inerr
}

func (s *TemplateStore) extractIdentifiers(line string) []string {
	m := identifiersRe.FindAllStringSubmatch(line, -1)
	identifiers := make([]string, len(m))
	for i := range m {
		identifiers[i] = m[i][1]
	}
	return identifiers
}

// In the previous implementation of base templates in Hugo, we parsed and applied these base templates on
// request, e.g. in the middle of rendering. The idea was that we coulnd't know upfront which layoyt/base template
// combination that would be used.
// This, however, added a lot of complexity involving a careful dance of template cloning and parsing
// (Go HTML tenplates cannot be parsed after any of the templates in the tree have been executed).
// FindAllBaseTemplateCandidates finds all base template candidates for the given descriptor so we can apply them upfront.
// In this setup we may end up with unused base templates, but not having to do the cloning should more than make up for that.
func (s *TemplateStore) FindAllBaseTemplateCandidates(lockType doctree.LockType, dir string, desc TemplateDescriptor) []*TemplInfo {
	layoutsm := make(map[TemplateDescriptor]*TemplInfo)
	descBaseof := desc
	s.templatesTree.WalkPath(lockType, dir, func(k string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for _, vv := range v {
			if vv.Category != CategoryBaseof {
				continue
			}
			if vv.D.isKindInLayout(desc.Layout) && s.compareDescriptors(CategoryBaseof, descBaseof, vv.D) > 0 {
				// This may overwrite a match found further up in the tree.
				layoutsm[vv.D] = vv
			}
		}
		return false, nil
	})

	layouts := make([]*TemplInfo, 0, len(layoutsm))
	for _, v := range layoutsm {
		layouts = append(layouts, v)
	}
	sort.Sort(byPath(layouts))

	return layouts
}

func (t *TemplateStore) Unused() []*TemplInfo {
	var unused []*TemplInfo

	t.templatesTree.WalkPrefix(doctree.LockTypeNone, "", func(key string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for _, vv := range v {
			if vv.SubCategory != SubCategoryMain {
				// Skip inline partials and internal templates.
				continue
			}
			if vv.NoBaseOf {
				if vv.ExecutionCounter.Load() == 0 {
					unused = append(unused, vv)
				}
			} else {
				for _, vvv := range vv.BaseVariants {
					if vvv.Template.ExecutionCounter.Load() == 0 {
						unused = append(unused, vvv.Template)
					}
				}
			}

		}
		return false, nil
	})

	sort.Sort(byPath(unused))
	return unused
}

func (t *TemplateStore) GetFunc(name string) (reflect.Value, bool) {
	v, found := t.storeSite.execHelper.funcs[name]
	return v, found
}

func (t *TemplateStore) LookupByPath(templatePath string) *TemplInfo {
	v, _ := t.templatesByPath.Get(templatePath)
	return v
}

// TemplateQuery is used in LookupPagesLayout to find the best matching template.
type TemplateQuery struct {
	// The lock type to use.
	LockType doctree.LockType

	// The directory to walk down to.
	Dir string

	// The category to look in.
	Category Category

	// The template descriptor to match against.
	Desc TemplateDescriptor

	// Whether to even consider this candidate.
	Consider func(candidate *TemplInfo) bool
}

func (q *TemplateQuery) init() {
	if q.Desc.Kind == kinds.KindTemporary {
		q.Desc.Kind = ""
	} else if kinds.GetKindMain(q.Desc.Kind) == "" {
		q.Desc.Kind = ""
	}
	if q.Desc.Layout == "" && q.Desc.Kind != "" {
		if q.Desc.Kind == kinds.KindPage {
			q.Desc.Layout = layoutSingle
		} else {
			q.Desc.Layout = layoutList
		}
	}

	if q.Consider == nil {
		q.Consider = func(match *TemplInfo) bool {
			return true
		}
	}
}

func (s *TemplateStore) LookupPagesLayout(q TemplateQuery) *TemplInfo {
	q.init()
	m := s.findBestMatchWalkPath(q)

	if m != nil && !m.NoBaseOf {
		// Pick the best matching baseof template.
		var bestMatch *TemplWithBaseApplied
		for _, l := range m.BaseVariants {
			if bestMatch == nil {
				bestMatch = l
				continue
			}
			weight := s.compareDescriptors(CategoryLayout, l.Base.D, bestMatch.Base.D)
			if weight > 0 {
				bestMatch = l
			}

		}
		return bestMatch.Template
	}

	return m
}

func (s *TemplateStore) LookupPartial(pth string, desc TemplateDescriptor) *TemplInfo {
	if desc.Layout != "" {
		panic("shortcode template descriptor must not have a layout")
	}
	return s.findBestMatchGet(s.keyPartials(pth), CategoryPartial, nil, desc)
}

func (s *TemplateStore) LookupShortcode(pth string, include func(match *TemplInfo) bool, desc TemplateDescriptor) *TemplInfo {
	if desc.Layout != "" {
		panic("shortcode template descriptor must not have a layout")
	}
	k := s.keyShortcodes(pth)
	t := s.findBestMatchGet(k, CategoryShortcode, include, desc)

	return t
}

// WithSiteOpts creates a new store with the given site options.
// This is used to create per site template store, all sharing the same templates,
// but with a different template function execution context.
func (s TemplateStore) WithSiteOpts(opts SiteOptions) *TemplateStore {
	s.siteOpts = opts
	s.storeSite = configureSiteStorage(opts, s.opts.Watching)
	return &s
}

func (s *TemplateStore) findBestMatchGet(key string, category Category, consider func(candidate *TemplInfo) bool, desc TemplateDescriptor) *TemplInfo {
	key = strings.ToLower(key)

	v := s.templatesTree.Get(key)
	if v == nil {
		return nil
	}

	best := bestMatch{}

	for k, vv := range v {
		if vv.Category != category {
			continue
		}

		if consider != nil && !consider(vv) {
			continue
		}

		// Note that on tie-breakers, the depth in the tree decides.
		if weight := s.compareDescriptors(category, desc, k); weight >= best.weight || best.desc.IsZero() {
			best.weight = weight
			best.templ = vv
			best.desc = k
		}
	}

	if best.weight <= 0 {
		return nil
	}

	return best.templ
}

func (s *TemplateStore) findBestMatchWalkPath(q TemplateQuery) *TemplInfo {
	best := bestMatch{}
	key := s.keyPages(q.Dir)

	s.templatesTree.WalkPath(q.LockType, key, func(k string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for k, vv := range v {
			if vv.Category != q.Category {
				continue
			}

			if !q.Consider(vv) {
				continue
			}

			// Note that on tie-breaks, the depth in the tree decides.
			weight := s.compareDescriptors(q.Category, q.Desc, k)
			if weight > 0 {
			}
			if weight >= best.weight || best.weight == 0 {
				if weight > 0 && weight == best.weight {
					if vv.SubCategory == SubCategoryInternal {
						// Prefer user provided template on tie-break.
						continue
					}
				}
				best.weight = weight
				best.templ = vv
				best.desc = k

				if dodebug2 && best.weight > 0 {
					fmt.Println(k.Layout, k.OutputFormat, "best", best.weight, best.desc.OutputFormat)
				}
			}
		}

		return false, nil
	})

	if best.weight <= 0 {
		return nil
	}

	return best.templ
}

func (t *TemplateStore) addDeferredTemplate(owner *TemplInfo, name string, n *parse.ListNode) error {
	if _, found := t.templatesByPath.Get(name); found {
		return nil
	}

	var templ tpl.Template

	if owner.D.IsPlainText {
		prototype := t.tns.parseText // prototypeCloneIDDefer TODO1
		tt, err := prototype.New(name).Parse("")
		if err != nil {
			return fmt.Errorf("failed to parse empty text template %q: %w", name, err)
		}
		tt.Tree.Root = n
		templ = tt
	} else {
		prototype := t.tns.parseHTML
		tt, err := prototype.New(name).Parse("")
		if err != nil {
			return fmt.Errorf("failed to parse empty HTML template %q: %w", name, err)
		}
		tt.Tree.Root = n
		templ = tt
	}

	t.templatesByPath.Set(name, &TemplInfo{
		Fi:       owner.Fi,
		PathInfo: owner.PathInfo,
		D:        owner.D,
		Template: templ,
	})

	return nil
}

// TODO1 remember to sync changes in master/tplimpl before merging this.
//
//go:embed all:embedded/templates/*
var embeddedTemplatesFs embed.FS

func (s *TemplateStore) insertEmbedded() error {
	tree, unlock := s.templatesTree.LockTree(doctree.LockTypeWrite)
	defer unlock()
	return fs.WalkDir(embeddedTemplatesFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d == nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}

		templb, err := embeddedTemplatesFs.ReadFile(path)
		if err != nil {
			return err
		}

		// Get the newlines on Windows in line with how we had it back when we used Go Generate
		// to write the templates to Go files.
		templ := string(bytes.ReplaceAll(templb, []byte("\r\n"), []byte("\n")))
		name := strings.TrimPrefix(filepath.ToSlash(path), "embedded/templates/")

		insertOne := func(name, content string) error {
			pi := s.opts.PathParser.Parse(files.ComponentFolderLayouts, name)
			ti, err := s.insertTemplate(pi, nil, false, tree)
			if err != nil {
				return err
			}
			if ti != nil {
				// Currently none of the embedded templates need a baseof template.
				ti.NoBaseOf = true
				ti.Content = content
				ti.SubCategory = SubCategoryInternal
			}

			return nil
		}

		if err := insertOne(name, templ); err != nil {
			return err
		}

		if aliases, found := embeddedTemplatesAliases[name]; found {
			for _, alias := range aliases {
				if err := insertOne(alias, templ); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *TemplateStore) insertTemplate(pi *paths.Path, fi hugofs.FileMetaInfo, replace bool, tree doctree.Tree[map[TemplateDescriptor]*TemplInfo]) (*TemplInfo, error) {
	key, category, d := s.toKeyCategoryAndDescriptor(pi)

	m := tree.Get(key)
	if m == nil {
		m = make(map[TemplateDescriptor]*TemplInfo)
		tree.Insert(key, m)
	}

	if !replace {
		if _, found := m[d]; found {
			return nil, nil
		}
	}

	ti := &TemplInfo{
		PathInfo: pi,
		Fi:       fi,
		D:        d,
		Category: category,
		NoBaseOf: category > CategoryLayout,
	}

	m[d] = ti

	s.templatesByPath.Set(pi.Path(), ti)
	if fi != nil {
		if pi2 := fi.Meta().PathInfo; pi2 != pi {
			s.templatesByPath.Set(pi2.Path(), ti)
		}
	}

	return ti, nil
}

func (s *TemplateStore) insertTemplates(include func(fi hugofs.FileMetaInfo) bool, replace bool) error {
	tree, unlock := s.templatesTree.LockTree(doctree.LockTypeWrite)
	defer unlock()
	if include == nil {
		include = func(fi hugofs.FileMetaInfo) bool {
			return true
		}
	}

	// Set if we need to reset the base variants.
	var resetBaseVariants bool

	walker := func(pth string, fi hugofs.FileMetaInfo) error {
		if fi.IsDir() {
			return nil
		}

		if !include(fi) {
			return nil
		}

		pi := fi.Meta().PathInfo
		switch pi.Section() {
		case "partials", "shortcodes", "pages":
			// OK
		default:
			// Legacy value, e.g. posts/list.html, /_default/list.html, move to "pages".
			p := strings.TrimPrefix(pi.Path(), "/_default")
			pi = s.opts.PathParser.Parse(files.ComponentFolderLayouts, path.Join(pagesRoot, p))
		}

		if replace && pi.NameNoIdentifier() == "baseof" {
			// A baseof file has changed.
			resetBaseVariants = true
		}

		ti, err := s.insertTemplate(pi, fi, replace, tree)
		if err != nil || ti == nil {
			return err
		}

		if err := s.tns.readTemplateInto(ti); err != nil {
			return err
		}

		// TODO1 check old stuff below.

		/*if isDotFile(path) || isBackupFile(path) {
			return nil
		}*/

		// name := strings.TrimPrefix(filepath.ToSlash(path), "/")
		// filename := filepath.Base(path)

		// TODO1
		/*outputFormats := t.Conf.GetConfigSection("outputFormats").(output.Formats)
		outputFormat, found := outputFormats.FromFilename(filename)*/

		/*if found && outputFormat.IsPlainText {
			name = textTmplNamePrefix + name
		}*/

		return nil
	}

	if err := helpers.Walk(s.opts.Fs, "", walker); err != nil {
		if !herrors.IsNotExist(err) {
			return err
		}
		return nil
	}

	if resetBaseVariants {
		s.tns.baseofHtmlClones = nil
		s.tns.baseofTextClones = nil
		tree.WalkPrefix(doctree.LockTypeNone, "", func(key string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
			for _, vv := range v {
				if !vv.NoBaseOf {
					vv.state = processingStateInitial
				}
			}
			return false, nil
		})
	}

	return nil
}

func (s *TemplateStore) keyPages(dir string) string {
	return paths.TrimTrailing(pagesRoot + paths.AddLeadingSlash(dir))
}

func (s *TemplateStore) keyPartials(dir string) string {
	return partialsRoot + paths.AddLeadingSlash(dir)
}

func (s *TemplateStore) keyShortcodes(dir string) string {
	return shortcodesRoot + paths.AddLeadingSlash(dir)
}

func (s *TemplateStore) parseTemplates() error {
	tree, unlock := s.templatesTree.LockTree(doctree.LockTypeWrite)
	defer unlock()

	// Read and parse all templates.
	err := tree.WalkPrefix(doctree.LockTypeNone, "", func(key string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for _, vv := range v {
			if vv.state == processingStateTransformed {
				continue
			}
			if err := s.tns.parseTemplate(vv); err != nil {
				return true, err
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	// Lookup and apply base templates where needed.
	err = tree.WalkPrefix(doctree.LockTypeNone, "", func(key string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for _, vv := range v {
			if vv.state == processingStateTransformed {
				continue
			}
			if !vv.NoBaseOf {
				d := vv.D
				// Find all compatible base templates on this or a lower level.
				baseTemplates := s.FindAllBaseTemplateCandidates(doctree.LockTypeNone, key, d)
				if len(baseTemplates) == 0 {
					return true, fmt.Errorf("no base template found for %s (%#v)", key, d)
				}
				vv.BaseVariants = make(map[TemplateDescriptor]*TemplWithBaseApplied)
				for _, base := range baseTemplates {
					if err := s.tns.applyBaseTemplate(vv, base); err != nil {
						return true, err
					}
				}

			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	return err
}

// TODO1 think about _markup in all of this.
func (s *TemplateStore) toKeyCategoryAndDescriptor(p *paths.Path) (string, Category, TemplateDescriptor) {
	k := p.Dir()
	var (
		mediaType    string
		outputFormat output.Format
	)

	if ofs := p.OutputFormat(); ofs != "" {
		if of, found := s.opts.OutputFormats.GetByName(ofs); found {
			outputFormat = of
			mediaType = of.MediaType.Type
		}
	}

	if mediaType == "" {
		if ext := p.Ext(); ext != "" {
			if of, found := s.opts.OutputFormats.GetBySuffix(ext); found {
				outputFormat = of
				mediaType = of.MediaType.Type
			} else {
				if mt, _, found := s.opts.MediaTypes.GetFirstBySuffix(ext); found {
					mediaType = mt.Type
					if outputFormat.IsZero() {
						// For e.g. index.xml we will in the default confg now have the application/rss+xml  media type.
						// Try a last time to find the output format using the SubType as the name.
						// As to template resolution, this value is currently only used to
						// decide if this is a text or HTML template.
						outputFormat, _ = s.opts.OutputFormats.GetByName(mt.SubType)
					}
				}
			}
		}
	}

	if strings.Contains(p.Path(), "_default") {
		fmt.Println("Key", p.Path()) // TODO1
		panic("no defatult in path")
	}

	d := TemplateDescriptor{
		Lang:         p.Lang(),
		OutputFormat: p.OutputFormat(),
		MediaType:    mediaType,
		Kind:         p.Kind(),
		// Type:         "TODO", // p.Section(),
		Layout:      p.NameNoIdentifier(),
		IsPlainText: outputFormat.IsPlainText,
	}

	if d.Layout == d.OutputFormat {
		d.Layout = ""
	}

	if d.Kind == kinds.KindTemporary {
		d.Kind = ""
	}

	section := p.Section()

	var category Category
	if d.Layout == "baseof" {
		category = CategoryBaseof
		d.Layout = ""
	} else {
		switch section {
		case "pages", "", "_default":
			if strings.Contains(p.Path(), "_markup") {
				category = CategoryMarkup
			} else {
				category = CategoryLayout
			}
		case "shortcodes":
			category = CategoryShortcode
		case "partials":
			category = CategoryPartial
		case "templates":
			category = CategoryTemplates
		case "_hugo":
			category = CategoryHugo
		case "_server":
			category = CategoryServer
		default:
			panic("unknown category: " + p.Section())
		}
	}

	if category == CategoryPartial || category == CategoryShortcode {
		d.Layout = ""
		k = p.PathNoIdentifier()
	}

	// Legacy layout for home page.
	if d.Layout == "index" {
		if d.Kind == "" {
			d.Kind = kinds.KindHome
		}
		d.Layout = ""
	}

	if d.Layout == d.Kind {
		d.Layout = ""
	}

	if strings.HasPrefix(k, "/_default") {
		k = strings.TrimPrefix(k, "/_default")
	}

	if k == "" {
		k = "/"
	}

	if category == CategoryMarkup {
		// We store all template nodes for a given directory on the same level.
		k = strings.TrimSuffix(k, "/_markup")
		parts := strings.Split(d.Layout, "-")
		if len(parts) < 2 {
			panic("markup template must have at least 2 parts")
		}
		// Either 2 or 3 parts, e.g. render-codeblock-go.
		d.Variant1 = parts[1]
		if len(parts) > 2 {
			d.Variant2 = parts[2]
		}
		// TODO1 variant2
		d.Layout = "" // This allows using page layout as part of the key for lookups.
	}

	// TODO1 remove.
	if dodebug {
		if strings.HasPrefix(k, "/pages") {
			fmt.Println("k", k, "k", d.Kind, "l:", d.Layout, "v1:", d.Variant1, "v2:", d.Variant2, "o:", d.OutputFormat)
		}
	}

	// TODO1 baseof from layout to variant1.
	// TODO1 check sitemapindex

	// fmt.Println("K", d.Kind, "L", d.Layout, "O", d.OutputFormat)

	return k, category, d
}

func (s *TemplateStore) extractInlinePartials() error {
	tree, unlock := s.templatesTree.LockTree(doctree.LockTypeWrite)
	defer unlock()

	p := s.tns
	// We may find both inline and external partials in the current template namespaces,
	// so only add the ones we have not seen before.
	addIfNotSeen := func(isText bool, templs ...tpl.Template) error {
		for _, templ := range templs {
			if templ.Name() == "" || !strings.HasPrefix(templ.Name(), "partials/") {
				continue
			}

			// TODO1 test this vs the others.
			name := templ.Name()
			if !paths.HasExt(name) {
				// Assume HTML. This in line with how the lookup works.
				name = name + ".html"
			}
			pi := s.opts.PathParser.Parse(files.ComponentFolderLayouts, name)
			ti, err := s.insertTemplate(pi, nil, false, tree)
			if err != nil {
				return err
			}

			if ti != nil {
				ti.Template = templ
				ti.NoBaseOf = true
				ti.SubCategory = SubCategoryInline
				ti.D.IsPlainText = isText
			}

		}
		return nil
	}
	addIfNotSeen(false, p.templatesIn(p.parseHTML)...)
	addIfNotSeen(true, p.templatesIn(p.parseText)...)

	// This is unfortunate and should be improved upon. We need to clone the template when parsing the base templates.
	for _, t := range p.baseofHtmlClones {
		if err := addIfNotSeen(false, p.templatesIn(t)...); err != nil {
			return err
		}
	}
	for _, t := range p.baseofTextClones {
		if err := addIfNotSeen(true, p.templatesIn(t)...); err != nil {
			return err
		}
	}
	return nil
}

func (s *TemplateStore) transformTemplates() error {
	lookup := func(name string, in *TemplInfo) *TemplInfo { // TODO1 check if we use the entire object.
		v, found := s.templatesByPath.Get(paths.AddLeadingSlash(name)) // TODO1 text vs not.
		if found {
			// TODO1 check if this is reached.
			return v
		}

		if in.D.IsPlainText {
			templ := in.Template.(*texttemplate.Template).Lookup(name)
			if templ != nil {
				return &TemplInfo{
					Template: templ,
				}
			}
		} else {
			templ := in.Template.(*htmltemplate.Template).Lookup(name)
			if templ != nil {
				return &TemplInfo{
					Template: templ,
				}
			}
		}

		return v
	}

	err := s.templatesTree.WalkPrefix(doctree.LockTypeWrite, "", func(key string, v map[TemplateDescriptor]*TemplInfo) (bool, error) {
		for _, vv := range v {
			if vv.state == processingStateTransformed {
				continue
			}
			vv.state = processingStateTransformed
			if vv.Category == CategoryBaseof {
				continue
			}
			if !vv.NoBaseOf {
				for _, vvv := range vv.BaseVariants {
					tctx, err := applyTemplateTransformers(vvv.Template, lookup)
					if err != nil {
						return true, err
					}

					for name, node := range tctx.deferNodes {
						if err := s.addDeferredTemplate(vvv.Overlay, name, node); err != nil {
							return true, err
						}
					}

				}
			} else {
				tctx, err := applyTemplateTransformers(vv, lookup)
				if err != nil {
					return true, err
				}

				for name, node := range tctx.deferNodes {
					if err := s.addDeferredTemplate(vv, name, node); err != nil {
						return true, err
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return nil
}

type bestMatch struct {
	templ  *TemplInfo
	desc   TemplateDescriptor
	weight int
}

type byPath []*TemplInfo

func (a byPath) Len() int { return len(a) }
func (a byPath) Less(i, j int) bool {
	return a[i].PathInfo.Path() < a[j].PathInfo.Path()
}

func (a byPath) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// the parts of a template store that's set per site.
type storeSite struct {
	opts       SiteOptions
	execHelper *templateExecHelper
	executer   texttemplate.Executer
}

func isCustomLayout(s string) bool {
	if s == "" {
		return false
	}
	return s != layoutList && s != layoutSingle
}

func configureSiteStorage(opts SiteOptions, watching bool) *storeSite {
	funcsv := make(map[string]reflect.Value)

	for k, v := range opts.TemplateFuncs {
		vv := reflect.ValueOf(v)
		funcsv[k] = vv
	}

	// Duplicate Go's internal funcs here for faster lookups.
	for k, v := range template.GoFuncs {
		if _, exists := funcsv[k]; !exists {
			vv, ok := v.(reflect.Value)
			if !ok {
				vv = reflect.ValueOf(v)
			}
			funcsv[k] = vv
		}
	}

	for k, v := range texttemplate.GoFuncs {
		if _, exists := funcsv[k]; !exists {
			funcsv[k] = v
		}
	}

	s := &storeSite{
		opts: opts,
		execHelper: &templateExecHelper{
			watching:   watching,
			funcs:      funcsv,
			site:       reflect.ValueOf(opts.Site),
			siteParams: reflect.ValueOf(opts.Site.Params()),
		},
	}

	s.executer = texttemplate.NewExecuter(s.execHelper)

	return s
}

func printTemplateTree(t *TemplInfo) {
	templ := t.Template

	switch tt := templ.(type) {
	case *htmltemplate.Template:
		fmt.Println(tt.Tree.Root.String())
	case *texttemplate.Template:
		fmt.Println(tt.Tree.Root.String())
	}
}
