package gateway

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"llm-gateway/internal/metrics"
)

// HandleChat proxies a chat request to the selected worker and re-streams
// the response to the client as Server-Sent Events, recording TTFT and ITL.
func (g *Gateway) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	prefixHash := extractPrefixHash(body, g.prefixBytes)
	worker, isHit := g.SelectWorker(prefixHash)

	cacheStatus := "miss"
	if isHit {
		cacheStatus = "hit"
	}

	slog.Info("routed request", "prefix", hashHex(prefixHash), "worker", worker.ID, "cache", cacheStatus)

	atomic.AddInt64(&worker.ActiveConns, 1)
	defer atomic.AddInt64(&worker.ActiveConns, -1)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, worker.URL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Proxy error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Warn("worker unreachable", "worker", worker.ID, "err", err)
		http.Error(w, "Worker offline", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		io.Copy(w, resp.Body)
		return
	}

	reader := bufio.NewReader(resp.Body)
	firstToken := true
	var lastChunk time.Time

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			now := time.Now()

			if firstToken {
				ttft := now.Sub(start).Seconds()
				metrics.TTFT.WithLabelValues(worker.ID, cacheStatus).Observe(ttft)
				slog.Info("first token", "ttft_s", ttft, "worker", worker.ID, "cache", cacheStatus)
				firstToken = false
			} else {
				metrics.ITL.WithLabelValues(worker.ID).Observe(now.Sub(lastChunk).Seconds())
			}
			lastChunk = now

			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}

		if readErr != nil {
			break
		}
	}
}

// HandleHealth reports that the gateway process is up.
func HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}
