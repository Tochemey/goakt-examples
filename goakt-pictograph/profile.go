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
	"context"
	"strings"
	"sync"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/extension"
)

// ProfileStoreExtensionID is the unique identifier for the profile
// store extension. Used by `ctx.Extension(...)` lookups from actors
// that need to spawn or message profile grains.
// Extension IDs may only contain [a-zA-Z0-9_-]; no dots, no slashes.
const ProfileStoreExtensionID = "pictograph_profile_store"

//
// In-memory map persisted across grain activations. Mirrors the
// goakt-iot-twin pattern: a real deployment would back this with a DB
// or the goakt persistence extension; the demo just keeps it in RAM.

type profileSnapshot struct {
	Name        string
	GamesPlayed int
	Wins        int
	TotalScore  int
}

type profileStore struct {
	mu   sync.Mutex
	data map[string]profileSnapshot // playerID → snapshot
}

func newProfileStore() *profileStore {
	return &profileStore{data: make(map[string]profileSnapshot)}
}

// ID satisfies extension.Extension. Together with the call to
// actor.WithExtensions(store) in main.go this lets every actor in the
// system fetch the store via ctx.Extension(ProfileStoreExtensionID),
// avoiding a package-level singleton.
func (s *profileStore) ID() string { return ProfileStoreExtensionID }

var _ extension.Extension = (*profileStore)(nil)

// profileStoreFromExtension retrieves the registered store from the
// actor system. Returns nil if WithExtensions wasn't called with one —
// callers should treat that as a configuration error.
func profileStoreFromExtension(system actor.ActorSystem) *profileStore {
	for _, ext := range system.Extensions() {
		if ext.ID() == ProfileStoreExtensionID {
			if s, ok := ext.(*profileStore); ok {
				return s
			}
		}
	}
	return nil
}

func (s *profileStore) load(id string) (profileSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[id]
	return snap, ok
}

func (s *profileStore) save(id string, snap profileSnapshot) {
	s.mu.Lock()
	s.data[id] = snap
	s.mu.Unlock()
}

// PlayerProfileGrain is one virtual actor per player id. It activates
// on the first GrainIdentity lookup, restores its snapshot from the
// in-memory store, and persists the snapshot back on deactivation.
// This is the same shape as goakt-iot-twin's DeviceTwin, just applied
// to a game player rather than an IoT device.
//
// Activation lifetime: this example doesn't set WithGrainDeactivateAfter,
// so the grain stays activated until the cluster's eviction strategy
// removes it (or until system shutdown). A real deployment would pick
// a passivation window appropriate for its memory budget.
//
// In cluster mode the grain's home node is chosen by GoAkt's grain
// placement; subsequent lookups from any node route to that home.
type PlayerProfileGrain struct {
	store *profileStore
	id    string
	state profileSnapshot
}

var _ actor.Grain = (*PlayerProfileGrain)(nil)

func (g *PlayerProfileGrain) OnActivate(_ context.Context, props *actor.GrainProps) error {
	// Grain names are namespaced (GrainPrefix + playerID). Strip the
	// prefix so the in-memory store key matches what the caller sees.
	g.id = strings.TrimPrefix(props.Identity().Name(), GrainPrefix)
	if snap, ok := g.store.load(g.id); ok {
		g.state = snap
		props.ActorSystem().Logger().Debugf("[profile %s] reactivated (games=%d wins=%d)",
			g.id, g.state.GamesPlayed, g.state.Wins)
	} else {
		props.ActorSystem().Logger().Debugf("[profile %s] activated fresh", g.id)
	}
	return nil
}

func (g *PlayerProfileGrain) OnDeactivate(_ context.Context, props *actor.GrainProps) error {
	g.store.save(g.id, g.state)
	props.ActorSystem().Logger().Debugf("[profile %s] passivated (games=%d wins=%d)",
		g.id, g.state.GamesPlayed, g.state.Wins)
	return nil
}

func (g *PlayerProfileGrain) OnReceive(ctx *actor.GrainContext) {
	switch msg := ctx.Message().(type) {
	case *GetProfile:
		ctx.Response(&ProfileView{
			PlayerID:    g.id,
			Name:        g.state.Name,
			GamesPlayed: g.state.GamesPlayed,
			Wins:        g.state.Wins,
			TotalScore:  g.state.TotalScore,
		})
	case *SetName:
		// Name updates flow through the grain so the persisted record
		// reflects whatever the player typed on join; otherwise a
		// reactivated grain would forget the friendly name.
		g.state.Name = msg.Name
		ctx.NoErr()
	case *RecordGame:
		g.state.GamesPlayed++
		if msg.WonThisGame {
			g.state.Wins++
		}
		g.state.TotalScore += msg.GameScore
		ctx.NoErr()
	default:
		ctx.Unhandled()
	}
}

// SetName is a grain message that updates the persisted display name.
// Kept here rather than in types.go because it's grain-internal — no
// actor outside profile.go has reason to know about it.
type SetName struct{ Name string }
