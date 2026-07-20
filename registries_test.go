package enrichment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestRegistriesClientBlocksLoopbackRepositoryURL(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRegistriesClient()
	purl := "pkg:npm/lodash?repository_url=" + url.QueryEscape(srv.URL)

	_, err := c.GetVersions(context.Background(), purl)
	if err == nil {
		t.Fatalf("expected safehttp to refuse loopback %s, got nil error", srv.URL)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("loopback server received %d requests; safehttp gate did not block dial", n)
	}
}

func TestAcquireSemaphore(t *testing.T) {
	sem := make(chan struct{}, 1)

	if !acquireSemaphore(context.Background(), sem) {
		t.Fatal("acquireSemaphore() = false, want true")
	}

	select {
	case <-sem:
	default:
		t.Fatal("semaphore was not acquired")
	}
}

func TestAcquireSemaphoreReturnsFalseWhenContextCanceled(t *testing.T) {
	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if acquireSemaphore(ctx, sem) {
		t.Fatal("acquireSemaphore() = true, want false")
	}
}
