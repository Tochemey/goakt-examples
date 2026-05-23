// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"fmt"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/extension"

	"github.com/tochemey/goakt-examples/v2/goakt-scrabble/scrabble"
)

const RegistryExtensionID = "scrabble_registry"

// LangBundle pairs a Language with its loaded DAWG.
type LangBundle struct {
	Lang *scrabble.Language
	Dawg *scrabble.DAWG
}

// Registry holds the loaded language bundles. Cluster-spawned RoomActors
// fetch their bundle via ctx.Extension(RegistryExtensionID).
type Registry struct {
	bundles map[string]*LangBundle
}

var _ extension.Extension = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{bundles: make(map[string]*LangBundle)}
}

func (r *Registry) ID() string { return RegistryExtensionID }

func (r *Registry) Add(bundle *LangBundle) {
	r.bundles[bundle.Lang.Code] = bundle
}

// Get returns the language bundle for code, or an error if unregistered.
func (r *Registry) Get(code string) (*LangBundle, error) {
	bundle, ok := r.bundles[code]
	if !ok {
		return nil, fmt.Errorf("scrabble: language %q not registered", code)
	}

	return bundle, nil
}

// Codes returns the registered language codes in arbitrary order. Used
// to validate JoinOrCreate.Language at lobby time.
func (r *Registry) Codes() []string {
	out := make([]string, 0, len(r.bundles))

	for code := range r.bundles {
		out = append(out, code)
	}

	return out
}

func registryFromExtension(system actor.ActorSystem) *Registry {
	for _, ext := range system.Extensions() {
		if ext.ID() == RegistryExtensionID {
			if registry, ok := ext.(*Registry); ok {
				return registry
			}
		}
	}

	return nil
}
