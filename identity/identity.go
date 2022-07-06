// Copyright 2023 The Hugo Authors. All rights reserved.
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

// Package provides ways to identify values in Hugo. Used for dependency tracking etc.
package identity

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// Anonymous is an Identity that can be used when identity doesn't matter.
	Anonymous = StringIdentity("__anonymous")

	// GenghisKhan is an Identity almost everyone relates to.
	GenghisKhan = StringIdentity("__genghiskhan")
)

var baseIdentifierIncr = &IncrementByOne{}

// NewIdentityManager creates a new Manager.
func NewManager(id Identity) Manager {
	return &identityManager{
		Identity: id,
		ids:      Identities{},
	}
}

// NewManagerWithDebugEnabled creates a new Manager with debug enabled.
func NewManagerWithDebugEnabled(root Identity) Manager {
	return &identityManager{
		Identity: root,
		ids:      Identities{root: true},
		debug:    true,
	}
}

// Identities stores identity providers.
type Identities map[Identity]bool

func (ids Identities) AsSlice() []Identity {
	s := make([]Identity, len(ids))
	i := 0
	for v := range ids {
		s[i] = v
		i++
	}
	return s
}

func (ids Identities) String() string {
	var sb strings.Builder
	i := 0
	for id := range ids {
		sb.WriteString(fmt.Sprintf("[%s]", id.IdentifierBase()))
		if i < len(ids)-1 {
			sb.WriteString(", ")
		}
		i++
	}
	return sb.String()
}

type finder struct {
	probableMatch bool

	seen map[Identity]bool
}

func (f *finder) Find(id Identity, ids Identities) (Identity, bool, string) {
	return f.find(id, ids, 0)
}

func (f *finder) find(id Identity, ids Identities, depth int) (Identity, bool, string) {

	if depth > 20 {
		panic("too many levels")
	}

	if id == Anonymous {
		return nil, false, "anonymous"
	}
	if f.probableMatch && id == GenghisKhan {
		return id, true, "genghiskhan"
	}

	if _, found := ids[id]; found {
		return id, true, "direct"
	}

	for v := range ids {
		if _, found := f.seen[v]; found {
			continue
		}
		f.seen[v] = true

		id2 := unwrap(v)

		if id2 != Anonymous {
			if id2 == id {
				// TODO1 Eq interface.
				return v, true, "direct"
			}

			if f.probableMatch {
				if id2 == nil || id == nil {
					continue
				}

				if id2.IdentifierBase() == id.IdentifierBase() {
					return v, true, "base"
				}

				if pe, ok := id.(IsProbablyDependentProvider); ok && pe.IsProbablyDependent(v) {
					return v, true, "probably"
				}

				if pe, ok := v.(IsProbablyDependentProvider); ok && pe.IsProbablyDependent(id) {
					return v, true, "probably"
				}

			}
		}

		switch t := v.(type) {
		case Manager:
			if nested, found, reason := f.find(id, t.GetIdentities(), depth+1); found {
				return nested, found, reason
			}
		}
	}

	return nil, false, "not found"
}

// DependencyManagerProvider provides a manager for dependencies.
type DependencyManagerProvider interface {
	GetDependencyManager() Manager
}

// DependencyManagerProviderFunc is a function that implements the DependencyManagerProvider interface.
type DependencyManagerProviderFunc func() Manager

func (d DependencyManagerProviderFunc) GetDependencyManager() Manager {
	return d()
}

// Identity represents a thing in Hugo (a Page, a template etc.)
// Any implementation must be comparable/hashable.
type Identity interface {
	IdentifierBase() any
}

// IsProbablyDependentProvider is an optional interface for Identity.
type IsProbablyDependentProvider interface {
	IsProbablyDependent(other Identity) bool
}

// IdentityProvider can be implemented by types that isn't itself and Identity,
// usually because they're not comparable/hashable.
type IdentityProvider interface {
	GetIdentity() Identity
}

// IdentityGroupProvider can be implemented by tightly connected types.
// Current use case is Resource transformation via Hugo Pipes.
type IdentityGroupProvider interface {
	GetIdentityGroup() Identity
}

// IdentityLookupProvider provides a way to look up an Identity by name.
type IdentityLookupProvider interface {
	LookupIdentity(name string) (Identity, bool)
}

// Manager  is an Identity that also manages identities, typically dependencies.
type Manager interface {
	Identity
	GetIdentity() Identity
	GetIdentities() Identities
	AddIdentity(ids ...Identity)
	Contains(id Identity) bool
	ContainsProbably(id Identity) bool
	Reset()
}

var NoopDependencyManagerProvider = DependencyManagerProviderFunc(func() Manager { return NopManager })

type nopManager int

var NopManager = new(nopManager)

func (m *nopManager) GetIdentities() Identities {
	return nil
}

func (m *nopManager) GetIdentity() Identity {
	return nil
}

func (m *nopManager) AddIdentity(ids ...Identity) {

}

func (m *nopManager) Contains(id Identity) bool {
	return false
}

func (m *nopManager) ContainsProbably(id Identity) bool {
	return false
}

func (m *nopManager) Reset() {
}

func (m *nopManager) IdentifierBase() any {
	return ""
}

type identityManager struct {
	Identity

	debug bool

	// mu protects _changes_ to this manager,
	// reads currently assumes no concurrent writes.
	mu  sync.RWMutex
	ids Identities
}

func (im *identityManager) String() string {
	return fmt.Sprintf("IdentityManager: %s", im.Identity)
}

func (im *identityManager) GetIdentity() Identity {
	return im.Identity
}

func (im *identityManager) AddIdentity(ids ...Identity) {
	im.mu.Lock()
	for _, id := range ids {
		if id == Anonymous {
			continue
		}
		if _, found := im.ids[id]; !found {
			im.ids[id] = true
		}
	}
	im.mu.Unlock()
}

func (im *identityManager) Reset() {
	im.mu.Lock()
	im.ids = Identities{}
	im.mu.Unlock()
}

// TODO(bep) these identities are currently only read on server reloads
// so there should be no concurrency issues, but that may change.
func (im *identityManager) GetIdentities() Identities {
	return im.ids
}

func (im *identityManager) Contains(id Identity) bool {
	if im.Identity != Anonymous && id == im.Identity {
		return true
	}

	f := &finder{seen: make(map[Identity]bool)}
	_, found, _ := f.Find(id, im.ids)

	return found
}

func (im *identityManager) ContainsProbably(id Identity) bool {
	if im.Identity == GenghisKhan {
		return true
	}
	if im.Identity != Anonymous {
		if id == im.Identity {
			return true
		}
		if id.IdentifierBase() == im.Identity.IdentifierBase() {
			return true
		}
	}

	f := &finder{seen: make(map[Identity]bool), probableMatch: true}
	_, found, _ := f.Find(id, im.ids)
	return found
}

// Incrementer increments and returns the value.
// Typically used for IDs.
type Incrementer interface {
	Incr() int
}

// IncrementByOne implements Incrementer adding 1 every time Incr is called.
type IncrementByOne struct {
	counter uint64
}

func (c *IncrementByOne) Incr() int {
	return int(atomic.AddUint64(&c.counter, uint64(1)))
}

// IsNotDependent returns whether p1 is certainly not dependent on p2.
// False positives are OK (but not great).
func IsNotDependent(p1, p2 Identity) bool {
	return !isProbablyDependent(p2, p1)
}

func isProbablyDependent(p1, p2 Identity) bool {
	if p1 == Anonymous || p2 == Anonymous {
		return false
	}

	if p1 == GenghisKhan && p2 == GenghisKhan {
		return false
	}

	if p1 == p2 {
		return true
	}

	if p1 == nil || p2 == nil {
		return false
	}

	if p1.IdentifierBase() == p2.IdentifierBase() {
		return true
	}

	// Step two needs to be checked in both directions.
	if isProbablyDependentStep2(p1, p2) {
		return true
	}

	if isProbablyDependentStep2(p2, p1) {
		return true
	}

	return false
}

func isProbablyDependentStep2(p1, p2 Identity) bool {
	switch p2v := p2.(type) {
	case IsProbablyDependentProvider:
		if p2v.IsProbablyDependent(p1) {

			return true
		}
	case Manager:
		if p2v.ContainsProbably(p1) {
			return true
		}
	case DependencyManagerProvider:
		if p2v.GetDependencyManager().ContainsProbably(p1) {
			return true
		}
	}

	return false
}

// StringIdentity is an Identity that wraps a string.
type StringIdentity string

func (s StringIdentity) IdentifierBase() any {
	return string(s)
}

var (
	identityInterface                  = reflect.TypeOf((*Identity)(nil)).Elem()
	identityProviderInterface          = reflect.TypeOf((*IdentityProvider)(nil)).Elem()
	identityManagerProviderInterface   = reflect.TypeOf((*identityManager)(nil)).Elem()
	identityGroupProviderInterface     = reflect.TypeOf((*IdentityGroupProvider)(nil)).Elem()
	dependencyManagerProviderInterface = reflect.TypeOf((*DependencyManagerProvider)(nil)).Elem()
)

// WalkIdentities walks identities in v and applies cb to every identity found.
// Return true from cb to terminate.
// If deep is true, it will also walk nested Identities in any Manager found.
// It returns whether any Identity could be found.
func WalkIdentities(v any, deep bool, cb func(level int, id Identity) bool) bool {
	var seen map[Identity]bool
	if deep {
		seen = make(map[Identity]bool)

	}

	found, _ := walkIdentities(v, 0, deep, seen, cb)
	return found
}

func walkIdentities(v any, level int, deep bool, seen map[Identity]bool, cb func(level int, id Identity) bool) (found bool, quit bool) {
	if level > 5 {
		panic("too deep")
	}
	if id, ok := v.(Identity); ok {
		if deep && seen[id] {
			return
		}
		if deep {
			seen[id] = true
		}
		found = true
		if quit = cb(level, id); quit {
			return
		}
		if deep {
			if m, ok := v.(Manager); ok {
				for id := range m.GetIdentities() {
					if _, quit = walkIdentities(id, level+1, deep, seen, cb); quit {
						return
					}
				}
			}
		}
	}

	if mp, ok := v.(DependencyManagerProvider); ok {
		found = true
		m := mp.GetDependencyManager()
		if cb(level, m) {
			return
		}
		if deep {
			for id := range m.GetIdentities() {
				if _, quit = walkIdentities(id, level+1, deep, seen, cb); quit {
					return
				}
			}
		}
	}

	if id, ok := v.(IdentityProvider); ok {
		found = true
		if quit = cb(level, id.GetIdentity()); quit {
			return
		}
	}

	if id, ok := v.(IdentityGroupProvider); ok {
		found = true
		if quit = cb(level, id.GetIdentityGroup()); quit {
			return
		}
	}

	if ids, ok := v.(Identities); ok {
		found = len(ids) > 0
		for id := range ids {
			if quit = cb(level, id); quit {
				return
			}
		}
	}
	return
}

// FirstIdentity returns the first Identity in v, Anonymous if none found
func FirstIdentity(v any) Identity {
	var result Identity = Anonymous
	WalkIdentities(v, false, func(level int, id Identity) bool {
		result = id
		return true
	})

	return result
}

// WalkIdentitiesValue is the same as WalkIdentitiesValue, but it takes
// a reflect.Value.
// Note that this will not walk into a Manager's Identities.
func WalkIdentitiesValue(v reflect.Value, cb func(id Identity) bool) bool {
	if !v.IsValid() {
		return false
	}

	var found bool

	if v.Type().Implements(identityInterface) {
		found = true
		if cb(v.Interface().(Identity)) {
			return found
		}
	}

	if v.Type().Implements(dependencyManagerProviderInterface) {
		found = true
		if cb(v.Interface().(DependencyManagerProvider).GetDependencyManager()) {
			return found
		}
	}

	if v.Type().Implements(identityProviderInterface) {
		found = true
		if cb(v.Interface().(IdentityProvider).GetIdentity()) {
			return found
		}
	}

	if v.Type().Implements(identityGroupProviderInterface) {
		found = true
		if cb(v.Interface().(IdentityGroupProvider).GetIdentityGroup()) {
			return found
		}
	}
	return found
}

func unwrap(id Identity) Identity {
	idd := id
	for {
		switch t := idd.(type) {
		case IdentityProvider:
			idd = t.GetIdentity()
		default:
			return idd
		}
	}

}

// PrintIdentityInfo is used for debugging/tests only.
func PrintIdentityInfo(v any) {
	WalkIdentities(v, true, func(level int, id Identity) bool {
		fmt.Printf("%s%s (%T)\n", strings.Repeat("  ", level), id.IdentifierBase(), id)
		return false
	})

}
