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
	"time"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/persistence"
)

// BlocksTopic is the pub/sub topic on which freshly mined blocks fan out to
// every chain replica in the cluster.
const BlocksTopic = "blocks.minted"

// ReplicaNamePrefix prefixes the pod name to build a replica's actor name.
const ReplicaNamePrefix = "chain-"

// ReplicaName returns the cluster-unique name of the chain replica running on
// the given pod.
func ReplicaName(pod string) string {
	return ReplicaNamePrefix + pod
}

// catchUpTimeout bounds a replica's catch-up requests to each of its peers.
const catchUpTimeout = 3 * time.Second

// Replica maintains this pod's full copy of the chain, backed by the local
// Pebble store: every pod holds the whole ledger, the way real blockchain
// nodes do. A replica appends the blocks announced on the blocks topic after
// validating them against its tip, and when it starts (or is asked to
// resynchronize) it catches up on missed blocks from its peer replicas.
type Replica struct {
	peers []string
	store persistence.Store
	chain *chain.Chain
}

var _ goakt.Actor = (*Replica)(nil)

// NewReplica creates a chain replica. peers are the actor names of the
// replicas running on the other pods.
func NewReplica(peers []string) *Replica {
	return &Replica{peers: peers}
}

// PreStart restores the local copy of the chain from the Pebble store. Every
// restored block goes through the same validation as a block received from
// the network.
func (x *Replica) PreStart(ctx *goakt.Context) error {
	x.store = ctx.Extension(persistence.PebbleStoreID).(persistence.Store)
	blocks, err := x.store.Load(ctx.Context())
	if err != nil {
		return err
	}

	x.chain = chain.New()
	for _, block := range blocks {
		if err := x.chain.Append(block); err != nil {
			return err
		}
	}
	return nil
}

func (x *Replica) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goakt.PostStart:
		// subscribe to the blocks topic first so no announcement is missed
		// while catching up from the peers
		ctx.Tell(ctx.ActorSystem().TopicActor(), goakt.NewSubscribe(BlocksTopic))
		x.catchUp(ctx)
		ctx.Logger().Infof("%s started at block %d", ctx.Self().Name(), x.chain.Last().Index)
	case *goakt.SubscribeAck:
		ctx.Logger().Infof("%s subscribed to %s", ctx.Self().Name(), BlocksTopic)
	case *messages.BlockMinted:
		x.append(ctx, msg.Block)
	case *messages.GetBlocksFrom:
		ctx.Response(&messages.BlocksFrom{Blocks: x.chain.Since(msg.FromIndex)})
	case *messages.SyncChain:
		x.catchUp(ctx)
		ctx.Response(&messages.SyncDone{Index: x.chain.Last().Index})
	case *messages.GetChain:
		ctx.Response(&messages.ChainState{Blocks: x.chain.Blocks()})
	case *messages.GetLastHash:
		ctx.Response(&messages.LastHash{Hash: x.chain.Last().Hash})
	case *messages.GetLastIndex:
		ctx.Response(&messages.LastIndex{Index: x.chain.Last().Index})
	case *messages.GetLastBlock:
		ctx.Response(&messages.LastBlock{Block: x.chain.Last()})
	default:
		ctx.Unhandled()
	}
}

func (x *Replica) PostStop(*goakt.Context) error {
	return nil
}

// append validates a block against the local tip, appends it, and persists
// it. A block that does not extend the tip signals missed announcements, so
// the replica catches up from its peers instead.
func (x *Replica) append(ctx *goakt.ReceiveContext, block chain.Block) {
	if err := x.chain.Append(block); err != nil {
		ctx.Logger().Warnf("%s rejected announced block %d (%v), catching up from peers",
			ctx.Self().Name(), block.Index, err)
		x.catchUp(ctx)
		return
	}

	if err := x.store.Append(ctx.Context(), block); err != nil {
		// a failed write restarts the replica, which reloads the chain from
		// the store so memory and disk stay consistent
		ctx.Err(err)
		return
	}

	ctx.Logger().Infof("%s appended block %d with hash %s (%d transaction(s))",
		ctx.Self().Name(), block.Index, block.Hash, len(block.Transactions))
}

// catchUp asks every peer replica for the blocks past the local tip and
// appends the longest answer. Peers that are down or behind are skipped; with
// a single miner in the cluster there are no forks to resolve.
func (x *Replica) catchUp(ctx *goakt.ReceiveContext) {
	var longest []chain.Block

	for _, peer := range x.peers {
		pid, err := ctx.ActorSystem().ActorOf(ctx.Context(), peer)
		if err != nil {
			continue
		}

		reply, err := goakt.Ask(ctx.Context(), pid, &messages.GetBlocksFrom{FromIndex: x.chain.Last().Index + 1}, catchUpTimeout)
		if err != nil {
			continue
		}

		if blocks := reply.(*messages.BlocksFrom).Blocks; len(blocks) > len(longest) {
			longest = blocks
		}
	}

	for _, block := range longest {
		if err := x.chain.Append(block); err != nil {
			ctx.Logger().Warnf("%s stopped catching up at block %d: %v", ctx.Self().Name(), block.Index, err)
			return
		}

		if err := x.store.Append(ctx.Context(), block); err != nil {
			ctx.Err(err)
			return
		}
	}

	if len(longest) > 0 {
		ctx.Logger().Infof("%s caught up to block %d", ctx.Self().Name(), x.chain.Last().Index)
	}
}
