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
func NewManager(root Identity) Manager {
	return &identityManager{
		Identity: root,
		ids:      Identities{root: true},
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

func (ids Identities) find(depth int, probableMatch bool, id Identity) (Identity, bool) {
	if id == Anonymous {
		return nil, false
	}
	if probableMatch && id == GenghisKhan {
		return id, true
	}
	if _, found := ids[id]; found {
		return id, true
	}

	depth++

	// There may be infinite recursion in templates.
	if depth > 100 {
		// Bail out.¨
		if probableMatch {
			return GenghisKhan, true
		}
		panic("probable infinite recursion in identity search")
	}

	for id2 := range ids {
		if id2 == id {
			// TODO1 Eq interface.
			return id2, true
		}

		if probableMatch {
			if id2 == nil || id == nil {
				continue
			}

			if id2.IdentifierBase() == id.IdentifierBase() {
				return id2, true
			}

			if pe, ok := id.(IsProbablyDependentProvider); ok && pe.IsProbablyDependent(id2) {
				return id2, true
			}

			if pe, ok := id2.(IsProbablyDependentProvider); ok && pe.IsProbablyDependent(id) {
				return id2, true
			}

		}

		switch t := id2.(type) {
		case IdentitiesProvider:
			if nested, found := t.GetIdentities().find(depth, probableMatch, id); found {
				return nested, found
			}
		}
	}

	return nil, false
}

// IdentitiesProvider provides all Identities.
type IdentitiesProvider interface {
	GetIdentities() Identities
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
	IdentitiesProvider
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

	// mu protects _changes_ to this manager,
	// reads currently assumes no concurrent writes.
	mu  sync.RWMutex
	ids Identities
}

// String is used for debugging.
func (im *identityManager) String() string {
	var sb strings.Builder

	var printIDs func(ids Identities, level int)

	printIDs = func(ids Identities, level int) {
		for id := range ids {
			sb.WriteString(fmt.Sprintf("%s%s (%T)\n", strings.Repeat("  ", level), id.IdentifierBase(), id))
			if idg, ok := id.(IdentitiesProvider); ok {
				printIDs(idg.GetIdentities(), level+1)
			}
		}
	}
	sb.WriteString(fmt.Sprintf("Manager: %q\n", im.IdentifierBase()))

	printIDs(im.ids, 1)

	return sb.String()
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
	im.ids = Identities{im.Identity: true}
	im.mu.Unlock()
}

// TODO(bep) these identities are currently only read on server reloads
// so there should be no concurrency issues, but that may change.
func (im *identityManager) GetIdentities() Identities {
	return im.ids
}

func (im *identityManager) Contains(id Identity) bool {
	_, found := im.ids.find(0, false, id)
	return found
}

func (im *identityManager) ContainsProbably(id Identity) bool {
	v, found := im.ids.find(0, true, id)
	if found {
		fmt.Println("HERE?", im.ids, "=>", id, "=>", v)

	}
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
	identityInterface              = reflect.TypeOf((*Identity)(nil)).Elem()
	identityProviderInterface      = reflect.TypeOf((*IdentityProvider)(nil)).Elem()
	identityGroupProviderInterface = reflect.TypeOf((*IdentityGroupProvider)(nil)).Elem()
)

// WalkIdentities walks identities in v and applies cb to every identity found.
// Return true from cb to terminate.
// It returns whether any Identity could be found.
func WalkIdentities(v any, cb func(id Identity) bool) bool {
	var found bool
	if id, ok := v.(Identity); ok {
		found = true
		if cb(id) {
			return found
		}
	}
	if id, ok := v.(IdentityProvider); ok {
		found = true
		if cb(id.GetIdentity()) {
			return found
		}
	}
	if id, ok := v.(IdentityGroupProvider); ok {
		found = true
		if cb(id.GetIdentityGroup()) {
			return found
		}
	}
	return found
}

// FirstIdentity returns the first Identity in v, Anonymous if none found
func FirstIdentity(v any) Identity {
	var result Identity = Anonymous
	WalkIdentities(v, func(id Identity) bool {
		result = id
		return true
	})

	return result
}

// WalkIdentitiesValue is the same as WalkIdentitiesValue, but it takes
// a reflect.Value.
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
