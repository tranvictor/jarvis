package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteParsesJSONAndSendsPayloadOnly(t *testing.T) {
	const canary = "CANARY_UncleBob_7f3a"
	var saw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		saw, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"risk":"danger","bullets":["bad"],"asset_effect":"loses tokens","reconfirms":["upgrade"]}`}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), URL: srv.URL, Model: "grok-4.6", Key: "test-key"}
	p := Payload{ChainID: 1, Method: "upgradeTo", LocalCodes: []string{"upgrade"}}
	raw, _ := json.Marshal(p)
	reply, err := c.Complete(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Risk != "danger" || len(reply.Bullets) != 1 || reply.Reconfirms[0] != "upgrade" {
		t.Fatalf("%+v", reply)
	}
	if strings.Contains(string(saw), canary) {
		t.Fatalf("canary in HTTP body: %s", saw)
	}
	if !strings.Contains(string(saw), `"role":"system"`) {
		t.Fatalf("system message missing: %s", saw)
	}
	if !strings.Contains(string(saw), "upgradeTo") {
		t.Fatalf("payload missing: %s", saw)
	}
}

func TestCompleteMissingKey(t *testing.T) {
	c := &Client{Key: ""}
	_, err := c.Complete(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
