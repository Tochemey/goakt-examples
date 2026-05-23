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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgProfileStore persists PlayerProfileGrain state in Postgres so that
// profile data survives pod restarts and lookups for a given player id
// return the same view from any pod in the cluster.
type pgProfileStore struct {
	pool *pgxpool.Pool
}

var _ profileStore = (*pgProfileStore)(nil)

const profileSchema = `
CREATE TABLE IF NOT EXISTS player_profiles (
    id           TEXT        PRIMARY KEY,
    name         TEXT        NOT NULL DEFAULT '',
    games_played INTEGER     NOT NULL DEFAULT 0,
    wins         INTEGER     NOT NULL DEFAULT 0,
    total_score  INTEGER     NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// newPgProfileStore connects to Postgres using the libpq-style URL,
// runs the idempotent schema migration, and returns a store ready
// for use. The caller owns lifecycle: call Close on shutdown.
func newPgProfileStore(ctx context.Context, dsn string) (*pgProfileStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if _, err := pool.Exec(ctx, profileSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &pgProfileStore{pool: pool}, nil
}

func (s *pgProfileStore) ID() string { return ProfileStoreExtensionID }

func (s *pgProfileStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *pgProfileStore) Load(ctx context.Context, id string) (profileSnapshot, bool, error) {
	const q = `SELECT name, games_played, wins, total_score FROM player_profiles WHERE id = $1`

	var snap profileSnapshot
	err := s.pool.QueryRow(ctx, q, id).Scan(&snap.Name, &snap.GamesPlayed, &snap.Wins, &snap.TotalScore)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return profileSnapshot{}, false, nil
	case err != nil:
		return profileSnapshot{}, false, err
	default:
		return snap, true, nil
	}
}

func (s *pgProfileStore) Save(ctx context.Context, id string, snap profileSnapshot) error {
	const q = `
INSERT INTO player_profiles (id, name, games_played, wins, total_score, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (id) DO UPDATE SET
    name         = EXCLUDED.name,
    games_played = EXCLUDED.games_played,
    wins         = EXCLUDED.wins,
    total_score  = EXCLUDED.total_score,
    updated_at   = now();`

	_, err := s.pool.Exec(ctx, q, id, snap.Name, snap.GamesPlayed, snap.Wins, snap.TotalScore)

	return err
}
