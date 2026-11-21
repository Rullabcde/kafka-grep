package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		brokersFlag = flag.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
		topicsFlag  = flag.String("topics", "", "comma-separated topic names")
		regexFlag   = flag.String("regex", "", "topic name regex pattern")
		queryFlag   = flag.String("q", "", "search query (text, /regex/, or CEL expr e.g. value.id == \"123\")")
		modeFlag    = flag.String("mode", "from-end", "scan mode: from-end | from-start | from-offset")
		fromOffFlag = flag.Int64("from-offset", 0, "starting offset if mode is from-offset")
		limitFlag   = flag.Int64("limit", 50, "max matched messages to return")
		perPartFlag = flag.Int64("per-partition", 5000, "max messages to scan backward per partition in from-end mode")
		sinceFlag   = flag.String("since", "", "only scan messages newer than this: 1d, 12h, 30m, RFC3339, or unix seconds")
		workersFlag = flag.Int("workers", 32, "max concurrent topic scanners")
		webPort     = flag.String("web", "", "run as web UI server on given port (e.g. :8080)")
		ciFlag      = flag.Bool("ci", false, "case insensitive matching")
		jsonFlag    = flag.Bool("json", false, "output results as JSON Lines (NDJSON) for piping to jq")
	)
	flag.Parse()

	brokers := splitList(*brokersFlag)
	if len(brokers) == 0 {
		fmt.Fprintln(os.Stderr, "error: -brokers is required")
		os.Exit(1)
	}

	if *webPort != "" {
		runWebServer(*webPort, brokers)
		return
	}

	if *topicsFlag == "" && *regexFlag == "" {
		fmt.Fprintln(os.Stderr, "error: specify -topics or -regex, or pass -web=:8080 for web mode")
		os.Exit(1)
	}

	topics := splitList(*topicsFlag)
	opts := SearchOptions{
		Brokers:         brokers,
		Topics:          topics,
		RegexTopic:      *regexFlag,
		Query:           *queryFlag,
		Mode:            *modeFlag,
		FromOffset:      *fromOffFlag,
		Limit:           *limitFlag,
		PerPartition:    *perPartFlag,
		SinceMillis:     parseSince(*sinceFlag),
		MaxConcurrent:   *workersFlag,
		MatchKey:        true,
		MatchValue:      true,
		MatchHeaders:    true,
		CaseInsensitive: *ciFlag,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logOut := os.Stdout
	if *jsonFlag {
		logOut = os.Stderr
	}

	fmt.Fprintf(logOut, "Searching Kafka (brokers=%s, topics=%v, q=%q, mode=%s, limit=%d)...\n",
		strings.Join(brokers, ","), opts.Topics, opts.Query, opts.Mode, opts.Limit)

	start := time.Now()
	res, err := SearchBatch(ctx, nil, opts)
	elapsed := time.Since(start)

	if err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "search error: %v\n", err)
	}

	if res != nil {
		if *jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetEscapeHTML(false)
			for _, m := range res.Matches {
				_ = enc.Encode(m)
			}
			return
		}
		for _, m := range res.Matches {
			fmt.Printf("[%s #%d @%d] ts=%s key=%s\n  %s\n",
				m.Topic, m.Partition, m.Offset,
				time.UnixMilli(m.Timestamp).Format(time.RFC3339),
				m.Key,
				m.Value,
			)
		}
		fmt.Fprintf(logOut, "\nDone: found %d matches in %v\n", len(res.Matches), elapsed)
	}
}

func runWebServer(addr string, brokers []string) {
	if !strings.HasPrefix(addr, ":") {
		addr = ":" + addr
	}
	sseHandler, err := NewSSEHandler(brokers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/search", sseHandler)
	mux.Handle("/fonts/", fontHandler())
	mux.Handle("/", UIHandler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Fprintf(os.Stderr, "Web UI: http://localhost%s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// parseSince returns epoch millis from RFC3339, unix seconds, or a relative
// duration back from now (1d, 12h, 30m). Relative values are the common case:
// "show me the last day" beats typing a timestamp.
func parseSince(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return n * 1000
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.ParseFloat(days, 64); err == nil && n > 0 {
			return time.Now().Add(-time.Duration(n * float64(24*time.Hour))).UnixMilli()
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return time.Now().Add(-d).UnixMilli()
	}
	fmt.Fprintf(os.Stderr, "warning: ignoring -since=%q (want RFC3339, unix seconds, or 1d/12h/30m)\n", s)
	return 0
}
