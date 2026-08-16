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

// Package service exposes the blockchain node over a small REST API. The API
// runs on every pod: reads of the chain are served by the pod's local replica,
// while writes resolve the Node singleton by name across the cluster and
// message it wherever it currently lives.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	goakt "github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/messages"
)

// askTimeout bounds the HTTP handlers' requests to the node actor.
const askTimeout = 5 * time.Second

// BlockchainService serves the REST API:
//
//	GET  /status        the whole blockchain, most recent block first (local replica)
//	GET  /transactions  the pending transactions
//	POST /transactions  submit a new transaction
//	GET  /mine          start mining a new block
//	GET  /validate      check a proof against the last block hash (?proof=N)
type BlockchainService struct {
	actorSystem goakt.ActorSystem
	logger      log.Logger
	port        int
	server      *http.Server
	replicaName string
}

func NewBlockchainService(system goakt.ActorSystem, port int, logger log.Logger) *BlockchainService {
	hostname, _ := os.Hostname()
	return &BlockchainService{
		actorSystem: system,
		logger:      logger,
		port:        port,
		replicaName: actors.ReplicaName(hostname),
	}
}

// node resolves the Node singleton across the cluster.
func (s *BlockchainService) node(ctx context.Context) (*goakt.PID, error) {
	pid, err := s.actorSystem.ActorOf(ctx, actors.NodeName)
	if err != nil {
		return nil, err
	}
	if pid.IsRemote() {
		s.logger.Debugf("%s found on remote node=%s", actors.NodeName,
			net.JoinHostPort(pid.Path().Host(), strconv.Itoa(pid.Path().Port())))
	}
	return pid, nil
}

// ask resolves the Node singleton and sends it a request-response message.
func (s *BlockchainService) ask(ctx context.Context, message any) (any, error) {
	pid, err := s.node(ctx)
	if err != nil {
		return nil, err
	}
	return goakt.Ask(ctx, pid, message, askTimeout)
}

// getStatus reads the chain from the replica running on this very pod: every
// pod holds the full ledger, so no request needs to leave the pod.
func (s *BlockchainService) getStatus(w http.ResponseWriter, r *http.Request) {
	pid, err := s.actorSystem.ActorOf(r.Context(), s.replicaName)
	if err != nil {
		s.writeError(w, err)
		return
	}
	reply, err := goakt.Ask(r.Context(), pid, &messages.GetChain{}, askTimeout)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply.(*messages.ChainState).Blocks)
}

func (s *BlockchainService) getTransactions(w http.ResponseWriter, r *http.Request) {
	reply, err := s.ask(r.Context(), &messages.GetPendingTransactions{})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply.(*messages.PendingTransactions).Items)
}

func (s *BlockchainService) submitTransaction(w http.ResponseWriter, r *http.Request) {
	var transaction chain.Transaction
	if err := json.NewDecoder(r.Body).Decode(&transaction); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reply, err := s.ask(r.Context(), &messages.SubmitTransaction{Transaction: transaction})
	if err != nil {
		s.writeError(w, err)
		return
	}
	submitted := reply.(*messages.TransactionSubmitted)
	writeJSON(w, http.StatusCreated, map[string]any{
		"message": fmt.Sprintf("Transaction will be added to block %d", submitted.BlockIndex),
	})
}

func (s *BlockchainService) mine(w http.ResponseWriter, r *http.Request) {
	pid, err := s.node(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := goakt.Tell(r.Context(), pid, &messages.MineBlock{}); err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"message": "Mining started"})
}

func (s *BlockchainService) validate(w http.ResponseWriter, r *http.Request) {
	proof, err := strconv.ParseInt(r.URL.Query().Get("proof"), 10, 64)
	if err != nil {
		http.Error(w, "invalid or missing proof query parameter", http.StatusBadRequest)
		return
	}
	reply, err := s.ask(r.Context(), &messages.CheckPowSolution{Proof: proof})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": reply.(*messages.ProofValidity).Valid})
}

func (s *BlockchainService) writeError(w http.ResponseWriter, err error) {
	if errors.Is(err, gerrors.ErrActorNotFound) {
		http.Error(w, "blockchain node not available yet", http.StatusServiceUnavailable)
		return
	}
	s.logger.Errorf("request failed: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func (s *BlockchainService) Start() {
	go func() {
		s.listenAndServe()
	}()
}

func (s *BlockchainService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *BlockchainService) listenAndServe() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.getStatus)
	mux.HandleFunc("GET /transactions", s.getTransactions)
	mux.HandleFunc("POST /transactions", s.submitTransaction)
	mux.HandleFunc("GET /mine", s.mine)
	mux.HandleFunc("GET /validate", s.validate)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler:           mux,
	}

	s.logger.Infof("Blockchain service listening on :%d", s.port)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Errorf("failed to start service: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
