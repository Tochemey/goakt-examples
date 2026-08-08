package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/dnssd/api"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb/samplepbconnect"
)

// stubs standing in for the actor-backed implementations, so this test covers the
// transport wiring (one mux, one port, two protocols) and nothing else.

type stubREST struct{}

func (stubREST) CreateAccount(w http.ResponseWriter, r *http.Request) { writeStub(w, "rest-create") }
func (stubREST) CreditAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	writeStub(w, accountId)
}
func (stubREST) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	writeStub(w, accountId)
}

func writeStub(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{AccountId: id, AccountBalance: 42}})
}

type stubRPC struct{}

func (stubRPC) CreateAccount(ctx context.Context, c *connect.Request[samplepb.CreateAccountRequest]) (*connect.Response[samplepb.CreateAccountResponse], error) {
	return connect.NewResponse(&samplepb.CreateAccountResponse{Account: &samplepb.Account{
		AccountId: c.Msg.GetCreateAccount().GetAccountId(), AccountBalance: 42,
	}}), nil
}
func (stubRPC) CreditAccount(ctx context.Context, c *connect.Request[samplepb.CreditAccountRequest]) (*connect.Response[samplepb.CreditAccountResponse], error) {
	return connect.NewResponse(&samplepb.CreditAccountResponse{}), nil
}
func (stubRPC) GetAccount(ctx context.Context, c *connect.Request[samplepb.GetAccountRequest]) (*connect.Response[samplepb.GetAccountResponse], error) {
	return connect.NewResponse(&samplepb.GetAccountResponse{Account: &samplepb.Account{
		AccountId: c.Msg.GetAccountId(), AccountBalance: 7,
	}}), nil
}

// buildServer mirrors listenAndServe's composition exactly.
func buildServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	mux.HandleFunc("GET /docs", serveSwaggerUI)

	rpcPath, rpcHandler := samplepbconnect.NewAccountServiceHandler(stubRPC{})
	mux.Handle(rpcPath, rpcHandler)

	handler := api.HandlerWithOptions(stubREST{}, api.StdHTTPServerOptions{BaseRouter: mux})

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Protocols: protocols, Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Close() }
}

func TestRESTAndRPCShareOnePort(t *testing.T) {
	addr, stop := buildServer(t)
	defer stop()

	// 1. REST over HTTP/1.1
	resp, err := http.Get("http://" + addr + "/accounts/acc-1")
	if err != nil {
		t.Fatalf("REST GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("REST status = %d, want 200", resp.StatusCode)
	}
	var out api.AccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("REST decode failed: %v", err)
	}
	if out.Account.AccountId != "acc-1" {
		t.Fatalf("REST account id = %q, want acc-1", out.Account.AccountId)
	}
	t.Logf("REST  proto=%s account=%s balance=%v", resp.Proto, out.Account.AccountId, out.Account.AccountBalance)

	// 2. Swagger UI on the same port
	docs, err := http.Get("http://" + addr + "/docs")
	if err != nil || docs.StatusCode != http.StatusOK {
		t.Fatalf("/docs failed: %v status=%v", err, docs)
	}
	docs.Body.Close()

	// 3. Connect RPC over unencrypted HTTP/2 (what a gRPC client needs)
	h2 := &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, a string, _ *tls.Config) (net.Conn, error) {
			return net.Dial(network, a)
		},
	}}
	client := samplepbconnect.NewAccountServiceClient(h2, "http://"+addr, connect.WithGRPC())
	rpcResp, err := client.GetAccount(context.Background(),
		connect.NewRequest(&samplepb.GetAccountRequest{AccountId: "acc-2"}))
	if err != nil {
		t.Fatalf("gRPC GetAccount failed: %v", err)
	}
	if rpcResp.Msg.GetAccount().GetAccountId() != "acc-2" {
		t.Fatalf("gRPC account id = %q, want acc-2", rpcResp.Msg.GetAccount().GetAccountId())
	}
	t.Logf("gRPC  account=%s balance=%v", rpcResp.Msg.GetAccount().GetAccountId(), rpcResp.Msg.GetAccount().GetAccountBalance())
}
