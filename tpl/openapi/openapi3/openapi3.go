// Copyright 2021 The Hugo Authors. All rights reserved.
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

// Package openapi3 provides functions for generating OpenAPI v3 (Swagger) documentation.
package openapi3

import (
	"context"
	"fmt"
	"io"

	gyaml "github.com/ghodss/yaml"

	"errors"

	kopenapi3 "github.com/getkin/kin-openapi/openapi3"
	"github.com/gohugoio/hugo/cache/memcache"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/identity"
	"github.com/gohugoio/hugo/parser/metadecoders"
	"github.com/gohugoio/hugo/resources/resource"
)

// New returns a new instance of the openapi3-namespaced template functions.
func New(deps *deps.Deps) *Namespace {
	return &Namespace{
		cache: memcache.GetOrCreatePartition[string, *T](deps.MemCache, "tpl/openapi3", memcache.OptionsPartition{Weight: 30, ClearWhen: memcache.ClearOnChange}),
		deps:  deps,
	}
}

// Namespace provides template functions for the "openapi3".
type Namespace struct {
	cache *memcache.Partition[string, *T]
	deps  *deps.Deps
}

<<<<<<< HEAD
// OpenAPIDocument represents an OpenAPI 3 document.
type OpenAPIDocument struct {
	*kopenapi3.T
}

// Unmarshal unmarshals the given resource into an OpenAPI 3 document.
func (ns *Namespace) Unmarshal(r resource.UnmarshableResource) (*OpenAPIDocument, error) {
=======
var _ identity.IdentityGroupProvider = (*T)(nil)

// T shares cache life cycle with the other members of the same identity group.
type T struct {
	*kopenapi3.T
	identityGroup identity.Identity
}

func (t *T) GetIdentityGroup() identity.Identity {
	return t.identityGroup
}

// Unmarshal unmarshals the OpenAPI schemas in r into T.
// Note that ctx is provided by the framework.
func (ns *Namespace) Unmarshal(ctx context.Context, r resource.UnmarshableResource) (*T, error) {
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
	key := r.Key()
	if key == "" {
		return nil, errors.New("no Key set in Resource")
	}

<<<<<<< HEAD
	v, err := ns.cache.GetOrCreate(key, func() (any, error) {
		f := metadecoders.FormatFromStrings(r.MediaType().Suffixes()...)
=======
	v, err := ns.cache.GetOrCreate(ctx, key, func(string) (*T, error) {
		f := metadecoders.FormatFromMediaType(r.MediaType())
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
		if f == "" {
			return nil, fmt.Errorf("unknown media type: %q", r.MediaType())
		}

		reader, err := r.ReadSeekCloser()
		if err != nil {
			return nil, err
		}

		defer reader.Close()

		b, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}

		s := &kopenapi3.T{}
		switch f {
		case metadecoders.YAML:
			err = gyaml.Unmarshal(b, s)
		default:
			err = metadecoders.Default.UnmarshalTo(b, f, s)
		}
		if err != nil {
			return nil, err
		}

		err = kopenapi3.NewLoader().ResolveRefsIn(s, nil)

<<<<<<< HEAD
		return &OpenAPIDocument{T: s}, err
	})
	if err != nil {
		return nil, err
	}

	return v.(*OpenAPIDocument), nil
=======
		return &T{T: s, identityGroup: identity.FirstIdentity(r)}, nil

	})

	return v, err
>>>>>>> 9a9ea8ca9 (Improve content map, memory cache and dependency resolution)
}
