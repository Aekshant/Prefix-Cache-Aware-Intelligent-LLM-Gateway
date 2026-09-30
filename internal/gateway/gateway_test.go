package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llm-gateway/internal/config"
)

func newTestGateway(workers ...config.Worker) *Gateway {
	return New(&config.Config{
		Workers:        workers,
		CacheTTL:       time.Minute,
		MaxActiveConns: 5,
		PrefixBytes:    120,
	})
}

func TestSelectWorkerPinsPrefix(t *testing.T) {
	g := newTestGateway(
		config.Worker{ID: "a", URL: "http://a"},
		config.Worker{ID: "b", URL: "http://b"},
	)

	first, hit := g.SelectWorker(42)
	if hit {
		t.Fatal("first lookup should be a miss")
	}
	for i := 0; i < 5; i++ {
		w, hit := g.SelectWorker(42)
		if !hit || w.ID != first.ID {
			t.Fatalf("expected hit on %s, got %s (hit=%v)", first.ID, w.ID, hit)
		}
	}
}

func TestSelectWorkerFallsBackWhenSaturated(t *testing.T) {
	g := newTestGateway(
		config.Worker{ID: "a", URL: "http://a"},
		config.Worker{ID: "b", URL: "http://b"},
	)

	w, _ := g.SelectWorker(7)
	w.ActiveConns = g.maxActiveConns

	if _, hit := g.SelectWorker(7); hit {
		t.Fatal("saturated worker should not be reported as a cache hit")
	}
}

func TestEvictExpired(t *testing.T) {
	g := newTestGateway(config.Worker{ID: "a", URL: "http://a"})
	g.SelectWorker(1)
	g.SelectWorker(2)

	if n := g.evictExpired(time.Now()); n != 0 {
		t.Fatalf("nothing should expire yet, evicted %d", n)
	}
	if n := g.evictExpired(time.Now().Add(2 * time.Minute)); n != 2 {
		t.Fatalf("expected 2 evictions, got %d", n)
	}
}

func TestExtractPrefixHash(t *testing.T) {
	shared := strings.Repeat("x", 120)
	if extractPrefixHash([]byte(shared+"one"), 120) != extractPrefixHash([]byte(shared+"two"), 120) {
		t.Fatal("bodies with identical prefixes should hash equally")
	}
	if extractPrefixHash([]byte("short"), 120) == extractPrefixHash([]byte("other"), 120) {
		t.Fatal("different short bodies should hash differently")
	}
}

func TestHandleChatStreamsSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected upstream path %s", r.URL.Path)
		}
		io.WriteString(w, "{\"t\":1}\n{\"t\":2}\n")
	}))
	defer upstream.Close()

	g := newTestGateway(config.Worker{ID: "a", URL: upstream.URL})

	rec := httptest.NewRecorder()
	g.HandleChat(rec, httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"model":"m"}`)))

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("unexpected content type %q", ct)
	}
	want := "data: {\"t\":1}\n\n\ndata: {\"t\":2}\n\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("unexpected body:\n%q\nwant:\n%q", got, want)
	}
}

func TestHandleChatRejectsGet(t *testing.T) {
	g := newTestGateway(config.Worker{ID: "a", URL: "http://a"})
	rec := httptest.NewRecorder()
	g.HandleChat(rec, httptest.NewRequest(http.MethodGet, "/api/chat", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
