package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type Match struct {
	Topic        string            `json:"topic"`
	Partition    int32             `json:"partition"`
	Offset       int64             `json:"offset"`
	Timestamp    int64             `json:"timestamp_ms"`
	Key          string            `json:"key,omitempty"`
	Value        string            `json:"value,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	MatchedField string            `json:"matched_field,omitempty"`
	MatchedValue string            `json:"matched_value,omitempty"`
}

// ScanProgress exposes live per-partition scan counts for the UI.
// count is approximate: it is read while scans are running.
type ScanProgress struct {
	mu    sync.Mutex
	parts map[string]map[int32]*partProgress
	total atomic.Int64
}

type partProgress struct {
	scanned int64
	total   int64
	done    bool
}

type ProgressSnapshot struct {
	Scanned    int64          `json:"scanned"`
	Partitions []PartSnapshot `json:"partitions"`
}

type PartSnapshot struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Scanned   int64  `json:"scanned"`
	Total     int64  `json:"total"`
	Done      bool   `json:"done"`
}

func NewScanProgress() *ScanProgress {
	return &ScanProgress{parts: map[string]map[int32]*partProgress{}}
}

func (p *ScanProgress) register(topic string, partition int32, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.parts[topic] == nil {
		p.parts[topic] = map[int32]*partProgress{}
	}
	p.parts[topic][partition] = &partProgress{total: total}
}

func (p *ScanProgress) advance(topic string, partition int32) {
	p.total.Add(1)
	p.mu.Lock()
	if pp := p.parts[topic][partition]; pp != nil {
		pp.scanned++
	}
	p.mu.Unlock()
}

func (p *ScanProgress) finish(topic string, partition int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pp := p.parts[topic][partition]; pp != nil {
		pp.done = true
	}
}

func (p *ScanProgress) Snapshot() ProgressSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := ProgressSnapshot{Scanned: p.total.Load()}
	for topic, m := range p.parts {
		for part, pp := range m {
			out.Partitions = append(out.Partitions, PartSnapshot{
				Topic: topic, Partition: part,
				Scanned: pp.scanned, Total: pp.total, Done: pp.done,
			})
		}
	}
	sort.Slice(out.Partitions, func(i, j int) bool {
		if out.Partitions[i].Topic != out.Partitions[j].Topic {
			return out.Partitions[i].Topic < out.Partitions[j].Topic
		}
		return out.Partitions[i].Partition < out.Partitions[j].Partition
	})
	return out
}

type Sink interface {
	Emit(m Match) error
	Done()
}

type ScanResult struct {
	Matches []Match
	Err     error
}

type SearchOptions struct {
	Brokers         []string
	Topics          []string
	RegexTopic      string
	Query           string
	Mode            string
	FromOffset      int64
	ToOffset        int64
	Limit           int64
	PerPartition    int64
	MaxConcurrent   int
	MatchKey        bool
	MatchValue      bool
	MatchHeaders    bool
	CaseInsensitive bool
	MaxPoll         int
	Progress        *ScanProgress

	// SinceMillis scans only messages at or after this timestamp. The broker
	// resolves it to an offset, so the old range is never transferred.
	SinceMillis int64
}

func (o *SearchOptions) defaults() {
	if o.MaxPoll <= 0 {
		o.MaxPoll = 500
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = 64
	}
	if !o.MatchKey && !o.MatchValue && !o.MatchHeaders {
		o.MatchKey = true
		o.MatchValue = true
		o.MatchHeaders = true
	}
}

func NewClient(brokers []string) (*kgo.Client, error) {
	return kgo.NewClient(kgo.SeedBrokers(brokers...))
}

func NewScanClient(brokers []string, partitions map[string]map[int32]kgo.Offset) (*kgo.Client, error) {
	return kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumePartitions(partitions),
	)
}

func topicList(ctx context.Context, cl *kgo.Client, topics []string, regexTopic string) ([]string, error) {
	if len(topics) > 0 {
		return dedupe(topics), nil
	}
	adm := kadm.NewClient(cl)
	all, err := adm.ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	var names []string
	for name := range all {
		if regexTopic == "" || matchRegex(regexTopic, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func buildStartOffset(mode string, latest int64, fromOffset, perPartition int64) int64 {
	switch mode {
	case "from-start":
		return 0
	case "from-offset":
		if fromOffset > 0 {
			return fromOffset
		}
		return 0
	default:
		if perPartition > 0 {
			start := latest - perPartition
			if start < 0 {
				start = 0
			}
			return start
		}
		return 0
	}
}

func SearchBatch(ctx context.Context, cl *kgo.Client, opts SearchOptions) (*ScanResult, error) {
	opts.defaults()
	if len(opts.Brokers) == 0 && cl == nil {
		return nil, fmt.Errorf("brokers required")
	}
	own := cl == nil
	if cl == nil {
		var err error
		cl, err = NewClient(opts.Brokers)
		if err != nil {
			return nil, err
		}
	}
	if own {
		defer cl.Close()
	}

	topics, err := topicList(ctx, cl, opts.Topics, opts.RegexTopic)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return &ScanResult{}, nil
	}

	var mu sync.Mutex
	var res ScanResult
	matched := &atomic.Int64{}

	collect := func(m Match) error {
		mu.Lock()
		defer mu.Unlock()
		res.Matches = append(res.Matches, m)
		if opts.Limit > 0 && int64(len(res.Matches)) >= opts.Limit {
			return errLimitReached
		}
		return nil
	}

	done := make(chan struct{})
	errCh := make(chan error, 1)
	workers := makeWorkers(ctx, cl, topics, opts, matched, sinkFunc(collect), errCh)
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return &res, ctx.Err()
	case e := <-errCh:
		return &res, e
	}
	return &res, res.Err
}

var errLimitReached = fmt.Errorf("limit reached")

func sinkFunc(fn func(Match) error) Sink {
	return &funcSink{fn: fn}
}

type funcSink struct{ fn func(Match) error }

func (f *funcSink) Emit(m Match) error { return f.fn(m) }
func (f *funcSink) Done()              {}

// makeWorkers runs one goroutine per topic, bounded by MaxConcurrent.
func makeWorkers(ctx context.Context, cl *kgo.Client, topics []string, opts SearchOptions,
	matched *atomic.Int64, sink Sink, errCh chan<- error) *sync.WaitGroup {
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.MaxConcurrent)

	for _, topic := range topics {
		wg.Add(1)
		sem <- struct{}{}
		go func(topic string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := scanTopic(ctx, cl, topic, opts, matched, sink); err != nil {
				if err != errLimitReached {
					select {
					case errCh <- err:
					default:
					}
				}
			}
		}(topic)
	}
	return &wg
}

// scanTopic consumes all partitions of a single topic with one shared client.
// Each partition is tracked independently; the loop exits once every partition
// reaches its scan end or the global limit is hit.
func scanTopic(ctx context.Context, cl *kgo.Client, topic string,
	opts SearchOptions, matched *atomic.Int64, sink Sink) error {

	parts, err := partitionsForTopic(ctx, cl, topic)
	if err != nil {
		return fmt.Errorf("list partitions %s: %w", topic, err)
	}
	if len(parts) == 0 {
		return nil
	}

	adm := kadm.NewClient(cl)
	endOffsets, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		return fmt.Errorf("end offsets %s: %w", topic, err)
	}

	// A since time is resolved by the broker to a start offset, so history
	// before it is never fetched at all.
	sinceOffsets := map[int32]int64{}
	if opts.SinceMillis > 0 {
		listed, err := adm.ListOffsetsAfterMilli(ctx, opts.SinceMillis, topic)
		if err != nil {
			return fmt.Errorf("offsets after %d for %s: %w", opts.SinceMillis, topic, err)
		}
		for _, part := range parts {
			if lo, ok := listed.Lookup(topic, part); ok && lo.Err == nil && lo.Offset > 0 {
				sinceOffsets[part] = lo.Offset
			}
		}
	}

	type partitionState struct {
		part    int32
		start   int64
		scanEnd int64
		done    bool
	}
	states := make([]partitionState, len(parts))
	assignMap := make(map[int32]kgo.Offset, len(parts))
	for i, part := range parts {
		states[i].part = part
		listed, ok := endOffsets.Lookup(topic, part)
		if !ok {
			states[i].done = true
			continue
		}
		latest := listed.Offset
		start := buildStartOffset(opts.Mode, latest, opts.FromOffset, opts.PerPartition)
		// A since time can only narrow the window: take whichever is later.
		if opts.SinceMillis > 0 {
			if off, ok := sinceOffsets[part]; ok && off > start {
				start = off
			}
		}
		if start >= latest {
			states[i].done = true
			continue
		}
		to := latest
		if opts.ToOffset > 0 && opts.ToOffset < to {
			to = opts.ToOffset
		}
		scanEnd := int64(0)
		if opts.PerPartition > 0 {
			scanEnd = start + opts.PerPartition
			if scanEnd > to {
				scanEnd = to
			}
		} else {
			scanEnd = to
		}
		states[i] = partitionState{part: part, start: start, scanEnd: scanEnd}
		assignMap[part] = kgo.NewOffset().At(start)
	}

	activeCount := 0
	for i := range states {
		if !states[i].done {
			activeCount++
			if opts.Progress != nil {
				opts.Progress.register(topic, states[i].part, states[i].scanEnd-states[i].start)
			}
		}
	}
	if activeCount == 0 || len(assignMap) == 0 {
		return nil
	}

	cons, err := NewScanClient(opts.Brokers, map[string]map[int32]kgo.Offset{topic: assignMap})
	if err != nil {
		return err
	}
	defer cons.Close()

	stateByPart := make(map[int32]*partitionState, len(states))
	for i := range states {
		stateByPart[states[i].part] = &states[i]
	}

	filter := NewFilter(opts.Query, FilterOptions{
		MatchKey:        opts.MatchKey,
		MatchValue:      opts.MatchValue,
		MatchHeaders:    opts.MatchHeaders,
		CaseInsensitive: opts.CaseInsensitive,
	})

	for activeCount > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opts.Limit > 0 && matched.Load() >= opts.Limit {
			return errLimitReached
		}

		fetches := cons.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			var fatal error
			for _, e := range errs {
				if e.Err != nil && !strings.Contains(e.Err.Error(), "EOF") {
					fatal = e.Err
				}
			}
			if fatal != nil {
				return fmt.Errorf("fetch %s: %w", topic, fatal)
			}
			return nil
		}
		if fetches.Empty() {
			continue
		}

		var finished []int32
		limitHit := false

		fetches.EachPartition(func(ftp kgo.FetchTopicPartition) {
			if limitHit || ftp.Topic != topic {
				return
			}
			st, ok := stateByPart[ftp.Partition]
			if !ok || st.done {
				return
			}
			if ftp.Err != nil {
				if !strings.Contains(ftp.Err.Error(), "EOF") {
					st.done = true
					activeCount--
					finished = append(finished, st.part)
				}
				return
			}
			for _, rec := range ftp.Records {
				if st.scanEnd > 0 && rec.Offset >= st.scanEnd {
					st.done = true
					activeCount--
					finished = append(finished, st.part)
					break
				}
				if opts.Progress != nil {
					opts.Progress.advance(topic, ftp.Partition)
				}
				// The window is [start, scanEnd). A window that ends exactly at
				// the last message never yields an offset >= scanEnd, so this
				// record is the last one and the partition finishes after it.
				last := st.scanEnd > 0 && rec.Offset+1 >= st.scanEnd
				headers := headersMap(rec)
				if filter.Match(rec.Key, rec.Value, headers) {
					m := Match{
						Topic:     topic,
						Partition: ftp.Partition,
						Offset:    rec.Offset,
						Timestamp: rec.Timestamp.UnixMilli(),
						Key:       string(rec.Key),
						Value:     string(rec.Value),
						Headers:   headers,
					}
					if f, target, ok := filter.MatchedField(); ok {
						m.MatchedField = f
						m.MatchedValue = target
					}
					if opts.Limit > 0 && matched.Add(1) > opts.Limit {
						limitHit = true
						return
					}
					if err := sink.Emit(m); err != nil {
						return
					}
				}
				if last {
					st.done = true
					activeCount--
					finished = append(finished, st.part)
					break
				}
			}
		})

		if limitHit {
			return errLimitReached
		}
		for _, p := range finished {
			if opts.Progress != nil {
				opts.Progress.finish(topic, p)
			}
			cons.RemoveConsumePartitions(map[string][]int32{topic: {p}})
		}
	}

	return nil
}

func partitionsForTopic(ctx context.Context, cl *kgo.Client, topic string) ([]int32, error) {
	adm := kadm.NewClient(cl)
	md, err := adm.ListTopics(ctx, topic)
	if err != nil {
		return nil, err
	}
	detail, ok := md[topic]
	if !ok {
		return nil, fmt.Errorf("topic %q not found", topic)
	}
	parts := make([]int32, 0, len(detail.Partitions))
	for p := range detail.Partitions {
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		// kadm reports an unknown topic as an empty partition map, not an error.
		return nil, fmt.Errorf("topic %q not found", topic)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i] < parts[j] })
	return parts, nil
}

func headersMap(rec *kgo.Record) map[string]string {
	if len(rec.Headers) == 0 {
		return nil
	}
	m := make(map[string]string, len(rec.Headers))
	for _, h := range rec.Headers {
		m[h.Key] = string(h.Value)
	}
	return m
}
