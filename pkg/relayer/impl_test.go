package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/GoPolymarket/polymarket-go-sdk/v2/pkg/transport"
)

// recordingDoer is a minimal HTTP Doer used in tests. It serves canned
// response bodies keyed by request path and records the last request so
// callers can assert on method, path, and query parameters.
type recordingDoer struct {
	responses map[string]string
	lastReq   *http.Request
}

func newRecordingDoer() *recordingDoer {
	return &recordingDoer{responses: make(map[string]string)}
}

func (d *recordingDoer) addResponse(path, body string) {
	d.responses[path] = body
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	d.lastReq = req
	body, ok := d.responses[req.URL.Path]
	if !ok {
		return nil, errors.New("unexpected request to " + req.URL.Path)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}, nil
}

// --------------- NewClient ---------------

func TestNewClient_NilTransport(t *testing.T) {
	c := NewClient(nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_WithTransport(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), BaseURL))
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestBaseURL(t *testing.T) {
	if BaseURL != "https://relayer.polymarket.com" {
		t.Fatalf("unexpected BaseURL: %s", BaseURL)
	}
}

// --------------- Submit ---------------

func TestSubmit_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/submit", `{"transactionID":"0xabc","state":"pending"}`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.Submit(context.Background(), &SubmitRequest{
		Transaction: map[string]interface{}{"to": "0x1", "value": "0"},
		Signature:   "0xsig",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TransactionID != "0xabc" || resp.State != "pending" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if doer.lastReq == nil {
		t.Fatal("expected a request to be made")
	}
	if doer.lastReq.Method != http.MethodPost {
		t.Fatalf("expected POST, got %s", doer.lastReq.Method)
	}
}

func TestSubmit_NilRequest(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.Submit(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestSubmit_HTTPError(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.Submit(context.Background(), &SubmitRequest{
		Transaction: map[string]interface{}{"to": "0x1"},
		Signature:   "0xsig",
	})
	if err == nil {
		t.Fatal("expected error from transport")
	}
}

// --------------- GetTransaction ---------------

func TestGetTransaction_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/transaction", `{"id":"1","transactionID":"0xabc","state":"confirmed","from":"0x1","to":"0x2","hash":"0xhash","nonce":5}`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetTransaction(context.Background(), "0xabc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "1" || resp.TransactionID != "0xabc" || resp.State != "confirmed" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Nonce != 5 {
		t.Fatalf("expected nonce 5, got %d", resp.Nonce)
	}
	if doer.lastReq == nil {
		t.Fatal("expected a request to be made")
	}
	if got := doer.lastReq.URL.Query().Get("id"); got != "0xabc" {
		t.Fatalf("expected id=0xabc, got %q", got)
	}
}

func TestGetTransaction_EmptyID(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.GetTransaction(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

// --------------- GetTransactions ---------------

func TestGetTransactions_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/transactions", `[{"id":"1","state":"confirmed"},{"id":"2","state":"pending"}]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetTransactions(context.Background(), &GetTransactionsRequest{Limit: 10, Offset: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 2 || resp[0].ID != "1" || resp[1].State != "pending" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	q := doer.lastReq.URL.Query()
	if q.Get("limit") != "10" {
		t.Errorf("expected limit=10, got %q", q.Get("limit"))
	}
	if q.Get("offset") != "5" {
		t.Errorf("expected offset=5, got %q", q.Get("offset"))
	}
}

func TestGetTransactions_NilRequest(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/transactions", `[]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetTransactions(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty result, got %d", len(resp))
	}
	// No pagination params should be set for a nil request.
	if got := doer.lastReq.URL.RawQuery; got != "" {
		t.Fatalf("expected no query params, got %q", got)
	}
}

func TestGetTransactions_ZeroPagination(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/transactions", `[]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	_, err := c.GetTransactions(context.Background(), &GetTransactionsRequest{Limit: 0, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := doer.lastReq.URL.RawQuery; got != "" {
		t.Fatalf("expected no query params for zero pagination, got %q", got)
	}
}

// --------------- GetNonce ---------------

func TestGetNonce_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/nonce", `{"nonce":"7"}`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetNonce(context.Background(), "0xsigner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Nonce != "7" {
		t.Fatalf("expected nonce 7, got %s", resp.Nonce)
	}
	if got := doer.lastReq.URL.Query().Get("signer"); got != "0xsigner" {
		t.Fatalf("expected signer=0xsigner, got %q", got)
	}
}

func TestGetNonce_EmptySigner(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.GetNonce(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty signer")
	}
}

// --------------- GetRelayPayload ---------------

func TestGetRelayPayload_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/relay-payload", `{"relayerAddress":"0xrelayer","nonce":"3"}`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetRelayPayload(context.Background(), "0xsigner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RelayerAddress != "0xrelayer" || resp.Nonce != "3" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := doer.lastReq.URL.Query().Get("signer"); got != "0xsigner" {
		t.Fatalf("expected signer=0xsigner, got %q", got)
	}
}

func TestGetRelayPayload_EmptySigner(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.GetRelayPayload(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty signer")
	}
}

// --------------- GetDeployed ---------------

func TestGetDeployed_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/deployed", `{"deployed":true}`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetDeployed(context.Background(), "0xaddr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Deployed {
		t.Fatal("expected deployed=true")
	}
	if got := doer.lastReq.URL.Query().Get("address"); got != "0xaddr" {
		t.Fatalf("expected address=0xaddr, got %q", got)
	}
}

func TestGetDeployed_EmptyAddress(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.GetDeployed(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

// --------------- GetAPIKeys ---------------

func TestGetAPIKeys_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/relayer/api/keys", `[{"id":"k1","key":"sk-xxx","name":"main","createdAt":"2026-01-01"}]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.GetAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 1 || resp[0].ID != "k1" || resp[0].Name != "main" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if doer.lastReq == nil {
		t.Fatal("expected a request to be made")
	}
	if doer.lastReq.URL.Path != "/relayer/api/keys" {
		t.Fatalf("expected path /relayer/api/keys, got %s", doer.lastReq.URL.Path)
	}
}

func TestGetAPIKeys_HTTPError(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.GetAPIKeys(context.Background())
	if err == nil {
		t.Fatal("expected error from transport")
	}
}

// --------------- types ---------------

func TestSubmitRequestJSON(t *testing.T) {
	req := SubmitRequest{
		Transaction: map[string]interface{}{"to": "0x1"},
		Signature:   "0xsig",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	s := string(data)
	if !bytes.Contains([]byte(s), []byte(`"signature":"0xsig"`)) {
		t.Errorf("expected signature in JSON, got %s", s)
	}
}
