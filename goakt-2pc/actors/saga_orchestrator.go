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

package actors

import (
	"fmt"
	"time"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-saga/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-saga/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-saga/persistence"
)

const askTimeout = 10 * time.Second

// SagaOrchestrator coordinates money transfers using the saga pattern
type SagaOrchestrator struct {
	storage persistence.Store
}

var _ goakt.Actor = (*SagaOrchestrator)(nil)

// NewSagaOrchestrator creates a new saga orchestrator
func NewSagaOrchestrator() *SagaOrchestrator {
	return &SagaOrchestrator{}
}

// PreStart initializes the orchestrator
func (x *SagaOrchestrator) PreStart(ctx *goakt.Context) error {
	x.storage = ctx.Extension(persistence.PostgresStateStoreID).(persistence.Store)
	return nil
}

// PostStop is a no-op for the saga orchestrator (transfer state is persisted during execution)
func (x *SagaOrchestrator) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles saga messages
func (x *SagaOrchestrator) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.StartTransfer:
		x.handleStartTransfer(ctx, msg)
	case *messages.GetTransferStatus:
		x.handleGetTransferStatus(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (x *SagaOrchestrator) handleStartTransfer(ctx *goakt.ReceiveContext, msg *messages.StartTransfer) {
	transfer := domain.NewTransfer(msg.TransferID, msg.FromAccountID, msg.ToAccountID, msg.Amount)
	if err := x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer); err != nil {
		ctx.Logger().Errorf("failed to persist transfer state: %v", err)
		ctx.Response(&messages.TransferFailed{TransferID: msg.TransferID, Reason: err.Error()})
		return
	}

	system := ctx.ActorSystem()

	// Step 1: Debit source account
	fromPid, err := x.getOrSpawnAccount(ctx, system, msg.FromAccountID)
	if err != nil {
		x.failTransfer(ctx, msg.TransferID, fmt.Sprintf("failed to get from account: %v", err))
		return
	}

	debitReply, err := goakt.Ask(ctx.Context(), fromPid, &messages.DebitAccount{
		AccountID: msg.FromAccountID,
		Amount:    msg.Amount,
	}, askTimeout)
	if err != nil {
		x.failTransfer(ctx, msg.TransferID, fmt.Sprintf("debit failed: %v", err))
		return
	}

	if _, ok := debitReply.(error); ok {
		transfer.SetStatus(domain.TransferStatusFailed)
		transfer.SetReason("insufficient funds")
		_ = x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer)
		ctx.Response(&messages.TransferFailed{TransferID: msg.TransferID, Reason: "insufficient funds"})
		return
	}

	// Step 2: Credit destination account
	toPid, err := x.getOrSpawnAccount(ctx, system, msg.ToAccountID)
	if err != nil {
		x.compensateAndFail(ctx, fromPid, msg.FromAccountID, msg.Amount, msg.TransferID, fmt.Sprintf("failed to get to account: %v", err))
		return
	}

	creditReply, err := goakt.Ask(ctx.Context(), toPid, &messages.CreditAccount{
		AccountID: msg.ToAccountID,
		Amount:    msg.Amount,
	}, askTimeout)
	if err != nil {
		x.compensateAndFail(ctx, fromPid, msg.FromAccountID, msg.Amount, msg.TransferID, fmt.Sprintf("credit failed: %v", err))
		return
	}

	_ = creditReply // success

	// Saga completed successfully
	transfer.SetStatus(domain.TransferStatusCompleted)
	_ = x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer)
	ctx.Response(&messages.TransferCompleted{TransferID: msg.TransferID})
}

func (x *SagaOrchestrator) compensateAndFail(ctx *goakt.ReceiveContext, fromPid *goakt.PID, fromAccountID string, amount float64, transferID, reason string) {
	ctx.Logger().Infof("compensating: crediting back %s with %f", fromAccountID, amount)
	transfer, _ := x.storage.GetTransferState(ctx.Context(), transferID)
	if transfer != nil {
		transfer.SetStatus(domain.TransferStatusCompensating)
		_ = x.storage.WriteTransferState(ctx.Context(), transferID, transfer)
	}

	_, _ = goakt.Ask(ctx.Context(), fromPid, &messages.CreditAccount{
		AccountID: fromAccountID,
		Amount:    amount,
	}, askTimeout)

	if transfer != nil {
		transfer.SetStatus(domain.TransferStatusFailed)
		transfer.SetReason(reason)
		_ = x.storage.WriteTransferState(ctx.Context(), transferID, transfer)
	}
	ctx.Response(&messages.TransferFailed{TransferID: transferID, Reason: reason})
}

func (x *SagaOrchestrator) failTransfer(ctx *goakt.ReceiveContext, transferID, reason string) {
	transfer, _ := x.storage.GetTransferState(ctx.Context(), transferID)
	if transfer != nil {
		transfer.SetStatus(domain.TransferStatusFailed)
		transfer.SetReason(reason)
		_ = x.storage.WriteTransferState(ctx.Context(), transferID, transfer)
	}
	ctx.Response(&messages.TransferFailed{TransferID: transferID, Reason: reason})
}

func (x *SagaOrchestrator) getOrSpawnAccount(ctx *goakt.ReceiveContext, system goakt.ActorSystem, accountID string) (*goakt.PID, error) {
	pid, err := system.ActorOf(ctx.Context(), accountID)
	if err == nil {
		return pid, nil
	}
	accountEntity := NewAccountEntity()
	pid, err = system.Spawn(ctx.Context(), accountID, accountEntity, goakt.WithLongLived())
	if err != nil {
		return nil, err
	}
	return pid, nil
}

func (x *SagaOrchestrator) handleGetTransferStatus(ctx *goakt.ReceiveContext, msg *messages.GetTransferStatus) {
	transfer, err := x.storage.GetTransferState(ctx.Context(), msg.TransferID)
	if err != nil {
		ctx.Response(&messages.TransferStatus{TransferID: msg.TransferID, Status: "unknown", Reason: err.Error()})
		return
	}
	if transfer == nil {
		ctx.Response(&messages.TransferStatus{TransferID: msg.TransferID, Status: "not_found"})
		return
	}
	ctx.Response(&messages.TransferStatus{
		TransferID: transfer.TransferID(),
		Status:     transfer.Status(),
		Reason:     transfer.Reason(),
	})
}
