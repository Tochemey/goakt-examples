/*
 * MIT License
 *
 * Copyright (c) 2022-2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package persistence

import (
	"context"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/tochemey/goakt/v3/extension"
	"github.com/tochemey/gopack/postgres"

	"github.com/tochemey/goakt-examples/v2/grains-cluster/grains-dnssd/domain"
)

const StateStoreID = "PostgresStore"

type Store interface {
	extension.Extension
	Start(ctx context.Context) error
	WriteState(ctx context.Context, state *domain.Account) error
	GetState(ctx context.Context, actorID string) (*domain.Account, error)
	Stop(ctx context.Context) error
}

type PostgresStore struct {
	db postgres.Postgres
	sb sq.StatementBuilderType
}

var _ Store = (*PostgresStore)(nil)

func NewPostgresStore() Store {
	// load the database configuration from environment variables
	config := LoadConfig()
	// create the database connection
	db := postgres.New(config)
	// create the instance and return it
	return &PostgresStore{
		db: db,
		sb: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}

func (x *PostgresStore) ID() string {
	return StateStoreID
}

func (x *PostgresStore) Start(ctx context.Context) error {
	return x.db.Connect(ctx)
}

func (x *PostgresStore) WriteState(ctx context.Context, account *domain.Account) error {
	if account == nil {
		return errors.New("nil account")
	}

	txRunner, err := postgres.NewTxRunner(ctx, x.db)
	if err != nil {
		return err
	}

	runner := txRunner.
		AddSQLBuilder(&deleteStmt{account}).
		AddSQLBuilder(&insertionStateStmt{account})

	if err = runner.Run(); err != nil {
		return err
	}
	return nil
}

func (x *PostgresStore) GetState(ctx context.Context, accountID string) (*domain.Account, error) {
	statement := x.sb.
		Select(
			"account_id",
			"account_balance").
		From("accounts").
		Where(sq.Eq{"account_id": accountID})

	// get the sql statement and the arguments
	query, args, err := statement.ToSql()
	// handle the error
	if err != nil {
		return nil, err
	}

	account := new(domain.Account)
	if err := x.db.Select(ctx, &account, query, args...); err != nil {
		return nil, err
	}

	return account, nil
}

func (x *PostgresStore) Stop(ctx context.Context) error {
	return x.db.Disconnect(ctx)
}

type deleteStmt struct {
	account *domain.Account
}

func (s deleteStmt) ToSQL() (sqlStatement string, args []any, err error) {
	sqlStatement, args, err = sq.
		StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Delete("accounts").
		Where(sq.Eq{"account_id": s.account.AccountID()}).
		ToSql()
	return
}

type insertionStateStmt struct {
	account *domain.Account
}

func (s insertionStateStmt) ToSQL() (sqlStatement string, args []any, err error) {
	sqlStatement, args, err = sq.
		StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert("accounts").
		Columns(
			"account_id",
			"account_balance").
		Values(
			s.account.AccountID(),
			s.account.Balance(),
		).
		ToSql()
	return
}
