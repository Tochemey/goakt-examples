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

// Package actors implements the blockchain node's actors: the Node cluster
// singleton with its supervised children (Broker and Miner), and the chain
// Replica running on every pod.
package actors

import (
	"os"
	"time"

	"github.com/google/uuid"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/messages"
)

// NodeName is the cluster-wide name of the Node singleton. Every pod resolves
// the node through this name with ActorOf, wherever it currently lives.
const NodeName = "node0"

// askTimeout bounds the node's requests to its children and its local replica.
const askTimeout = time.Second

// syncTimeout bounds the resynchronization of the local replica when the
// node starts.
const syncTimeout = 10 * time.Second

// coinbase is the conventional sender of mining reward transactions.
const coinbase = "coinbase"

// Node is the backbone of the blockchain node. It runs as a cluster singleton:
// exactly one Node lives in the whole cluster, and when its host pod dies it
// is respawned on another one. It spawns and supervises the broker and the
// miner, mines on top of the chain replica of its host pod, and announces
// mined blocks to every replica on the blocks topic. Being the only miner in
// the cluster, it is the source of truth for new blocks and no forks can occur.
type Node struct {
	broker  *goakt.PID
	miner   *goakt.PID
	replica *goakt.PID
}

var _ goakt.Actor = (*Node)(nil)

func (x *Node) PreStart(*goakt.Context) error {
	return nil
}

func (x *Node) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goakt.PostStart:
		// the node restarts a failing child; children are long-lived so they
		// are not passivated between requests, and never relocated on their
		// own: they live and die with the singleton, which respawns them
		// wherever it lands after a failover
		opts := []goakt.SpawnOption{
			goakt.WithLongLived(),
			goakt.WithRelocationDisabled(),
			goakt.WithSupervisor(
				supervisor.NewSupervisor(
					supervisor.WithAnyErrorDirective(supervisor.RestartDirective))),
		}
		x.broker = ctx.Spawn("broker", &Broker{}, opts...)
		x.miner = ctx.Spawn("miner", &Miner{}, opts...)

		// the node mines on top of the chain replica of its host pod; after a
		// failover that replica may have missed the latest announcements, so
		// it is resynchronized from its peers before mining resumes
		hostname, err := os.Hostname()
		if err != nil {
			ctx.Err(err)
			return
		}
		replica, err := ctx.ActorSystem().ActorOf(ctx.Context(), ReplicaName(hostname))
		if err != nil {
			ctx.Err(err)
			return
		}
		x.replica = replica
		synced := ctx.Ask(x.replica, &messages.SyncChain{}, syncTimeout).(*messages.SyncDone)
		ctx.Logger().Infof("%s started on %s at block %d, supervising broker and miner",
			ctx.Self().Name(), hostname, synced.Index)
	case *messages.SubmitTransaction:
		ctx.Tell(x.broker, &messages.AddTransaction{Transaction: msg.Transaction})
		lastIndex := ctx.Ask(x.replica, &messages.GetLastIndex{}, askTimeout).(*messages.LastIndex)
		ctx.Response(&messages.TransactionSubmitted{BlockIndex: lastIndex.Index + 1})
	case *messages.MineBlock:
		lastHash := ctx.Ask(x.replica, &messages.GetLastHash{}, askTimeout).(*messages.LastHash)
		ctx.Tell(x.miner, &messages.Mine{LastHash: lastHash.Hash})
	case *messages.ProofFound:
		last := ctx.Ask(x.replica, &messages.GetLastBlock{}, askTimeout).(*messages.LastBlock)
		if last.Block.Hash != msg.LastHash {
			ctx.Logger().Warnf("the chain tip moved while mining, dropping proof %d", msg.Proof)
			ctx.Tell(x.miner, &messages.Ready{})
			return
		}
		// the mining reward for this node, then the block is assembled from
		// every pending transaction and announced to every chain replica
		ctx.Tell(x.broker, &messages.AddTransaction{Transaction: chain.Transaction{
			Sender:    coinbase,
			Recipient: ctx.Self().Name(),
			Value:     chain.MiningReward,
		}})
		pending := ctx.Ask(x.broker, &messages.GetTransactions{}, askTimeout).(*messages.PendingTransactions)
		block := chain.NextBlock(last.Block, pending.Items, msg.Proof, time.Now().UnixMilli())
		ctx.Tell(ctx.ActorSystem().TopicActor(),
			goakt.NewPublish(uuid.NewString(), BlocksTopic, &messages.BlockMinted{Block: block}))
		ctx.Tell(x.broker, &messages.ClearTransactions{})
		ctx.Tell(x.miner, &messages.Ready{})
	case *messages.CheckPowSolution:
		lastHash := ctx.Ask(x.replica, &messages.GetLastHash{}, askTimeout).(*messages.LastHash)
		validity := ctx.Ask(x.miner, &messages.Validate{Hash: lastHash.Hash, Proof: msg.Proof}, askTimeout)
		ctx.Response(validity)
	case *messages.GetPendingTransactions:
		ctx.Response(ctx.Ask(x.broker, &messages.GetTransactions{}, askTimeout))
	case *goakt.Terminated:
		ctx.Logger().Infof("Child actor %s terminated", msg.ActorPath().String())
	default:
		ctx.Unhandled()
	}
}

func (x *Node) PostStop(*goakt.Context) error {
	return nil
}
