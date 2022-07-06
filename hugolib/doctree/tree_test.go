package doctree_test

import (
	"context"
	"fmt"
	"math/rand"
	"path"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/common/para"
	"github.com/gohugoio/hugo/hugolib/doctree"
	"github.com/google/go-cmp/cmp"
)

var eq = qt.CmpEquals(
	cmp.Comparer(func(n1, n2 *testValue) bool {
		if n1 == n2 {
			return true
		}

		return n1.ID == n2.ID && n1.Lang == n2.Lang && n1.Role == n2.Role
	}),
)

func TestTree(t *testing.T) {
	c := qt.New(t)

	zeroZero := doctree.New(
		doctree.Config[*testValue]{
			Dimensions: []int{0, 0},
			Shifter:    &testShifter{},
		},
	)

	a := &testValue{ID: "/a"}
	zeroZero.Insert("/a", a)
	ab := &testValue{ID: "/a/b"}
	zeroZero.Insert("/a/b", ab)

	c.Assert(zeroZero.Get("/a"), eq, &testValue{ID: "/a", Lang: 0, Role: 0})
	s, v := zeroZero.LongestPrefix("/a/b/c", nil)
	c.Assert(v, eq, ab)
	c.Assert(s, eq, "/a/b")

	// Change language.
	oneZero := zeroZero.Increment(0)
	c.Assert(zeroZero.Get("/a"), eq, &testValue{ID: "/a", Lang: 0, Role: 0})
	c.Assert(oneZero.Get("/a"), eq, &testValue{ID: "/a", Lang: 1, Role: 0})

	// Change role.
	oneOne := oneZero.Increment(1)
	c.Assert(zeroZero.Get("/a"), eq, &testValue{ID: "/a", Lang: 0, Role: 0})
	c.Assert(oneZero.Get("/a"), eq, &testValue{ID: "/a", Lang: 1, Role: 0})
	c.Assert(oneOne.Get("/a"), eq, &testValue{ID: "/a", Lang: 1, Role: 1})

}

func TestTreeInsert(t *testing.T) {
	c := qt.New(t)

	tree := doctree.New(
		doctree.Config[*testValue]{
			Dimensions: []int{0, 0},
			Shifter:    &testShifter{},
		},
	)

	a := &testValue{ID: "/a"}
	tree.Insert("/a", a)
	ab := &testValue{ID: "/a/b"}
	tree.Insert("/a/b", ab)

	c.Assert(tree.Get("/a"), eq, &testValue{ID: "/a", Lang: 0, Role: 0})
	c.Assert(tree.Get("/notfound"), qt.IsNil)

	ab2 := &testValue{ID: "/a/b", Lang: 0}
	v, ok := tree.Insert("/a/b", ab2)
	c.Assert(ok, qt.IsTrue)
	c.Assert(v, qt.DeepEquals, ab2)

	tree1 := tree.Increment(0)
	c.Assert(tree1.Get("/a/b"), qt.DeepEquals, &testValue{ID: "/a/b", Lang: 1})
}

func TestTreeData(t *testing.T) {
	c := qt.New(t)

	tree := doctree.New(
		doctree.Config[*testValue]{
			Dimensions: []int{0, 0},
			Shifter:    &testShifter{},
		},
	)

	tree.Insert("", &testValue{ID: "HOME"})
	tree.Insert("/a", &testValue{ID: "/a"})
	tree.Insert("/a/b", &testValue{ID: "/a/b"})
	tree.Insert("/b", &testValue{ID: "/b"})
	tree.Insert("/b/c", &testValue{ID: "/b/c"})
	tree.Insert("/b/c/d", &testValue{ID: "/b/c/d"})

	var values []string

	walkCfg := doctree.WalkConfig[*testValue]{
		Callback: func(ctx *doctree.WalkContext[*testValue], s string, t *testValue) (bool, error) {
			ctx.Data().Insert(s, map[string]any{
				"id": t.ID,
			})

			if s != "" {
				p, v := ctx.Data().LongestPrefix(path.Dir(s))
				values = append(values, fmt.Sprintf("%s:%s:%v", s, p, v))
			}
			return false, nil
		},
	}

	tree.Walk(context.TODO(), walkCfg)

	c.Assert(strings.Join(values, "|"), qt.Equals, "/a::map[id:HOME]|/a/b:/a:map[id:/a]|/b::map[id:HOME]|/b/c:/b:map[id:/b]|/b/c/d:/b/c:map[id:/b/c]")

}

func TestTreeEvents(t *testing.T) {
	c := qt.New(t)

	tree := doctree.New(
		doctree.Config[*testValue]{
			Dimensions: []int{0, 0},
			Shifter:    &testShifter{echo: true},
		},
	)

	tree.Insert("/a", &testValue{ID: "/a", Weight: 2, IsBranch: true})
	tree.Insert("/a/p1", &testValue{ID: "/a/p1", Weight: 5})
	tree.Insert("/a/p", &testValue{ID: "/a/p2", Weight: 6})
	tree.Insert("/a/s1", &testValue{ID: "/a/s1", Weight: 5, IsBranch: true})
	tree.Insert("/a/s1/p1", &testValue{ID: "/a/s1/p1", Weight: 8})
	tree.Insert("/a/s1/p1", &testValue{ID: "/a/s1/p2", Weight: 9})
	tree.Insert("/a/s1/s2", &testValue{ID: "/a/s1/s2", Weight: 6, IsBranch: true})
	tree.Insert("/a/s1/s2/p1", &testValue{ID: "/a/s1/s2/p1", Weight: 8})
	tree.Insert("/a/s1/s2/p2", &testValue{ID: "/a/s1/s2/p2", Weight: 7})

	walkCfg := doctree.WalkConfig[*testValue]{
		Callback: func(ctx *doctree.WalkContext[*testValue], s string, t *testValue) (bool, error) {
			if t.IsBranch {
				ctx.AddEventListener("weight", s, func(e *doctree.Event[*testValue]) {
					if e.Source.Weight > t.Weight {
						t.Weight = e.Source.Weight
						ctx.SendEvent(&doctree.Event[*testValue]{Source: t, Path: s, Name: "weight"})
					}

					// Reduces the amount of events bubbling up the tree. If the weight for this branch has
					// increased, that will be announced in its own event.
					e.StopPropagation()
				})
			} else {
				ctx.SendEvent(&doctree.Event[*testValue]{Source: t, Path: s, Name: "weight"})
			}

			return false, nil
		},
	}

	tree.Walk(context.TODO(), walkCfg)

	c.Assert(tree.Get("/a").Weight, eq, 9)
	c.Assert(tree.Get("/a/s1").Weight, eq, 9)
	c.Assert(tree.Get("/a/p").Weight, eq, 6)
	c.Assert(tree.Get("/a/s1/s2").Weight, eq, 8)
	c.Assert(tree.Get("/a/s1/s2/p2").Weight, eq, 7)
}

func TestTreePara(t *testing.T) {
	c := qt.New(t)

	p := para.New(4)
	r, _ := p.Start(context.Background())

	tree := doctree.New(
		doctree.Config[*testValue]{
			Dimensions: []int{0, 0},
			Shifter:    &testShifter{},
		},
	)

	for i := 0; i < 8; i++ {
		i := i
		r.Run(func() error {
			a := &testValue{ID: "/a"}
			tree.Insert("/a", a)
			ab := &testValue{ID: "/a/b"}
			tree.Insert("/a/b", ab)

			key := fmt.Sprintf("/a/b/c/%d", i)
			val := &testValue{ID: key}
			tree.Insert(key, val)
			c.Assert(tree.Get(key), eq, val)
			//s, _ := tree.LongestPrefix(key, nil)
			//c.Assert(s, eq, "/a/b")

			return nil
		})
	}

	c.Assert(r.Wait(), qt.IsNil)
}

func TestValidateKey(t *testing.T) {
	c := qt.New(t)

	c.Assert(doctree.ValidateKey(""), qt.IsNil)
	c.Assert(doctree.ValidateKey("/a/b/c"), qt.IsNil)
	c.Assert(doctree.ValidateKey("/"), qt.IsNotNil)
	c.Assert(doctree.ValidateKey("a"), qt.IsNotNil)
	c.Assert(doctree.ValidateKey("abc"), qt.IsNotNil)
	c.Assert(doctree.ValidateKey("/abc/"), qt.IsNotNil)
}

func BenchmarkDimensionsWalk(b *testing.B) {
	const numElements = 1000

	createTree := func() *doctree.Root[*testValue] {
		tree := doctree.New(
			doctree.Config[*testValue]{
				Dimensions: []int{0, 0},
				Shifter:    &testShifter{},
			},
		)

		for i := 0; i < numElements; i++ {
			lang, role := rand.Intn(2), rand.Intn(2)
			tree.Insert(fmt.Sprintf("/%d", i), &testValue{ID: fmt.Sprintf("/%d", i), Lang: lang, Role: role, Weight: i, NoCopy: true})
		}

		return tree

	}

	walkCfg := doctree.WalkConfig[*testValue]{
		Callback: func(ctx *doctree.WalkContext[*testValue], s string, t *testValue) (bool, error) {
			return false, nil
		},
	}

	for _, numElements := range []int{1000, 10000, 100000} {

		b.Run(fmt.Sprintf("Walk one dimension %d", numElements), func(b *testing.B) {
			tree := createTree()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree.Walk(context.TODO(), walkCfg)
			}
		})

		b.Run(fmt.Sprintf("Walk all dimensions %d", numElements), func(b *testing.B) {
			base := createTree()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for d1 := 0; d1 < 2; d1++ {
					for d2 := 0; d2 < 2; d2++ {
						tree := base.Shape(d1, d2)
						tree.Walk(context.TODO(), walkCfg)
					}
				}
			}
		})

	}
}

type testValue struct {
	ID   string
	Lang int
	Role int

	Weight   int
	IsBranch bool

	NoCopy bool
}

func (t *testValue) getLang() int {
	return t.Lang
}

type testShifter struct {
	echo bool
}

func (s *testShifter) Shift(n *testValue, dimension []int) (*testValue, bool) {
	if s.echo {
		return n, true
	}
	if n.NoCopy {
		if n.Lang == dimension[0] && n.Role == dimension[1] {
			return n, true
		}
		return nil, false
	}
	if len(dimension) != 2 {
		panic("invalid dimension")
	}
	c := *n
	c.Lang = dimension[0]
	c.Role = dimension[1]
	return &c, true
}

func (s *testShifter) All(n *testValue) []*testValue {
	return []*testValue{n}
}

func (s *testShifter) Dimension(n *testValue, d int) []*testValue {
	return []*testValue{n}
}

func (s *testShifter) Insert(old, new *testValue) (*testValue, bool) {
	return new, true
}
