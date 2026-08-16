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

// Package messages defines the actor messages. They are plain Go structs;
// the ones that travel between pods are registered as serializables with the
// remoting layer, which encodes them with CBOR.
package messages

import "github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"

// AddTransaction appends a transaction to the list of pending transactions.
// Fire-and-forget: the broker does not respond.
type AddTransaction struct {
	Transaction chain.Transaction
}

// GetTransactions requests the pending transactions. The broker answers with
// PendingTransactions.
type GetTransactions struct{}

// PendingTransactions is the broker's response to GetTransactions.
type PendingTransactions struct {
	Items []chain.Transaction
}

// ClearTransactions empties the list of pending transactions once they have
// been included in a mined block.
type ClearTransactions struct{}

// Mine asks the miner to find the proof-of-work for the given last block hash.
// The miner becomes busy and pipes a ProofFound message back to the sender
// when the computation completes.
type Mine struct {
	LastHash string
}

// ProofFound is the result of the mining computation, delivered to the node
// through PipeTo when the computation completes.
type ProofFound struct {
	LastHash string
	Proof    int64
}

// Validate asks the miner to verify a proof-of-work solution for a given
// hash. The miner answers with ProofValidity in both its ready and busy states.
type Validate struct {
	Hash  string
	Proof int64
}

// ProofValidity is the miner's response to Validate.
type ProofValidity struct {
	Valid bool
}

// Ready tells a busy miner that the mined block has been added to the chain
// and that it can accept mining requests again.
type Ready struct{}

// BlockMinted announces a freshly mined block. The node publishes it to the
// blocks topic and every chain replica appends it to its local copy.
type BlockMinted struct {
	Block chain.Block
}

// GetBlocksFrom asks a chain replica for every block it holds from the given
// index on. Replicas use it to catch up from their peers when they start.
type GetBlocksFrom struct {
	FromIndex int64
}

// BlocksFrom is a chain replica's response to GetBlocksFrom.
type BlocksFrom struct {
	Blocks []chain.Block
}

// SyncChain asks a chain replica to catch up from its peers. The replica
// answers with SyncDone once its copy is up to date.
type SyncChain struct{}

// SyncDone is a chain replica's response to SyncChain.
type SyncDone struct {
	Index int64
}

// GetChain requests the full chain. The chain replica answers with ChainState.
type GetChain struct{}

// ChainState is the chain replica's response to GetChain.
type ChainState struct {
	Blocks []chain.Block
}

// GetLastHash requests the hash of the most recent block.
type GetLastHash struct{}

// LastHash is the chain replica's response to GetLastHash.
type LastHash struct {
	Hash string
}

// GetLastIndex requests the index of the most recent block.
type GetLastIndex struct{}

// LastIndex is the chain replica's response to GetLastIndex.
type LastIndex struct {
	Index int64
}

// GetLastBlock requests the most recent block.
type GetLastBlock struct{}

// LastBlock is the chain replica's response to GetLastBlock.
type LastBlock struct {
	Block chain.Block
}

// MineBlock kicks off the mining workflow: the node fetches the last hash,
// hands it to the miner, and adds the mined block when the proof comes back.
type MineBlock struct{}

// SubmitTransaction adds a transaction to the node's broker. The node answers
// with TransactionSubmitted.
type SubmitTransaction struct {
	Transaction chain.Transaction
}

// TransactionSubmitted tells the caller which block the transaction will be
// part of once mined.
type TransactionSubmitted struct {
	BlockIndex int64
}

// CheckPowSolution asks the node to validate a proof against the current last
// block hash. The node answers with ProofValidity.
type CheckPowSolution struct {
	Proof int64
}

// GetPendingTransactions requests the node's pending transactions. The node
// answers with PendingTransactions.
type GetPendingTransactions struct{}
