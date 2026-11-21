package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFilterSubstring memastikan pencarian substring bekerja di key/value/headers.
func TestFilterSubstring(t *testing.T) {
	f := NewFilter("payment", FilterOptions{MatchKey: true, MatchValue: true, MatchHeaders: true})
	if !f.Match([]byte("payment-123"), []byte(`{"txn":"x"}`), map[string]string{"src": "api"}) {
		t.Fatal("expected match on key")
	}
	if !f.Match([]byte("k"), []byte(`{"event":"payment_received"}`), nil) {
		t.Fatal("expected match on value")
	}
	if !f.Match([]byte("k"), []byte(`{}`), map[string]string{"origin": "payment_service"}) {
		t.Fatal("expected match on header")
	}
	if f.Match([]byte("abc"), []byte(`{"event":"refund"}`), nil) {
		t.Fatal("expected no match")
	}
}

// TestFilterRegex memastikan pola /regex/ bekerja.
func TestFilterRegex(t *testing.T) {
	f := NewFilter(`/order-\d+/`, FilterOptions{MatchValue: true})
	if !f.Match(nil, []byte(`{"order_id":"order-42"}`), nil) {
		t.Fatal("expected regex match")
	}
	if f.Match(nil, []byte(`{"order_id":"abc"}`), nil) {
		t.Fatal("expected no regex match")
	}
}

// TestFilterCEL memastikan ekspresi CEL sederhana bekerja.
func TestFilterCEL(t *testing.T) {
	f := NewFilter(`value.userId == "123"`, FilterOptions{MatchValue: true})
	if !f.Match(nil, []byte(`{"userId":"123","name":"x"}`), nil) {
		t.Fatal("expected CEL match on userId")
	}
	if f.Match(nil, []byte(`{"userId":"456"}`), nil) {
		t.Fatal("expected no CEL match")
	}

	f2 := NewFilter(`headers.source contains "pay"`, FilterOptions{MatchHeaders: true})
	if !f2.Match(nil, nil, map[string]string{"source": "payment"}) {
		t.Fatal("expected CEL header contains match")
	}
}

// TestFilterCELFastPath memastikan target yang tidak ada di raw bytes langsung
// di-skip tanpa unmarshal JSON (fast-path optimasi untuk payload besar).
func TestFilterCELFastPath(t *testing.T) {
	// Payload besar (meniru HTML berita) dengan id di ujung.
	big := []byte(`{"content":"<p>` + strings.Repeat("x", 5000) + `</p>","id":"abc123"}`)

	f := NewFilter(`value.id == "abc123"`, FilterOptions{MatchValue: true})
	if !f.Match(nil, big, nil) {
		t.Fatal("expected fast-path match when id present")
	}

	// id tidak ada -> harus cepat salah, tanpa unmarshal.
	f2 := NewFilter(`value.id == "zzz999"`, FilterOptions{MatchValue: true})
	if f2.Match(nil, big, nil) {
		t.Fatal("expected no match when id absent")
	}
}

// TestFilterEmpty memastikan query kosong match semua.
func TestFilterEmpty(t *testing.T) {
	f := NewFilter("", FilterOptions{MatchValue: true})
	if !f.Match(nil, []byte("anything"), nil) {
		t.Fatal("empty filter should match all")
	}
}

// TestMatchRegex memastikan filter regex nama topik bekerja.
func TestMatchRegex(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"orders|payments", "orders", true},
		{"orders|payments", "payments", true},
		{"orders|payments", "notifications", false},
		{"^order-.*", "order-events", true},
		{"^order-.*", "billing", false},
		{"", "anything", true}, // pattern kosong -> match semua
	}
	for _, c := range cases {
		if got := matchRegex(c.pattern, c.name); got != c.want {
			t.Errorf("matchRegex(%q,%q)=%v want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// TestBuildStartOffset memastikan perhitungan offset awal benar.
func TestBuildStartOffset(t *testing.T) {
	cases := []struct {
		mode   string
		latest int64
		from   int64
		per    int64
		want   int64
	}{
		{"from-start", 1000, 0, 0, 0},
		{"from-end", 1000, 0, 100, 900},
		{"from-end", 1000, 0, 5000, 0}, // per > latest -> clamp 0
		{"from-offset", 1000, 200, 0, 200},
		{"from-offset", 1000, -5, 0, 0},
	}
	for _, c := range cases {
		if got := buildStartOffset(c.mode, c.latest, c.from, c.per); got != c.want {
			t.Errorf("buildStartOffset(%q,%d,%d,%d)=%d want %d", c.mode, c.latest, c.from, c.per, got, c.want)
		}
	}
}

// TestExtractJSON memastikan ekstraksi field nested.
func TestExtractJSON(t *testing.T) {
	got := extractJSON([]byte(`{"a":{"b":"hello"},"n":42}`), "a.b")
	if got != "hello" {
		t.Fatalf("extractJSON nested = %q, want hello", got)
	}
	if extractJSON([]byte(`{"n":42}`), "n") != "42" {
		t.Fatal("extractJSON int failed")
	}
}

// TestContainsBytes memastikan naive search benar.
func TestContainsBytes(t *testing.T) {
	if !containsBytes([]byte("hello world"), []byte("world")) {
		t.Fatal("expected contains")
	}
	if containsBytes([]byte("hello"), []byte("world")) {
		t.Fatal("expected not contains")
	}
	if !containsBytes([]byte("abc"), []byte("")) {
		t.Fatal("empty needle should match")
	}
}

// TestTopicListDedup memastikan list topic tanpa duplikat.
func TestTopicListDedup(t *testing.T) {
	out := dedupe([]string{"a", "b", "a", "c"})
	if strings.Join(out, ",") != "a,b,c" {
		t.Fatalf("dedupe got %v", out)
	}
}

// TestSplitList memastikan parsing daftar.
func TestSplitList(t *testing.T) {
	if got := splitList("a,b, c, ,d"); strings.Join(got, ",") != "a,b,c,d" {
		t.Fatalf("splitList got %v", got)
	}
}

// TestContextCancel memastikan worker berhenti saat context dibatalkan.
func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Tidak ada Kafka; pastikan tidak panic dan mengembalikan error/empty.
	_, _ = SearchBatch(ctx, nil, SearchOptions{
		Brokers: []string{"localhost:9092"},
		Topics:  []string{"no-such-topic"},
		Limit:   1,
	})
}

func TestMatchedField(t *testing.T) {
	f := NewFilter(`value.id == "abc"`, FilterOptions{MatchValue: true})
	field, target, ok := f.MatchedField()
	if !ok || field != "id" || target != "abc" {
		t.Fatalf("MatchedField = %q %q %v", field, target, ok)
	}
	if _, _, ok := NewFilter("abc", FilterOptions{MatchValue: true}).MatchedField(); ok {
		t.Fatal("substring filter must not report a field")
	}
}

func TestProgressSnapshot(t *testing.T) {
	p := NewScanProgress()
	p.register("t", 0, 10)
	p.register("t", 1, 5)
	for i := 0; i < 4; i++ {
		p.advance("t", 0)
	}
	p.finish("t", 0)
	s := p.Snapshot()
	if s.Scanned != 4 || len(s.Partitions) != 2 {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.Partitions[0].Scanned != 4 || !s.Partitions[0].Done {
		t.Fatalf("partition 0 = %+v", s.Partitions[0])
	}
}

func TestClassifyError(t *testing.T) {
	cases := map[string]string{
		"topic \"x\" not found":           "topic",
		"dial tcp 10.0.0.1:9092: refused": "broker",
		"filter is not valid: bad regex":  "filter",
		"unexpected internal state":       "internal",
	}
	for msg, want := range cases {
		if got, _ := classifyError(errString(msg)); got != want {
			t.Errorf("classifyError(%q) = %q, want %q", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestParseSince(t *testing.T) {
	if got := parseSince(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := parseSince("2026-01-02T03:04:05Z"); got != 1767323045000 {
		t.Errorf("rfc3339 = %d", got)
	}
	if got := parseSince("1767323045"); got != 1767323045000 {
		t.Errorf("unix seconds = %d", got)
	}
	if got := parseSince("not-a-time"); got != 0 {
		t.Errorf("garbage = %d, want 0", got)
	}
}

func TestParseSinceRelative(t *testing.T) {
	for _, in := range []string{"1d", "12h", "30m"} {
		got := parseSince(in)
		if got <= 0 || got >= time.Now().UnixMilli() {
			t.Errorf("parseSince(%q) = %d, want past timestamp", in, got)
		}
	}
	if got := parseSince("1d"); time.Now().UnixMilli()-got < int64(23*time.Hour/time.Millisecond) {
		t.Errorf("1d resolved too recent: %d", got)
	}
}
