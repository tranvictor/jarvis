package explorers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetABIStringRetriesRateLimitAndRedactsKey(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(`{"status":"0","message":"NOTOK","result":"Max calls per sec rate limit reached (3/sec)"}`))
			return
		}
		w.Write([]byte(`{"status":"1","message":"OK","result":"[]"}`))
	}))
	defer srv.Close()

	prevDelay := abiRetryDelay
	abiRetryDelay = time.Millisecond
	t.Cleanup(func() { abiRetryDelay = prevDelay })

	ee := NewEtherscanLikeExplorer(srv.URL, "SECRETKEY", 1)
	got, err := ee.GetABIString("0x0000000000000000000000000000000000000001")
	if err != nil || got != "[]" {
		t.Fatalf("got %q, %v", got, err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestGetABIStringErrorsNeverContainTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"0","message":"NOTOK","result":"Contract source code not verified"}`))
	}))
	defer srv.Close()

	ee := NewEtherscanLikeExplorer(srv.URL, "SECRETKEY", 1)
	_, err := ee.GetABIString("0x0000000000000000000000000000000000000001")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRETKEY") || strings.Contains(err.Error(), "apikey") {
		t.Fatalf("error leaks the api key: %s", err)
	}
	if !strings.Contains(err.Error(), "Contract source code not verified") {
		t.Fatalf("error lost the explorer message: %s", err)
	}

	srv.Close()
	_, err = ee.GetABIString("0x0000000000000000000000000000000000000001")
	if err == nil || strings.Contains(err.Error(), "SECRETKEY") {
		t.Fatalf("transport error leaks the api key: %v", err)
	}
}
