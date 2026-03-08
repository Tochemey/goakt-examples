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

const (
	TransferStatusPending      = "pending"
	TransferStatusCompleted    = "completed"
	TransferStatusFailed       = "failed"
	TransferStatusCompensating = "compensating"
)

type Transfer struct {
	transferID    string
	fromAccountID string
	toAccountID   string
	amount        float64
	status        string
	reason        string
	createdAt     time.Time
	updatedAt     time.Time
}

func NewTransfer(transferID, fromAccountID, toAccountID string, amount float64) *Transfer {
	now := time.Now()
	return &Transfer{
		transferID:    transferID,
		fromAccountID: fromAccountID,
		toAccountID:   toAccountID,
		amount:        amount,
		status:        TransferStatusPending,
		createdAt:     now,
		updatedAt:     now,
	}
}

func (t *Transfer) TransferID() string    { return t.transferID }
func (t *Transfer) FromAccountID() string { return t.fromAccountID }
func (t *Transfer) ToAccountID() string   { return t.toAccountID }
func (t *Transfer) Amount() float64       { return t.amount }
func (t *Transfer) Status() string        { return t.status }
func (t *Transfer) Reason() string        { return t.reason }
func (t *Transfer) CreatedAt() time.Time  { return t.createdAt }
func (t *Transfer) UpdatedAt() time.Time  { return t.updatedAt }

func (t *Transfer) SetStatus(status string) {
	t.status = status
	t.updatedAt = time.Now()
}

func (t *Transfer) SetReason(reason string) {
	t.reason = reason
}

// NewTransferFromPersistence restores a transfer from persistence (all fields)
func NewTransferFromPersistence(transferID, fromAccountID, toAccountID, status, reason string, amount float64, createdAt, updatedAt time.Time) *Transfer {
	return &Transfer{
		transferID:    transferID,
		fromAccountID: fromAccountID,
		toAccountID:   toAccountID,
		amount:        amount,
		status:        status,
		reason:        reason,
		createdAt:     createdAt,
		updatedAt:     updatedAt,
	}
}
