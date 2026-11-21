package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type SSEHandler struct {
	cl      *kgo.Client
	brokers []string
}

func NewSSEHandler(brokers []string) (*SSEHandler, error) {
	cl, err := NewClient(brokers)
	if err != nil {
		return nil, err
	}
	return &SSEHandler{cl: cl, brokers: brokers}, nil
}

// ServeHTTP handles GET /api/search and streams matches as server-sent events.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := &SearchOptions{
		Brokers:         h.brokers,
		Topics:          splitList(q.Get("topics")),
		RegexTopic:      q.Get("regex"),
		Query:           q.Get("q"),
		Mode:            firstNonEmpty(q.Get("mode"), "from-end"),
		FromOffset:      parseInt64(q.Get("from")),
		ToOffset:        parseInt64(q.Get("to")),
		Limit:           parseInt64(q.Get("limit")),
		PerPartition:    parseInt64(q.Get("per")),
		MaxConcurrent:   int(parseInt64(q.Get("workers"))),
		MatchKey:        parseBoolDefault(q.Get("key"), true),
		MatchValue:      parseBoolDefault(q.Get("value"), true),
		MatchHeaders:    parseBoolDefault(q.Get("headers"), true),
		CaseInsensitive: parseBoolDefault(q.Get("ci"), false),
		SinceMillis:     parseSince(q.Get("since")),
		Progress:        NewScanProgress(),
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.PerPartition <= 0 {
		opts.PerPartition = 20000
	}
	opts.defaults()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	out := &sseWriter{w: w, f: flusher}

	if err := validateQuery(opts.Query); err != nil {
		out.write("error", errorEvent{Kind: "filter", Message: err.Error()})
		return
	}

	ctx := r.Context()
	topics, err := topicList(ctx, h.cl, opts.Topics, opts.RegexTopic)
	if err != nil {
		kind, msg := classifyError(err)
		out.write("error", errorEvent{Kind: kind, Message: msg})
		return
	}
	if len(topics) == 0 {
		out.write("error", errorEvent{Kind: "topic", Message: "no topic matched the request"})
		return
	}

	start := time.Now()
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sink := &sseSink{out: out}
	attempts := &atomic.Int64{}
	errCh := make(chan error, 1)
	workers := makeWorkers(scanCtx, h.cl, topics, *opts, attempts, sink, errCh)

	doneCh := make(chan struct{})
	go func() { workers.Wait(); close(doneCh) }()

	progressDone := make(chan struct{})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-t.C:
				out.write("progress", opts.Progress.Snapshot())
			}
		}
	}()

	var scanErr error
	select {
	case <-doneCh:
		// A worker may report its error in the same instant it finishes.
		select {
		case scanErr = <-errCh:
		default:
		}
	case scanErr = <-errCh:
		cancel()
		<-doneCh
	case <-ctx.Done():
		cancel()
		<-doneCh
		close(progressDone)
		return
	}
	close(progressDone)

	if scanErr != nil && scanErr != errLimitReached && scanErr != context.Canceled {
		kind, msg := classifyError(scanErr)
		out.write("error", errorEvent{Kind: kind, Message: msg})
		return
	}

	out.write("done", doneEvent{
		Matches:   sink.matches.Load(),
		Scanned:   opts.Progress.Snapshot().Scanned,
		ElapsedMS: time.Since(start).Milliseconds(),
	})
}

type errorEvent struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type doneEvent struct {
	Matches   int64 `json:"matches"`
	Scanned   int64 `json:"scanned"`
	ElapsedMS int64 `json:"elapsed_ms"`
}

type sseWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
	f  http.Flusher
}

func (s *sseWriter) write(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b)
	s.f.Flush()
}

type sseSink struct {
	out     *sseWriter
	matches atomic.Int64
}

func (s *sseSink) Emit(m Match) error {
	s.out.write("match", m)
	s.matches.Add(1)
	return nil
}

// Done is a no-op: the handler writes the single done event after all workers exit.
func (s *sseSink) Done() {}

func validateQuery(q string) error {
	q = strings.TrimSpace(q)
	if len(q) > 2 && strings.HasPrefix(q, "/") && strings.HasSuffix(q, "/") {
		if _, err := regexp.Compile(q[1 : len(q)-1]); err != nil {
			return fmt.Errorf("filter is not valid: %v", err)
		}
	}
	return nil
}

// classifyError maps a scan error to a UI-facing kind and raw message.
func classifyError(err error) (kind, message string) {
	if err == nil {
		return "internal", ""
	}
	s := err.Error()
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "not found"):
		return "topic", s
	case strings.Contains(lower, "dial") || strings.Contains(lower, "connect") ||
		strings.Contains(lower, "refused") || strings.Contains(lower, "broker") ||
		strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") ||
		strings.Contains(lower, "eof"):
		return "broker", s
	case strings.Contains(lower, "filter") || strings.Contains(lower, "regexp"):
		return "filter", s
	default:
		return "internal", s
	}
}

func UIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
	})
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func parseBoolDefault(s string, def bool) bool {
	if s == "" {
		return def
	}
	return s == "true" || s == "1"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
