// Copyright 2020 The Hugo Authors. All rights reserved.
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

package hugolib

import (
	"fmt"
	"io"
	"path"
	"strings"

	radix "github.com/armon/go-radix"
	"github.com/pkg/errors"
)

func newContentBranchNode(key string, n *contentNode) *contentBranchNode {
	return &contentBranchNode{
		key:           key,
		n:             n,
		resources:     &contentBranchNodeTree{nodes: newNodeTree("resources")},
		pages:         &contentBranchNodeTree{nodes: newNodeTree("pages")},
		pageResources: &contentBranchNodeTree{nodes: newNodeTree("pageResources")},
		refs:          make(map[interface{}]ordinalWeight),
	}
}

func newNodeTree(name string) nodeTree {
	// TODO(bep) configure
	return radix.New()
	//return &nodeTreeUpdateTracer{name: name, nodeTree: radix.New()}
}

// TODO1 names section vs branch
func newSectionMap(createSectionNode func(key string) *contentNode) *sectionMap {
	return &sectionMap{
		sections:          newNodeTree("sections"),
		createSectionNode: createSectionNode,
	}
}

func (m *sectionMap) debug(prefix string, w io.Writer) {
	fmt.Fprintf(w, "[%s] Start:\n", prefix)
	m.WalkBranches(func(s string, n *contentBranchNode) bool {
		fmt.Fprintf(w, "[%s] Section: %q\n", prefix, s)
		n.pages.Walk(func(s string, n *contentNode) bool {
			fmt.Fprintf(w, "\t[%s] Page: %q\n", prefix, s)
			return false
		})
		n.pageResources.Walk(func(s string, n *contentNode) bool {
			fmt.Fprintf(w, "\t[%s] Branch Resource: %q\n", prefix, s)
			return false
		})
		n.pageResources.Walk(func(s string, n *contentNode) bool {
			fmt.Fprintf(w, "\t[%s] Leaf Resource: %q\n", prefix, s)
			return false
		})
		return false
	})
}

func (m *sectionMap) WalkBranchesPrefix(prefix string, cb func(s string, n *contentBranchNode) bool) {
	m.sections.WalkPrefix(prefix, func(s string, v interface{}) bool {
		return cb(s, v.(*contentBranchNode))
	})
}

func (m *sectionMap) WalkBranches(cb func(s string, n *contentBranchNode) bool) {
	m.sections.Walk(func(s string, v interface{}) bool {
		return cb(s, v.(*contentBranchNode))
	})
}

func newSectionMapQueryKey(value string, isPrefix bool) sectionMapQueryKey {
	return sectionMapQueryKey{Value: value, isPrefix: isPrefix, isSet: true}
}

// nodeTree defines the operations we use in radix.Tree.
type nodeTree interface {
	// Read ops
	Walk(fn radix.WalkFn)
	WalkPrefix(prefix string, fn radix.WalkFn)
	LongestPrefix(s string) (string, interface{}, bool)
	Get(s string) (interface{}, bool)
	Len() int

	// Update ops.
	Insert(s string, v interface{}) (interface{}, bool)
	Delete(s string) (interface{}, bool)
	DeletePrefix(s string) int
}

type nodeTreeUpdateTracer struct {
	name string
	nodeTree
}

func (t *nodeTreeUpdateTracer) Insert(s string, v interface{}) (interface{}, bool) {
	var typeInfo string
	switch n := v.(type) {
	case *contentNode:
		typeInfo = fmt.Sprint("n")
	case *contentBranchNode:
		typeInfo = fmt.Sprintf("b:isView:%t", n.n.isView())
	}
	fmt.Printf("[%s]\t[Insert] %q %s\n", t.name, s, typeInfo)
	return t.nodeTree.Insert(s, v)

}
func (t *nodeTreeUpdateTracer) Delete(s string) (interface{}, bool) {
	fmt.Printf("[%s]\t[Delete] %q\n", t.name, s)
	return t.nodeTree.Delete(s)

}
func (t *nodeTreeUpdateTracer) DeletePrefix(s string) int {
	n := t.nodeTree.DeletePrefix(s)
	fmt.Printf("[%s]\t[DeletePrefix] %q => %d\n", t.name, s, n)
	return n
}

type contentBranchNodeTree struct {
	nodes nodeTree
}

func (t contentBranchNodeTree) WalkPrefix(prefix string, cb ...contentTreeNodeCallback) {
	cbs := newcontentTreeNodeCallbackChain(cb...)
	t.nodes.WalkPrefix(prefix, func(s string, v interface{}) bool {
		return cbs(s, v.(*contentNode))
	})
}

func (t contentBranchNodeTree) Walk(cb ...contentTreeNodeCallback) {
	cbs := newcontentTreeNodeCallbackChain(cb...)
	t.nodes.Walk(func(s string, v interface{}) bool {
		return cbs(s, v.(*contentNode))
	})
}

func (t contentBranchNodeTree) Has(s string) bool {
	_, b := t.nodes.Get(s)
	return b
}

type contentBranchNode struct {
	key           string
	n             *contentNode
	resources     *contentBranchNodeTree
	pages         *contentBranchNodeTree
	pageResources *contentBranchNodeTree

	refs map[interface{}]ordinalWeight
}

func (b *contentBranchNode) InsertPage(key string, n *contentNode) {
	mustValidateSectionMapKey(key)
	b.pages.nodes.Insert(key, n)
}

func (b *contentBranchNode) InsertResource(key string, n *contentNode) error {
	mustValidateSectionMapKey(key)

	if _, _, found := b.pages.nodes.LongestPrefix(key); !found {
		return errors.Errorf("no page found for resource %q", key)
	}

	b.pageResources.nodes.Insert(key, n)

	return nil
}

type sectionMap struct {
	// sections stores *contentBranchNode
	sections nodeTree

	createSectionNode func(key string) *contentNode
}

func (m *sectionMap) InsertResource(key string, n *contentNode) error {
	if err := validateSectionMapKey(key); err != nil {
		return err
	}

	_, v, found := m.sections.LongestPrefix(key)
	if !found {
		return errors.Errorf("no section found for resource %q", key)
	}

	v.(*contentBranchNode).resources.nodes.Insert(key, n)

	return nil
}

// InsertSection inserts or updates a section.
// TODO1 key vs spaces vs
func (m *sectionMap) InsertSection(key string, n *contentNode) *contentBranchNode {
	mustValidateSectionMapKey(key)
	if v, found := m.sections.Get(key); found {
		// Update existing.
		branch := v.(*contentBranchNode)
		branch.n = n
		return branch
	}
	if strings.Count(key, "/") > 1 {
		// Make sure we have a root section.
		s, _, found := m.sections.LongestPrefix(key)
		if !found || s == "" {
			rkey := key[:strings.Index(key[1:], "/")+1]
			// It may be a taxonomy.
			m.sections.Insert(rkey, newContentBranchNode(rkey, m.createSectionNode(rkey)))
		}
	}
	branch := newContentBranchNode(key, n)
	m.sections.Insert(key, branch)
	return branch
}

func (m *sectionMap) LongestPrefix(key string) (string, *contentBranchNode) {
	k, v, found := m.sections.LongestPrefix(key)
	if !found {
		return "", nil
	}
	return k, v.(*contentBranchNode)
}

func (m *sectionMap) Has(key string) bool {
	_, found := m.sections.Get(key)
	return found
}

func (m *sectionMap) GetBranchOrLeaf(key string) *contentNode {
	s, branch := m.LongestPrefix(key)
	if branch != nil {
		if key == s {
			// A branch node.
			return branch.n
		}
		n, found := branch.pages.nodes.Get(key)
		if found {
			return n.(*contentNode)
		}
	}

	// Not  found.
	return nil
}

func (m *sectionMap) GetLeaf(key string) *contentNode {
	_, branch := m.LongestPrefix(key)
	if branch != nil {
		n, found := branch.pages.nodes.Get(key)
		if found {
			return n.(*contentNode)
		}
	}
	// Not  found.
	return nil
}

var noTaxonomiesFilter = func(s string, n *contentNode) bool {
	return n != nil && n.isView()
}

func (m *sectionMap) WalkPagesAllPrefixSection(
	prefix string,
	branchExclude, exclude contentTreeNodeCallback,
	callback contentTreeOwnerBranchNodeCallback) error {

	q := sectionMapQuery{
		BranchExclude: branchExclude,
		Exclude:       exclude,
		Branch: sectionMapQueryCallBacks{
			Key:  newSectionMapQueryKey(prefix, true),
			Page: callback,
		},
		Leaf: sectionMapQueryCallBacks{
			Page: callback,
		},
	}
	return m.Walk(q)
}

func (m *sectionMap) WalkPagesLeafsPrefixSection(
	prefix string,
	branchExclude, exclude contentTreeNodeCallback,
	callback contentTreeOwnerBranchNodeCallback) error {

	q := sectionMapQuery{
		BranchExclude: branchExclude,
		Exclude:       exclude,
		Branch: sectionMapQueryCallBacks{
			Key:  newSectionMapQueryKey(prefix, true),
			Page: nil,
		},
		Leaf: sectionMapQueryCallBacks{
			Page: callback,
		},
	}
	return m.Walk(q)
}

func (m *sectionMap) WalkPagesPrefixSectionNoRecurse(
	prefix string,
	branchExclude, exclude contentTreeNodeCallback,
	callback contentTreeOwnerBranchNodeCallback) error {

	q := sectionMapQuery{
		NoRecurse:     true,
		BranchExclude: branchExclude,
		Exclude:       exclude,
		Branch: sectionMapQueryCallBacks{
			Key:  newSectionMapQueryKey(prefix, true),
			Page: callback,
		},
		Leaf: sectionMapQueryCallBacks{
			Page: callback,
		},
	}
	return m.Walk(q)
}

func (m *sectionMap) Walk(q sectionMapQuery) error {
	if q.Branch.Key.IsZero() == q.Leaf.Key.IsZero() {
		return errors.New("must set at most one Key")
	}

	if q.Leaf.Key.IsPrefix() {
		return errors.New("prefix search is currently only implemented starting for branch keys")
	}

	if q.Exclude != nil {
		// Apply global node filters.
		applyFilterPage := func(c contentTreeOwnerBranchNodeCallback) contentTreeOwnerBranchNodeCallback {
			if c == nil {
				return nil
			}
			return func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
				if q.Exclude(s, n) {
					// Skip this node, but continue walk.
					return false
				}
				return c(branch, owner, s, n)
			}
		}

		applyFilterResource := func(c contentTreeOwnerNodeCallback) contentTreeOwnerNodeCallback {
			if c == nil {
				return nil
			}
			return func(branch *contentBranchNode, owner *contentNode, s string, n *contentNode) bool {
				if q.Exclude(s, n) {
					// Skip this node, but continue walk.
					return false
				}
				return c(branch, owner, s, n)
			}
		}

		q.Branch.Page = applyFilterPage(q.Branch.Page)
		q.Branch.Resource = applyFilterResource(q.Branch.Resource)
		q.Leaf.Page = applyFilterPage(q.Leaf.Page)
		q.Leaf.Resource = applyFilterResource(q.Leaf.Resource)

	}

	if q.BranchExclude != nil {
		cb := q.Branch.Page
		q.Branch.Page = func(branch, owner *contentBranchNode, s string, n *contentNode) bool {
			if q.BranchExclude(s, n) {
				return true
			}
			return cb(branch, owner, s, n)
		}
	}

	type depthType int

	const (
		depthAll depthType = iota
		depthBranch
		depthLeaf
	)

	handleBranchPage := func(depth depthType, s string, v interface{}) bool {
		bn := v.(*contentBranchNode)

		// TODO1 check when used and only load it then.
		var parentBranch *contentBranchNode
		if s != "" {
			d := path.Dir(s)
			_, parentBranch = m.LongestPrefix(d)
		}

		if depth <= depthBranch {
			if q.Branch.Page != nil && q.Branch.Page(parentBranch, bn, s, bn.n) {
				return false
			}

			if q.Branch.Resource != nil {
				bn.resources.nodes.Walk(func(s string, v interface{}) bool {
					// Note: We're passing the owning branch as the branch
					// to this branch's resources.
					return q.Branch.Resource(bn, bn.n, s, v.(*contentNode))
				})
			}
		}

		if q.OnlyBranches || depth == depthBranch {
			return false
		}

		if q.Leaf.Page != nil || q.Leaf.Resource != nil {
			bn.pages.nodes.Walk(func(s string, v interface{}) bool {
				n := v.(*contentNode)
				if q.Leaf.Page != nil && q.Leaf.Page(bn, bn, s, n) {
					return true
				}
				if q.Leaf.Resource != nil {
					// Interleave the Page's resources.
					bn.pageResources.nodes.WalkPrefix(s+"/", func(s string, v interface{}) bool {
						return q.Leaf.Resource(bn, n, s, v.(*contentNode))
					})
				}
				return false
			})
		}

		return false
	}

	if !q.Branch.Key.IsZero() {
		// Filter by section.
		if q.Branch.Key.IsPrefix() {

			if q.Branch.Key.Value != "" && q.Leaf.Page != nil {
				// Need to include the leaf pages of the owning branch.
				s := q.Branch.Key.Value[:len(q.Branch.Key.Value)-1]
				owner := m.Get(s)
				if owner != nil {
					if handleBranchPage(depthLeaf, s, owner) {
						// Done.
						return nil
					}
				}
			}

			var level int
			if q.NoRecurse {
				level = strings.Count(q.Branch.Key.Value, "/")
			}
			m.sections.WalkPrefix(
				q.Branch.Key.Value, func(s string, v interface{}) bool {
					if q.NoRecurse && strings.Count(s, "/") > level {
						return false
					}

					depth := depthAll
					if q.NoRecurse {
						depth = depthBranch
					}

					return handleBranchPage(depth, s, v)
				},
			)

			// Done.
			return nil
		}

		// Exact match.
		section := m.Get(q.Branch.Key.Value)
		if section != nil {
			if handleBranchPage(depthAll, q.Branch.Key.Value, section) {
				return nil
			}

		}
		// Done.
		return nil
	}

	if q.OnlyBranches || q.Leaf.Key.IsZero() || !q.Leaf.HasCallback() {
		// Done.
		return nil
	}

	_, section := m.LongestPrefix(q.Leaf.Key.Value)
	if section == nil {
		return nil
	}

	// Exact match.
	v, found := section.pages.nodes.Get(q.Leaf.Key.Value)
	if !found {
		return nil
	}

	if q.Leaf.Page != nil && q.Leaf.Page(section, section, q.Leaf.Key.Value, v.(*contentNode)) {
		return nil
	}

	if q.Leaf.Resource != nil {
		section.pageResources.nodes.WalkPrefix(q.Leaf.Key.Value+"/", func(s string, v interface{}) bool {
			return q.Leaf.Resource(section, section.n, s, v.(*contentNode))
		})
	}

	return nil
}

func (m *sectionMap) Get(key string) *contentBranchNode {
	v, found := m.sections.Get(key)
	if !found {
		return nil
	}
	return v.(*contentBranchNode)
}

// Returns
// 0 if s2 is a descendant of s1
// 1 if s2 is a sibling of s1
// else -1
func (m *sectionMap) treeRelation(s1, s2 string) int {

	if s1 == "" && s2 != "" {
		return 0
	}

	if strings.HasPrefix(s1, s2) {
		return 0
	}

	for {
		s2 = s2[:strings.LastIndex(s2, "/")]
		if s2 == "" {
			break
		}

		if s1 == s2 {
			return 0
		}

		if strings.HasPrefix(s1, s2) {
			return 1
		}
	}

	return -1
}

func (m *sectionMap) splitKey(k string) []string {
	if k == "" || k == "/" {
		return nil
	}

	return strings.Split(k, "/")[1:]
}

type sectionMapQuery struct {
	// Restrict query to one level.
	NoRecurse bool
	// Do not navigate down to the leaf nodes.
	OnlyBranches bool
	// Global node filter. Return true to skip.
	Exclude contentTreeNodeCallback
	// Branch node filter. Return true to skip.
	BranchExclude contentTreeNodeCallback
	// Handle branch (sections and taxonomies) nodes.
	Branch sectionMapQueryCallBacks
	// Handle leaf nodes (pages)
	Leaf sectionMapQueryCallBacks
}

type sectionMapQueryCallBacks struct {
	Key      sectionMapQueryKey
	Page     contentTreeOwnerBranchNodeCallback
	Resource contentTreeOwnerNodeCallback
}

func (q sectionMapQueryCallBacks) HasCallback() bool {
	return q.Page != nil || q.Resource != nil
}

type sectionMapQueryKey struct {
	Value string

	isSet    bool
	isPrefix bool
}

func (q sectionMapQueryKey) Eq(key string) bool {
	if q.IsZero() || q.isPrefix {
		return false
	}
	return q.Value == key
}

func (q sectionMapQueryKey) IsPrefix() bool {
	return !q.IsZero() && q.isPrefix
}

func (q sectionMapQueryKey) IsZero() bool {
	return !q.isSet
}

func mustValidateSectionMapKey(key string) {
	if err := validateSectionMapKey(key); err != nil {
		panic(err)
	}
}

func validateSectionMapKey(key string) error {
	if key == sectionHomeKey {
		return nil
	}

	if len(key) < 2 {
		return errors.Errorf("too short key: %q", key)
	}

	if key[0] != '/' {
		return errors.Errorf("key must start with '/': %q", key)
	}

	if key[len(key)-1] == '/' {
		return errors.Errorf("key must not end with '/': %q", key)
	}

	return nil
}
