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

	"github.com/tochemey/goakt-examples/v2/goakt-2pc/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/persistence"
)

const askTimeout = 10 * time.Second

// Coordinator coordinates money transfers using the 2 Phase Commit pattern
type Coordinator struct {
	storage persistence.Store
}

var _ goakt.Actor = (*Coordinator)(nil)

// NewCoordinator creates a new 2PC coordinator
func NewCoordinator() *Coordinator {
	return &Coordinator{}
}

// PreStart initializes the coordinator
func (x *Coordinator) PreStart(ctx *goakt.Context) error {
	x.storage = ctx.Extension(persistence.PostgresStateStoreID).(persistence.Store)
	return nil
}

// PostStop is a no-op for the coordinator (transfer state is persisted during execution)
func (x *Coordinator) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles coordinator messages
func (x *Coordinator) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.StartTransfer:
		x.handleStartTransfer(ctx, msg)
	case *messages.GetTransferStatus:
		x.handleGetTransferStatus(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (x *Coordinator) handleStartTransfer(ctx *goakt.ReceiveContext, msg *messages.StartTransfer) {
	transfer := domain.NewTransfer(msg.TransferID, msg.FromAccountID, msg.ToAccountID, msg.Amount)
	if err := x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer); err != nil {
		ctx.Logger().Errorf("failed to persist transfer state: %v", err)
		ctx.Response(&messages.TransferFailed{TransferID: msg.TransferID, Reason: err.Error()})
		return
	}

	system := ctx.ActorSystem()

	// Phase 1: Prepare - Get participants and send prepare requests
	fromPid, err := x.getOrSpawnAccount(ctx, system, msg.FromAccountID)
	if err != nil {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("failed to get from account: %v", err))
		return
	}

	toPid, err := x.getOrSpawnAccount(ctx, system, msg.ToAccountID)
	if err != nil {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("failed to get to account: %v", err))
		return
	}

	// Update status to prepared
	transfer.SetStatus(domain.TransferStatusPrepared)
	if err := x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer); err != nil {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("failed to persist prepared state: %v", err))
		return
	}

	// Send prepare to source account (debit)
	fromReply, err := goakt.Ask(ctx.Context(), fromPid, &messages.PrepareTransfer{
		TransferID: msg.TransferID,
		AccountID:  msg.FromAccountID,
		Amount:     msg.Amount,
		IsDebit:    true,
	}, askTimeout)
	if err != nil {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("prepare failed for source account: %v", err))
		return
	}

	// Check if source voted NO
	if voteNo, ok := fromReply.(*messages.VoteNo); ok {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("source account voted NO: %s", voteNo.Reason))
		return
	}

	// Send prepare to destination account (credit)
	toReply, err := goakt.Ask(ctx.Context(), toPid, &messages.PrepareTransfer{
		TransferID: msg.TransferID,
		AccountID:  msg.ToAccountID,
		Amount:     msg.Amount,
		IsDebit:    false,
	}, askTimeout)
	if err != nil {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("prepare failed for destination account: %v", err))
		return
	}

	// Check if destination voted NO
	if voteNo, ok := toReply.(*messages.VoteNo); ok {
		x.abortTransfer(ctx, msg.TransferID, fmt.Sprintf("destination account voted NO: %s", voteNo.Reason))
		return
	}

	// Phase 2: Commit - All participants voted YES
	ctx.Logger().Infof("all participants voted YES for transfer %s, proceeding to commit", msg.TransferID)

	// Commit source account
	_, err = goakt.Ask(ctx.Context(), fromPid, &messages.CommitTransfer{
		TransferID: msg.TransferID,
	}, askTimeout)
	if err != nil {
		// This is a serious error - we've promised to commit but failed
		// In a real system, we would need recovery logic here
		ctx.Logger().Errorf("CRITICAL: failed to commit source account for transfer %s: %v", msg.TransferID, err)
	}

	// Commit destination account
	_, err = goakt.Ask(ctx.Context(), toPid, &messages.CommitTransfer{
		TransferID: msg.TransferID,
	}, askTimeout)
	if err != nil {
		// This is a serious error - we've promised to commit but failed
		ctx.Logger().Errorf("CRITICAL: failed to commit destination account for transfer %s: %v", msg.TransferID, err)
	}

	// Mark transfer as committed
	transfer.SetStatus(domain.TransferStatusCommitted)
	if err := x.storage.WriteTransferState(ctx.Context(), msg.TransferID, transfer); err != nil {
		ctx.Logger().Errorf("failed to persist committed state for transfer %s: %v", msg.TransferID, err)
	}

	ctx.Response(&messages.TransferCompleted{TransferID: msg.TransferID})
}

func (x *Coordinator) abortTransfer(ctx *goakt.ReceiveContext, transferID, reason string) {
	transfer, _ := x.storage.GetTransferState(ctx.Context(), transferID)
	if transfer != nil {
		transfer.SetStatus(domain.TransferStatusAborted)
		transfer.SetReason(reason)
		if err := x.storage.WriteTransferState(ctx.Context(), transferID, transfer); err != nil {
			ctx.Logger().Errorf("failed to persist aborted state for transfer %s: %v", transferID, err)
		}
	}

	ctx.Logger().Infof("transfer %s aborted: %s", transferID, reason)
	ctx.Response(&messages.TransferFailed{TransferID: transferID, Reason: reason})
}

func (x *Coordinator) getOrSpawnAccount(ctx *goakt.ReceiveContext, system goakt.ActorSystem, accountID string) (*goakt.PID, error) {
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

func (x *Coordinator) handleGetTransferStatus(ctx *goakt.ReceiveContext, msg *messages.GetTransferStatus) {
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
