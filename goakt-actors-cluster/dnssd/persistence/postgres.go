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

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd/domain"
)

const PostgresStateStoreID = "PostgresStore"
const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	actor_id VARCHAR(255) NOT NULL PRIMARY KEY,
	balance NUMERIC(19, 2) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

type PostgresConfig struct {
	DBHost     string // DBHost represents the database host
	DBPort     int    // DBPort is the database port
	DBName     string // DBName is the database name
	DBUser     string // DBUser is the database user used to connect
	DBPassword string // DBPassword is the database password
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

// ID implements Store.
func (x *PostgresStore) ID() string {
	return PostgresStateStoreID
}

// Start starts the store and establishes the database connection pool
func (x *PostgresStore) Start(ctx context.Context) error {
	// create the connection config
	config, err := pgxpool.ParseConfig(x.connStr)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	// connect to the pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create the connection pool: %w", err)
	}

	// let us test the connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping the database connection: %w", err)
	}

	// create the schema if it does not exist
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return fmt.Errorf("failed to create the schema: %w", err)
	}

	// set the db handle
	x.pool = pool
	return nil
}

// Stop stops the store and releases any resources
// This should be called when the application is shutting down
// to ensure all connections are properly closed.
func (x *PostgresStore) Stop() error {
	if x.pool == nil {
		return nil
	}
	x.pool.Close()
	return nil
}

// WriteState implements Store.
func (x *PostgresStore) WriteState(ctx context.Context, actorID string, state *domain.Account) error {
	insertQuery := `INSERT INTO accounts (actor_id, balance) VALUES ($1, $2)
	ON CONFLICT (actor_id) DO UPDATE SET balance = EXCLUDED.balance;`
	_, err := x.pool.Exec(ctx, insertQuery, actorID, state.Balance)
	if err != nil {
		return fmt.Errorf("failed to write state for actor %s: %v\n", actorID, err)
	}
	return nil
}

// GetState implements Store.
func (x *PostgresStore) GetState(ctx context.Context, actorID string) (*domain.Account, error) {
	// prepare the select query
	selectQuery := `SELECT balance, created_at FROM accounts WHERE actor_id = $1;`
	var balance float64
	var createdAt time.Time

	// execute the query
	err := x.pool.QueryRow(ctx, selectQuery, actorID).Scan(&balance, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// return a new account with zero balance if no rows found
			return domain.NewAccount(actorID, 0, time.Time{}), nil
		}

		return nil, fmt.Errorf("failed to get state for actor %s: %v\n", actorID, err)
	}

	// return the retrieved state
	return domain.NewAccount(actorID, balance, createdAt), nil
}

func createConnectionString(host string, port int, name, user string, password string) string {
	info := fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable", host, port, user, name)
	// The Postgres driver gets confused in cases where the user has no password
	// set but a password is passed, so only set password if its non-empty
	if password != "" {
		info += fmt.Sprintf(" password=%s", password)
	}

	return info
}
