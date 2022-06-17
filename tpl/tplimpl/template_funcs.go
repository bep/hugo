// Copyright 2017-present The Hugo Authors. All rights reserved.
//
// Portions Copyright The Go Authors.

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

package tplimpl

import (
	"context"
	"reflect"
	"strings"

<<<<<<< HEAD
	"github.com/gohugoio/hugo/common/hreflect"
=======
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/resources/page"
	"github.com/gohugoio/hugo/tpl"

>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
	"github.com/gohugoio/hugo/common/maps"
	"github.com/gohugoio/hugo/tpl"

	template "github.com/gohugoio/hugo/tpl/internal/go_templates/htmltemplate"
	texttemplate "github.com/gohugoio/hugo/tpl/internal/go_templates/texttemplate"

	"github.com/gohugoio/hugo/deps"

	"github.com/gohugoio/hugo/tpl/internal"

	// Init the namespaces
	_ "github.com/gohugoio/hugo/tpl/cast"
	_ "github.com/gohugoio/hugo/tpl/collections"
	_ "github.com/gohugoio/hugo/tpl/compare"
	_ "github.com/gohugoio/hugo/tpl/crypto"
	_ "github.com/gohugoio/hugo/tpl/data"
	_ "github.com/gohugoio/hugo/tpl/debug"
	_ "github.com/gohugoio/hugo/tpl/diagrams"
	_ "github.com/gohugoio/hugo/tpl/encoding"
	_ "github.com/gohugoio/hugo/tpl/fmt"
	_ "github.com/gohugoio/hugo/tpl/hugo"
	_ "github.com/gohugoio/hugo/tpl/images"
	_ "github.com/gohugoio/hugo/tpl/inflect"
	_ "github.com/gohugoio/hugo/tpl/js"
	_ "github.com/gohugoio/hugo/tpl/lang"
	_ "github.com/gohugoio/hugo/tpl/math"
	_ "github.com/gohugoio/hugo/tpl/openapi/openapi3"
	_ "github.com/gohugoio/hugo/tpl/os"
	_ "github.com/gohugoio/hugo/tpl/partials"
	_ "github.com/gohugoio/hugo/tpl/path"
	_ "github.com/gohugoio/hugo/tpl/reflect"
	_ "github.com/gohugoio/hugo/tpl/resources"
	_ "github.com/gohugoio/hugo/tpl/safe"
	_ "github.com/gohugoio/hugo/tpl/site"
	_ "github.com/gohugoio/hugo/tpl/strings"
	_ "github.com/gohugoio/hugo/tpl/templates"
	_ "github.com/gohugoio/hugo/tpl/time"
	_ "github.com/gohugoio/hugo/tpl/transform"
	_ "github.com/gohugoio/hugo/tpl/urls"
)

var (
<<<<<<< HEAD
	_                texttemplate.ExecHelper = (*templateExecHelper)(nil)
	zero             reflect.Value
	contextInterface = reflect.TypeOf((*context.Context)(nil)).Elem()
=======
	_                                  texttemplate.ExecHelper = (*templateExecHelper)(nil)
	zero                               reflect.Value
	identityInterface                  = reflect.TypeOf((*identity.Identity)(nil)).Elem()
	identityProviderInterface          = reflect.TypeOf((*identity.IdentityProvider)(nil)).Elem()
	identityLookupProviderInterface    = reflect.TypeOf((*identity.IdentityLookupProvider)(nil)).Elem()
	dependencyManagerProviderInterface = reflect.TypeOf((*identity.DependencyManagerProvider)(nil)).Elem()
	contextInterface                   = reflect.TypeOf((*context.Context)(nil)).Elem()
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
)

type templateExecHelper struct {
	running bool // whether we're in server mode.
	funcs   map[string]reflect.Value
}

func (t *templateExecHelper) GetFunc(ctx context.Context, tmpl texttemplate.Preparer, name string) (fn reflect.Value, firstArg reflect.Value, found bool) {
	if fn, found := t.funcs[name]; found {
		if fn.Type().NumIn() > 0 {
			first := fn.Type().In(0)
			if first.Implements(contextInterface) {
<<<<<<< HEAD
				// TODO(bep) check if we can void this conversion every time -- and if that matters.
=======
				// TODO1 check if we can void this conversion every time -- and if that matters.
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
				// The first argument may be context.Context. This is never provided by the end user, but it's used to pass down
				// contextual information, e.g. the top level data context (e.g. Page).
				return fn, reflect.ValueOf(ctx), true
			}
		}

		return fn, zero, true
	}
	return zero, zero, false
}

func (t *templateExecHelper) Init(ctx context.Context, tmpl texttemplate.Preparer) {
<<<<<<< HEAD
=======
	if t.running {
		t.trackDeps(ctx, tmpl, "", reflect.Value{})
	}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func (t *templateExecHelper) GetMapValue(ctx context.Context, tmpl texttemplate.Preparer, receiver, key reflect.Value) (reflect.Value, bool) {
	if params, ok := receiver.Interface().(maps.Params); ok {
		// Case insensitive.
		keystr := strings.ToLower(key.String())
		v, found := params[keystr]
		if !found {
			return zero, false
		}
		return reflect.ValueOf(v), true
	}

	v := receiver.MapIndex(key)

	return v, v.IsValid()
}

func (t *templateExecHelper) GetMethod(ctx context.Context, tmpl texttemplate.Preparer, receiver reflect.Value, name string) (method reflect.Value, firstArg reflect.Value) {
	if t.running {
<<<<<<< HEAD
		switch name {
		case "GetPage", "Render":
			if info, ok := tmpl.(tpl.Info); ok {
				if m := receiver.MethodByName(name + "WithTemplateInfo"); m.IsValid() {
					return m, reflect.ValueOf(info)
				}
			}
		}
	}

	fn := hreflect.GetMethodByName(receiver, name)
	if !fn.IsValid() {
		return zero, zero
	}

	if fn.Type().NumIn() > 0 {
		first := fn.Type().In(0)
		if first.Implements(contextInterface) {
			// The first argument may be context.Context. This is never provided by the end user, but it's used to pass down
			// contextual information, e.g. the top level data context (e.g. Page).
			return fn, reflect.ValueOf(ctx)
		}
	}

	return fn, zero
=======
		t.trackDeps(ctx, tmpl, name, receiver)
	}

	fn := receiver.MethodByName(name)
	if !fn.IsValid() {
		return zero, zero
	}

	if fn.Type().NumIn() > 0 {
		first := fn.Type().In(0)
		if first.Implements(contextInterface) {
			// The first argument may be context.Context. This is never provided by the end user, but it's used to pass down
			// contextual information, e.g. the top level data context (e.g. Page).
			return fn, reflect.ValueOf(ctx)
		}
	}

	return fn, zero
}

func (t *templateExecHelper) trackDeps(ctx context.Context, tmpl texttemplate.Preparer, name string, receiver reflect.Value) {
	if tmpl == nil {
		panic("must provide a template")
	}

	dot := ctx.Value(texttemplate.DataContextKey)

	if dot == nil {
		return
	}

	// TODO1 remove all but DependencyManagerProvider
	// idm, ok := dot.(identity.Manager)

	dp, ok := dot.(identity.DependencyManagerProvider)

	if !ok {
		// Check for .Page, as in shortcodes.
		// TODO1 remove this interface from .Page
		var pp page.PageProvider
		if pp, ok = dot.(page.PageProvider); ok {
			dp, ok = pp.Page().(identity.DependencyManagerProvider)
		}
	}

	if !ok {
		// The aliases currently have no dependency manager.
		// TODO(bep)
		return
	}

	// TODO1 bookmark1

	idm := dp.GetDependencyManager()

	if info, ok := tmpl.(identity.Identity); ok {
		idm.AddIdentity(info)
	} else {

		// TODO1 fix this re shortcodes
		id := identity.NewPathIdentity("layouts", tmpl.(tpl.Template).Name(), "", "")
		idm.AddIdentity(id)
	}

	identity.WalkIdentitiesValue(receiver, func(id identity.Identity) bool {
		idm.AddIdentity(id)
		return false
	})

	if receiver.IsValid() {
		if receiver.Type().Implements(identityLookupProviderInterface) {
			if id, found := receiver.Interface().(identity.IdentityLookupProvider).LookupIdentity(name); found {
				idm.AddIdentity(id)
			}
		}
	}
>>>>>>> cb30cc82b (Improve content map, memory cache and dependency resolution)
}

func newTemplateExecuter(d *deps.Deps) (texttemplate.Executer, map[string]reflect.Value) {
	funcs := createFuncMap(d)
	funcsv := make(map[string]reflect.Value)

	for k, v := range funcs {
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

	exeHelper := &templateExecHelper{
		running: d.Running,
		funcs:   funcsv,
	}

	return texttemplate.NewExecuter(
		exeHelper,
	), funcsv
}

func createFuncMap(d *deps.Deps) map[string]any {
	funcMap := template.FuncMap{}

	// Merge the namespace funcs
	for _, nsf := range internal.TemplateFuncsNamespaceRegistry {
		ns := nsf(d)
		if _, exists := funcMap[ns.Name]; exists {
			panic(ns.Name + " is a duplicate template func")
		}
		funcMap[ns.Name] = ns.Context
		for _, mm := range ns.MethodMappings {
			for _, alias := range mm.Aliases {
				if _, exists := funcMap[alias]; exists {
					panic(alias + " is a duplicate template func")
				}
				funcMap[alias] = mm.Method
			}
		}
	}

	if d.OverloadedTemplateFuncs != nil {
		for k, v := range d.OverloadedTemplateFuncs {
			funcMap[k] = v
		}
	}

	return funcMap
}
