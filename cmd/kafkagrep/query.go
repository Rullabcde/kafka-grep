package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

type compiledExpr interface {
	Eval(key, value []byte, headers map[string]string) bool
}

// fieldExpr is implemented by expressions that match on a single JSON field,
// so the UI can highlight the field that caused the match.
type fieldExpr interface {
	MatchedField() (field, target string, ok bool)
}

// compileCEL parses simple field/op/target expressions without pulling in cel-go.
// Supports: value.field == "x", key == "y", headers.source contains "z".
func compileCEL(expr string) (compiledExpr, error) {
	expr = strings.TrimSpace(expr)
	for _, op := range []string{"==", "!=", "contains"} {
		if strings.Contains(expr, op) {
			parts := strings.SplitN(expr, op, 2)
			if len(parts) == 2 {
				field := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				return &simpleExpr{field: field, op: op, target: val}, nil
			}
		}
	}
	return nil, errNotExpr
}

var errNotExpr = stringErr("not an expression")

type stringErr string

func (s stringErr) Error() string { return string(s) }

type simpleExpr struct {
	field  string
	op     string
	target string
}

func (e *simpleExpr) MatchedField() (string, string, bool) {
	if strings.HasPrefix(e.field, "value.") {
		return strings.TrimPrefix(e.field, "value."), e.target, true
	}
	return "", "", false
}

func (e *simpleExpr) Eval(key, value []byte, headers map[string]string) bool {
	var got string
	switch {
	case e.field == "key":
		got = string(key)
	case strings.HasPrefix(e.field, "headers."):
		hKey := strings.TrimPrefix(e.field, "headers.")
		got = headers[hKey]
	case strings.HasPrefix(e.field, "value."):
		// Fast-path: skip unmarshal if target is not in raw bytes.
		if len(e.target) > 0 && !bytes.Contains(value, []byte(e.target)) {
			return false
		}
		path := strings.TrimPrefix(e.field, "value.")
		got = extractJSON(value, path)
	case e.field == "value":
		got = string(value)
	default:
		return false
	}

	switch e.op {
	case "==":
		return got == e.target
	case "!=":
		return got != e.target
	case "contains":
		return strings.Contains(got, e.target)
	default:
		return false
	}
}

func extractJSON(data []byte, field string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	parts := strings.Split(field, ".")
	var cur any = m
	for _, p := range parts {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = asMap[p]
	}
	if cur == nil {
		return ""
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), "\"")
}
