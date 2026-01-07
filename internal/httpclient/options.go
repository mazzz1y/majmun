package httpclient

import (
	"majmun/internal/config/common"
	"sort"
	"strings"
	"time"
)

type Options struct {
	TTL         time.Duration
	Retention   time.Duration
	Compression bool
	HTTPHeaders []common.NameValue
}

func (o Options) key() string {
	var b strings.Builder
	b.WriteString("ttl=")
	b.WriteString(o.TTL.String())
	b.WriteString(";ret=")
	b.WriteString(o.Retention.String())
	b.WriteString(";cmp=")
	if o.Compression {
		b.WriteString("1")
	} else {
		b.WriteString("0")
	}
	b.WriteString(";hdr=")
	b.WriteString(canonicalHeaders(o.HTTPHeaders))
	return b.String()
}

func canonicalHeaders(headers []common.NameValue) string {
	if len(headers) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(headers))
	for _, h := range headers {
		pairs = append(pairs, strings.ToLower(h.Name)+":"+h.Value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "\n")
}
