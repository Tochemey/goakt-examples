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

// Package chain implements the blockchain domain: transactions, blocks, the
// chain itself, and the proof-of-work.
package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	// difficulty is the number of leading zeros a proof-of-work hash must
	// have. Raise it to make mining take longer.
	difficulty = 4

	// MiningReward is the value of the coinbase transaction granted to the
	// node that mines a block.
	MiningReward = 100

	// genesisHash and genesisProof seed the genesis block of the chain.
	genesisHash  = "1"
	genesisProof = 100
)

// Transaction moves some value from a sender to a recipient.
type Transaction struct {
	Sender    string  `json:"sender"`
	Recipient string  `json:"recipient"`
	Value     float64 `json:"value"`
}

// Block is one link of the chain. Its hash is the SHA-256 digest of the JSON
// representation of the block (with the Hash field left empty), and its proof
// is the solution of the proof-of-work computed on the previous block's hash.
type Block struct {
	Index        int64         `json:"index"`
	Hash         string        `json:"hash"`
	PreviousHash string        `json:"previousHash"`
	Proof        int64         `json:"proof"`
	Transactions []Transaction `json:"transactions"`
	Timestamp    int64         `json:"timestamp"`
}

// Chain is the blockchain itself: the genesis block followed by every mined block.
type Chain struct {
	blocks []Block
}

// New creates a chain containing only the genesis block.
func New() *Chain {
	return &Chain{
		blocks: []Block{{
			Index: 0,
			Hash:  genesisHash,
			Proof: genesisProof,
		}},
	}
}

// Last returns the most recent block of the chain.
func (x *Chain) Last() Block {
	return x.blocks[len(x.blocks)-1]
}

// Blocks returns a copy of the chain, most recent block first.
func (x *Chain) Blocks() []Block {
	out := make([]Block, len(x.blocks))
	for i, block := range x.blocks {
		out[len(x.blocks)-1-i] = block
	}
	return out
}

// Since returns a copy of the blocks with an index greater than or equal to
// the given index, in ascending order.
func (x *Chain) Since(index int64) []Block {
	var out []Block
	for _, block := range x.blocks {
		if block.Index >= index {
			out = append(out, block)
		}
	}
	return out
}

// NextBlock builds the block that extends the given previous block with the
// given transactions and proof.
func NextBlock(previous Block, transactions []Transaction, proof int64, timestamp int64) Block {
	block := Block{
		Index:        previous.Index + 1,
		PreviousHash: previous.Hash,
		Proof:        proof,
		Transactions: transactions,
		Timestamp:    timestamp,
	}
	block.Hash = blockHash(block)
	return block
}

// Append validates the given block against the current tip of the chain and
// appends it. A block is accepted only when it extends the tip, links to its
// hash, carries a valid proof-of-work, and its own hash checks out.
func (x *Chain) Append(block Block) error {
	last := x.Last()
	switch {
	case block.Index != last.Index+1:
		return fmt.Errorf("block %d does not extend the tip %d", block.Index, last.Index)
	case block.PreviousHash != last.Hash:
		return fmt.Errorf("block %d does not link to the tip hash %s", block.Index, last.Hash)
	case !ValidProof(last.Hash, block.Proof):
		return fmt.Errorf("block %d carries an invalid proof %d", block.Index, block.Proof)
	case blockHash(block) != block.Hash:
		return fmt.Errorf("block %d hash mismatch", block.Index)
	}
	x.blocks = append(x.blocks, block)
	return nil
}

// blockHash computes the SHA-256 digest of the JSON representation of the block.
func blockHash(block Block) string {
	block.Hash = ""
	encoded, _ := json.Marshal(block)
	return sha256Hex(encoded)
}

// ValidProof reports whether SHA-256(lastHash + proof) starts with
// `difficulty` zeros.
func ValidProof(lastHash string, proof int64) bool {
	guess := lastHash + strconv.FormatInt(proof, 10)
	digest := sha256Hex([]byte(guess))
	return strings.HasPrefix(digest, strings.Repeat("0", difficulty))
}

// ProofOfWork brute-forces the proof for the given last block hash.
func ProofOfWork(lastHash string) int64 {
	var proof int64
	for !ValidProof(lastHash, proof) {
		proof++
	}
	return proof
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
