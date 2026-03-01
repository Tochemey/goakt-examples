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

package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/domain"
)

const PostgresStateStoreID = "PostgresStore"

type PostgresConfig struct {
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
}

type PostgresStore struct {
	config  *PostgresConfig
	pool    *pgxpool.Pool
	connStr string
}

var _ Store = (*PostgresStore)(nil)

func NewPostgresStore(config *PostgresConfig) Store {
	postgres := new(PostgresStore)
	postgres.config = config
	postgres.connStr = createConnectionString(config.DBHost, config.DBPort, config.DBName, config.DBUser, config.DBPassword)
	return postgres
}

func (x *PostgresStore) ID() string {
	return PostgresStateStoreID
}

func (x *PostgresStore) Start(ctx context.Context) error {
	config, err := pgxpool.ParseConfig(x.connStr)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create the connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping the database connection: %w", err)
	}

	x.pool = pool
	return nil
}

func (x *PostgresStore) Stop() error {
	if x.pool == nil {
		return nil
	}
	x.pool.Close()
	return nil
}

func (x *PostgresStore) WriteState(ctx context.Context, actorID string, state *domain.Account) error {
	insertQuery := `INSERT INTO accounts (actor_id, balance) VALUES ($1, $2)
	ON CONFLICT (actor_id) DO UPDATE SET balance = EXCLUDED.balance;`
	_, err := x.pool.Exec(ctx, insertQuery, actorID, state.Balance())
	if err != nil {
		return fmt.Errorf("failed to write state for actor %s: %v", actorID, err)
	}
	return nil
}

func (x *PostgresStore) GetState(ctx context.Context, actorID string) (*domain.Account, error) {
	selectQuery := `SELECT balance, created_at FROM accounts WHERE actor_id = $1;`
	var balance float64
	var createdAt time.Time

	err := x.pool.QueryRow(ctx, selectQuery, actorID).Scan(&balance, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewAccount(actorID, 0, time.Time{}), nil
		}
		return nil, fmt.Errorf("failed to get state for actor %s: %v", actorID, err)
	}

	return domain.NewAccount(actorID, balance, createdAt), nil
}

func createConnectionString(host string, port int, name, user string, password string) string {
	info := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", host, port, user, name)
	if password != "" {
		info += fmt.Sprintf(" password=%s", password)
	}
	return info
}
