package tplimplv2

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/resources/kinds"
)

func TestTemplateDescriptorCompare(t *testing.T) {
	c := qt.New(t)

	less := func(category Category, this, other1, other2 TemplateDescriptor) {
		c.Helper()
		result1 := this.Compare(category, other1)
		result2 := this.Compare(category, other2)
		c.Assert(result1 < result2, qt.IsTrue, qt.Commentf("%d < %d", result1, result2))
	}

	check := func(category Category, this, other TemplateDescriptor, less bool) {
		c.Helper()
		result := this.Compare(category, other)
		if less {
			c.Assert(result < 0, qt.IsTrue, qt.Commentf("%d", result))
		} else {
			c.Assert(result >= 0, qt.IsTrue, qt.Commentf("%d", result))
		}
	}

	less(
		CategoryLayout,
		TemplateDescriptor{Kind: kinds.KindHome, Layout: "list", OutputFormat: "html"},
		TemplateDescriptor{Layout: "list", OutputFormat: "html"},
		TemplateDescriptor{Kind: kinds.KindHome, OutputFormat: "html"},
	)

	check(
		CategoryLayout,
		TemplateDescriptor{Kind: kinds.KindHome, Layout: "list", OutputFormat: "html", MediaType: "text/html"},
		TemplateDescriptor{Kind: kinds.KindHome, Layout: "list", OutputFormat: "myformat", MediaType: "text/html"},
		false,
	)

	// TODO1 more tests please.

	// Base templates.
}
