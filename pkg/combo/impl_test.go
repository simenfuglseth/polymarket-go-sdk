package combo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/GoPolymarket/polymarket-go-sdk/v2/pkg/transport"
)

// recordingDoer is a minimal HTTP Doer used in tests. It serves canned
// response bodies keyed by request path and records the last request so
// callers can assert on method, path, and query parameters.
type recordingDoer struct {
	responses map[string]string
	lastReq   *http.Request
	status    int
}

func newRecordingDoer() *recordingDoer {
	return &recordingDoer{
		responses: make(map[string]string),
		status:    http.StatusOK,
	}
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
		StatusCode: d.status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}, nil
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

// --------------- NewClient ---------------

func TestNewClient_NilTransport(t *testing.T) {
	c := NewClient(nil)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_WithTransport(t *testing.T) {
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

// --------------- ComboMarkets ---------------

func TestComboMarkets_Success(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/combo-markets", `[
		{"id":"1","question":"Will X happen?","conditionIds":["0xabc","0xdef"],"slug":"will-x-happen","startDate":"2026-01-01","endDate":"2026-12-31","tags":[{"id":"t1","label":"crypto"}],"active":true,"closed":false}
	]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.ComboMarkets(context.Background(), &ComboMarketsRequest{
		Limit:  intPtr(10),
		Offset: intPtr(0),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 market, got %d", len(resp))
	}
	m := resp[0]
	if m.ID != "1" || m.Question != "Will X happen?" || m.Slug != "will-x-happen" {
		t.Fatalf("unexpected market: %+v", m)
	}
	if !m.Active || m.Closed {
		t.Fatalf("unexpected active/closed flags: active=%v closed=%v", m.Active, m.Closed)
	}
	if len(m.ConditionIDs) != 2 || m.ConditionIDs[0] != "0xabc" {
		t.Fatalf("unexpected condition ids: %v", m.ConditionIDs)
	}
	if len(m.Tags) != 1 || m.Tags[0].ID != "t1" {
		t.Fatalf("unexpected tags: %v", m.Tags)
	}
}

func TestComboMarkets_EmptyResult(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/combo-markets", `[]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	resp, err := c.ComboMarkets(context.Background(), &ComboMarketsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || len(resp) != 0 {
		t.Fatalf("expected empty non-nil slice, got %v", resp)
	}
}

func TestComboMarkets_NilRequest(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/combo-markets", `[]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	// A nil request must not panic and should issue the call without query params.
	resp, err := c.ComboMarkets(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty result, got %d", len(resp))
	}
	if doer.lastReq == nil {
		t.Fatal("expected a request to be made")
	}
	if got := doer.lastReq.URL.RawQuery; got != "" {
		t.Fatalf("expected no query params for nil request, got %q", got)
	}
}

func TestComboMarkets_QueryParams(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/combo-markets", `[]`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	_, err := c.ComboMarkets(context.Background(), &ComboMarketsRequest{
		Limit:   intPtr(25),
		Offset:  intPtr(50),
		Active:  boolPtr(true),
		Closed:  boolPtr(false),
		TagID:   "tag-1",
		TagSlug: "crypto",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doer.lastReq == nil {
		t.Fatal("expected a request to be made")
	}
	q := doer.lastReq.URL.Query()
	if q.Get("limit") != "25" {
		t.Errorf("expected limit=25, got %q", q.Get("limit"))
	}
	if q.Get("offset") != "50" {
		t.Errorf("expected offset=50, got %q", q.Get("offset"))
	}
	if q.Get("active") != "true" {
		t.Errorf("expected active=true, got %q", q.Get("active"))
	}
	if q.Get("closed") != "false" {
		t.Errorf("expected closed=false, got %q", q.Get("closed"))
	}
	if q.Get("tag_id") != "tag-1" {
		t.Errorf("expected tag_id=tag-1, got %q", q.Get("tag_id"))
	}
	if q.Get("tag_slug") != "crypto" {
		t.Errorf("expected tag_slug=crypto, got %q", q.Get("tag_slug"))
	}
}

func TestComboMarkets_HTTPError(t *testing.T) {
	// No response registered for the path -> Doer returns an error.
	c := NewClient(transport.NewClient(newRecordingDoer(), "http://example"))
	_, err := c.ComboMarkets(context.Background(), &ComboMarketsRequest{})
	if err == nil {
		t.Fatal("expected error from transport")
	}
}

func TestComboMarkets_InvalidJSON(t *testing.T) {
	doer := newRecordingDoer()
	doer.addResponse("/combo-markets", `not-json`)
	c := NewClient(transport.NewClient(doer, "http://example"))

	_, err := c.ComboMarkets(context.Background(), &ComboMarketsRequest{})
	if err == nil {
		t.Fatal("expected JSON unmarshal error")
	}
}

// --------------- query helpers ---------------

func TestAddInt(t *testing.T) {
	q := url.Values{}
	addInt(q, "limit", nil)
	if _, ok := q["limit"]; ok {
		t.Fatal("expected limit to be absent for nil pointer")
	}
	v := 42
	addInt(q, "limit", &v)
	if q.Get("limit") != "42" {
		t.Fatalf("expected limit=42, got %q", q.Get("limit"))
	}
}

func TestAddBool(t *testing.T) {
	q := url.Values{}
	addBool(q, "active", nil)
	if _, ok := q["active"]; ok {
		t.Fatal("expected active to be absent for nil pointer")
	}
	b := true
	addBool(q, "active", &b)
	if q.Get("active") != "true" {
		t.Fatalf("expected active=true, got %q", q.Get("active"))
	}
}

func TestAddString(t *testing.T) {
	q := url.Values{}
	addString(q, "tag_id", "")
	if _, ok := q["tag_id"]; ok {
		t.Fatal("expected tag_id to be absent for empty string")
	}
	addString(q, "tag_id", "crypto")
	if q.Get("tag_id") != "crypto" {
		t.Fatalf("expected tag_id=crypto, got %q", q.Get("tag_id"))
	}
}

// --------------- types ---------------

func TestComboMarketsRequestJSON(t *testing.T) {
	limit := 5
	active := true
	req := ComboMarketsRequest{
		Limit:   &limit,
		Offset:  &limit,
		Active:  &active,
		TagID:   "t",
		TagSlug: "s",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"limit":5`, `"active":true`, `"tag_id":"t"`, `"tag_slug":"s"`} {
		if !contains(s, want) {
			t.Errorf("expected JSON to contain %q, got %s", want, s)
		}
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
