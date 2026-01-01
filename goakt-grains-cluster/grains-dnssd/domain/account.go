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

package domain

import "time"

type Account struct {
	accountID string
	balance   float64
	createdAt time.Time
}

func NewAccount(accountID string, balance float64, createAt time.Time) *Account {
	return &Account{
		accountID: accountID,
		balance:   balance,
		createdAt: createAt,
	}
}

func (s *Account) SetBalance(balance float64) {
	s.balance = balance
}

func (s *Account) SetCreatedAt(createdAt time.Time) {
	s.createdAt = createdAt
}

func (s *Account) AccountID() string {
	return s.accountID
}

func (s *Account) Balance() float64 {
	return s.balance
}

func (s *Account) CreatedAt() time.Time {
	return s.createdAt
}
