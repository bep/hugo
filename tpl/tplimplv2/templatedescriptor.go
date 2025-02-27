package tplimplv2

import (
	"github.com/gohugoio/hugo/resources/kinds"
)

// This is used both as the key and in lookups.
// The signature for lookups for page templates would be something like:
// LookupPageLayout(p *paths.Path, desc TemplateDescriptor)
// Where we walk up the tree until we reaches the root and return the best match.
// Note that the root ("/") represents the default layouts, the home page is stored in "/pages".
// TODO1 rename.
type TemplateDescriptor struct {
	// These are set both in lookups and in the template store.
	Kind         string // page, home, section, taxonomy, term (and only those)
	Layout       string // list, single, baseof, mycustomlayout.
	Lang         string // en, nn, fr, ...
	OutputFormat string // rss, csv ...
	MediaType    string // text/html, text/plain, ...
	Variant1     string // contextual variant, e.g. "render-link" in render hooks."
	Variant2     string // contextual variant, e.g. "id" in render-passthrough.
	IsPlainText  bool   // Whether this is a plain text template.
}

// Note that this in this setup is usually a descriptor constructed from a page,
// so we want to find the best match for that page.
func (s *TemplateStore) compareDescriptors(category Category, this, other TemplateDescriptor) int {
	weight := this.doCompare(category, other)
	if this.OutputFormat == "rss" && this.Kind == "home" {
		// fmt.Printf("%#v\n%#v\n%d\n", this, other, weight)
	}
	if weight <= 0 {
		if category == CategoryShortcode || category == CategoryPartial {
			return 1
		}

		if category == CategoryMarkup && (this.Variant1 == other.Variant1) && (this.Variant2 == other.Variant2 || this.Variant2 != "" && other.Variant2 == "") {

			// See issue 13242.
			if this.OutputFormat != other.OutputFormat && this.OutputFormat == s.opts.DefaultOutputFormat {
				return -1
			}

			return 1
		}
	}

	return weight
}

func (this TemplateDescriptor) doCompare(category Category, other TemplateDescriptor) int {
	var weight int

	if other.IsPlainText != this.IsPlainText {
		return -1
	}
	if other.Kind != "" && other.Kind != this.Kind {
		return -1
	}
	if other.Layout != "" && other.Layout != this.Layout {
		return -1
	}
	if other.Lang != "" && other.Lang != this.Lang {
		return -1
	}

	if other.OutputFormat != "" && other.OutputFormat != this.OutputFormat {
		if this.MediaType != other.MediaType {
			return -1
		}

		if this.Kind != other.Kind && this.Layout != other.Layout {
			return -1
		}

		// Continue.
	}

	// One example of variant1 and 2 is for render codeblocks:
	// variant1=codeblock, variant2=go (language).
	if other.Variant1 != "" && other.Variant1 != this.Variant1 {
		return -1
	}
	if other.Variant2 != "" && this.Variant2 != "" && other.Variant2 != this.Variant2 {
		return -1
	}

	const (
		weightKind         = 4
		weightcustomLayout = 5
		weightLayout       = 3
		weightType         = 2
		weightMediaType    = 1
		weightLang         = 1
		weightOutputFormat = 3
		weightVariant1     = 5
		weightVariant2     = 3
	)

	// Now we know that the other descriptor is a subset of this one.
	// Now calculate the weight.
	weight++

	if other.Kind != "" && other.Kind == this.Kind {
		weight += weightKind
	}

	if other.Layout != "" && other.Layout == this.Layout {
		if isCustomLayout(this.Layout) {
			weight += weightcustomLayout
		} else {
			weight += weightLayout
		}
	}

	if other.Lang != "" && other.Lang == this.Lang {
		weight += weightLang
	}

	if other.OutputFormat != "" && other.OutputFormat == this.OutputFormat {
		weight += weightOutputFormat
	}

	if other.MediaType != "" && other.MediaType == this.MediaType {
		weight += weightMediaType
	}

	return weight
}

func (d TemplateDescriptor) IsZero() bool {
	return d == TemplateDescriptor{}
}

func (this TemplateDescriptor) isKindInLayout(layout string) bool {
	if this.Kind == "" {
		return true
	}
	if this.Kind != kinds.KindPage {
		return layout != layoutSingle
	}
	return layout != layoutList
}
