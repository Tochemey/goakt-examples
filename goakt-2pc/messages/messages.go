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

package messages

// Account represents an account in API responses
type Account struct {
	AccountID      string
	AccountBalance float64
}

// CreateAccount is the actor command to create an account
type CreateAccount struct {
	AccountID      string
	AccountBalance float64
}

// DebitAccount is the actor command to debit an account (for transfers)
type DebitAccount struct {
	AccountID string
	Amount    float64
}

// CreditAccount is the actor command to credit an account
type CreditAccount struct {
	AccountID string
	Amount    float64
}

// GetAccount is the actor command to get an account
type GetAccount struct {
	AccountID string
}

// StartTransfer initiates a 2PC-based money transfer
type StartTransfer struct {
	TransferID    string
	FromAccountID string
	ToAccountID   string
	Amount        float64
}

// TransferCompleted is sent when the 2PC completes successfully
type TransferCompleted struct {
	TransferID string
}

// TransferFailed is sent when the 2PC fails (after abort)
type TransferFailed struct {
	TransferID string
	Reason     string
}

// GetTransferStatus queries the coordinator for transfer status
type GetTransferStatus struct {
	TransferID string
}

// TransferStatus is the response for GetTransferStatus
type TransferStatus struct {
	TransferID string
	Status     string // "preparing", "prepared", "committed", "aborted"
	Reason     string // error message when failed
}

// PrepareTransfer is Phase 1: ask participants to prepare
type PrepareTransfer struct {
	TransferID string
	AccountID  string
	Amount     float64
	IsDebit    bool // true for source (debit), false for destination (credit)
}

// CommitTransfer is Phase 2: tell participants to commit
type CommitTransfer struct {
	TransferID string
}

// AbortTransfer is Phase 2: tell participants to abort
type AbortTransfer struct {
	TransferID string
}

// VoteYes is sent by participants when they can commit
type VoteYes struct {
	TransferID string
	AccountID  string
}

// VoteNo is sent by participants when they cannot commit
type VoteNo struct {
	TransferID string
	AccountID  string
	Reason     string
}
