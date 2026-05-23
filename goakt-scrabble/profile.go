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

const ProfileStoreExtensionID = "scrabble_profile_store"

type profileSnapshot struct {
	Name        string
	GamesPlayed int
	Wins        int
	TotalScore  int
}

// profileStore is the backing for PlayerProfileGrain. Two implementations
// ship in this repo: memProfileStore (process-local map) and pgProfileStore
// (Postgres). Selection happens in main.go based on DATABASE_URL.
type profileStore interface {
	extension.Extension
	Load(ctx context.Context, id string) (snap profileSnapshot, found bool, err error)
	Save(ctx context.Context, id string, snap profileSnapshot) error
}

// memProfileStore keeps profiles in a process-local map.
//
// In cluster mode every pod gets its own copy, so profile reads only
// observe writes made on the same pod. That is fine when the affinity
// cookie keeps a player's reconnects on the same pod, but profile data
// will not survive a pod restart or a session that lands elsewhere.
// Wire DATABASE_URL to use Postgres for cross-pod persistence.
type memProfileStore struct {
	mu   sync.Mutex
	data map[string]profileSnapshot
}

var _ profileStore = (*memProfileStore)(nil)

func newMemProfileStore() *memProfileStore {
	return &memProfileStore{data: make(map[string]profileSnapshot)}
}

func (s *memProfileStore) ID() string { return ProfileStoreExtensionID }

func (s *memProfileStore) Load(_ context.Context, id string) (profileSnapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, ok := s.data[id]

	return snap, ok, nil
}

func (s *memProfileStore) Save(_ context.Context, id string, snap profileSnapshot) error {
	s.mu.Lock()
	s.data[id] = snap
	s.mu.Unlock()

	return nil
}

func profileStoreFromExtension(system actor.ActorSystem) profileStore {
	for _, ext := range system.Extensions() {
		if ext.ID() == ProfileStoreExtensionID {
			if store, ok := ext.(profileStore); ok {
				return store
			}
		}
	}

	return nil
}

// PlayerProfileGrain is one virtual actor per player id.
type PlayerProfileGrain struct {
	store profileStore
	id    string
	state profileSnapshot
}

var _ actor.Grain = (*PlayerProfileGrain)(nil)

func (g *PlayerProfileGrain) OnActivate(ctx context.Context, props *actor.GrainProps) error {
	g.id = strings.TrimPrefix(props.Identity().Name(), GrainPrefix)

	snap, ok, err := g.store.Load(ctx, g.id)
	switch {
	case err != nil:
		// Treat a load failure as "no profile yet" — the grain still
		// activates with zero state. A persistence blip degrades the
		// player to a fresh profile rather than taking them offline.
		props.ActorSystem().Logger().Warnf("profile load failed for %s: %v", g.id, err)
	case ok:
		g.state = snap
	}

	return nil
}

func (g *PlayerProfileGrain) OnDeactivate(ctx context.Context, props *actor.GrainProps) error {
	if err := g.store.Save(ctx, g.id, g.state); err != nil {
		props.ActorSystem().Logger().Warnf("profile save failed for %s: %v", g.id, err)
	}

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

// SetName is a grain-internal message that persists a display name.
type SetName struct{ Name string }
