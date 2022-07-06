package hstrings

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestCommonPrefix(t *testing.T) {
	c := qt.New(t)

	c.Assert(CommonPrefix("a", "b"), qt.Equals, "")
	c.Assert(CommonPrefix("a", "a"), qt.Equals, "a")
	c.Assert(CommonPrefix("a", "ab"), qt.Equals, "a")
	c.Assert(CommonPrefix("ab", "a"), qt.Equals, "a")
	c.Assert(CommonPrefix("ab", "ab"), qt.Equals, "ab")
	c.Assert(CommonPrefix("ab", "abc"), qt.Equals, "ab")
	c.Assert(CommonPrefix("abc", "ab"), qt.Equals, "ab")
	c.Assert(CommonPrefix("abc", "abc"), qt.Equals, "abc")
	c.Assert(CommonPrefix("abc", "abcd"), qt.Equals, "abc")
	c.Assert(CommonPrefix("abcd", "abc"), qt.Equals, "abc")
}
