package main

import (
	"bytes"
	"regexp"
	"strings"
)

type FilterOptions struct {
	MatchKey        bool
	MatchValue      bool
	MatchHeaders    bool
	CaseInsensitive bool
}

type Filter struct {
	substr   []byte
	re       *regexp.Regexp
	opts     FilterOptions
	compiled compiledExpr
}

func NewFilter(query string, opts FilterOptions) *Filter {
	f := &Filter{opts: opts}
	q := strings.TrimSpace(query)
	if q == "" {
		return f
	}
	if strings.HasPrefix(q, "/") && strings.HasSuffix(q, "/") && len(q) > 2 {
		re, err := regexp.Compile(q[1 : len(q)-1])
		if err == nil {
			f.re = re
			return f
		}
	}
	if c, err := compileCEL(q); err == nil {
		f.compiled = c
		return f
	}
	if opts.CaseInsensitive {
		q = strings.ToLower(q)
	}
	f.substr = []byte(q)
	return f
}

func (f *Filter) Match(key, value []byte, headers map[string]string) bool {
	if f.substr == nil && f.re == nil && f.compiled == nil {
		return true
	}
	if f.compiled != nil {
		return f.compiled.Eval(key, value, headers)
	}
	if f.opts.MatchKey && f.matchBytes(key) {
		return true
	}
	if f.opts.MatchValue && f.matchBytes(value) {
		return true
	}
	if f.opts.MatchHeaders {
		for k, v := range headers {
			if f.matchString(k) || f.matchString(v) {
				return true
			}
		}
	}
	return false
}

// MatchedField reports the JSON field an expression filter matched on, if any.
func (f *Filter) MatchedField() (field, target string, ok bool) {
	if fe, isField := f.compiled.(fieldExpr); isField {
		return fe.MatchedField()
	}
	return "", "", false
}

func (f *Filter) matchBytes(b []byte) bool {
	if f.re != nil {
		return f.re.Match(b)
	}
	if f.opts.CaseInsensitive {
		return bytes.Contains(bytes.ToLower(b), f.substr)
	}
	return bytes.Contains(b, f.substr)
}

func (f *Filter) matchString(s string) bool {
	if f.re != nil {
		return f.re.MatchString(s)
	}
	if f.opts.CaseInsensitive {
		return strings.Contains(strings.ToLower(s), string(f.substr))
	}
	return strings.Contains(s, string(f.substr))
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	return bytes.Contains(haystack, needle)
}

func matchRegex(pattern, name string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(name)
}
