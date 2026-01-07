package app

import (
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"strings"
	"time"
)

type httpClientSettings struct {
	CacheEnabled bool
	TTL          time.Duration
	Retention    time.Duration
	Compression  bool
	HTTPHeaders  []common.NameValue
}

func httpClientOptions(pr proxy.Proxy) httpClientSettings {
	cacheEnabled := false
	if pr.HTTPClient.Cache.Enabled != nil {
		cacheEnabled = *pr.HTTPClient.Cache.Enabled
	}

	var ttl, retention time.Duration
	var compression bool

	if pr.HTTPClient.Cache.TTL != nil {
		ttl = time.Duration(*pr.HTTPClient.Cache.TTL)
	}
	if pr.HTTPClient.Cache.Retention != nil {
		retention = time.Duration(*pr.HTTPClient.Cache.Retention)
	}
	if pr.HTTPClient.Cache.Compression != nil {
		compression = *pr.HTTPClient.Cache.Compression
	}

	return httpClientSettings{
		CacheEnabled: cacheEnabled,
		TTL:          ttl,
		Retention:    retention,
		Compression:  compression,
		HTTPHeaders:  pr.HTTPClient.HTTPHeaders,
	}
}

func uniqueNames(names []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, n := range names {
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			result = append(result, n)
		}
	}
	return result
}

func mergeProxies(proxies ...proxy.Proxy) proxy.Proxy {
	result := proxy.Proxy{}
	for _, p := range proxies {
		if p.Enabled != nil {
			result.Enabled = p.Enabled
		}
		if p.ConcurrentStreams > 0 {
			result.ConcurrentStreams = p.ConcurrentStreams
		}

		if p.HTTPClient.Cache.Enabled != nil {
			result.HTTPClient.Cache.Enabled = p.HTTPClient.Cache.Enabled
		}
		if p.HTTPClient.Cache.TTL != nil {
			result.HTTPClient.Cache.TTL = p.HTTPClient.Cache.TTL
		}
		if p.HTTPClient.Cache.Retention != nil {
			result.HTTPClient.Cache.Retention = p.HTTPClient.Cache.Retention
		}
		if p.HTTPClient.Cache.Compression != nil {
			result.HTTPClient.Cache.Compression = p.HTTPClient.Cache.Compression
		}
		if len(p.HTTPClient.HTTPHeaders) > 0 {
			result.HTTPClient.HTTPHeaders = p.HTTPClient.HTTPHeaders
		}

		result.Stream = mergeHandlers(result.Stream, p.Stream)

		result.Error.Handler = mergeHandlers(result.Error.Handler, p.Error.Handler)

		result.Error.RateLimitExceeded = mergeHandlers(
			result.Error.Handler, result.Error.RateLimitExceeded, p.Error.RateLimitExceeded)

		result.Error.LinkExpired = mergeHandlers(
			result.Error.Handler, result.Error.LinkExpired, p.Error.LinkExpired)

		result.Error.UpstreamError = mergeHandlers(
			result.Error.Handler, result.Error.UpstreamError, p.Error.UpstreamError)
	}

	return result
}

func mergeHandlers(handlers ...proxy.Handler) proxy.Handler {
	result := proxy.Handler{}
	for _, h := range handlers {
		if len(h.Command) > 0 {
			result.Command = h.Command
		}
		mergePairs(&result.TemplateVars, h.TemplateVars)
		mergePairs(&result.EnvVars, h.EnvVars)
	}

	return result
}

func mergePairs[T ~[]common.NameValue](result *T, handler T) {
	if len(handler) == 0 {
		return
	}
	varMap := make(map[string]string, len(*result)+len(handler))
	for _, v := range *result {
		varMap[v.Name] = v.Value
	}
	for _, v := range handler {
		varMap[v.Name] = v.Value
	}
	merged := make([]common.NameValue, 0, len(varMap))
	for name, value := range varMap {
		merged = append(merged, common.NameValue{Name: name, Value: value})
	}
	*result = merged
}

func mergeNameValues(base []common.NameValue, override []common.NameValue) []common.NameValue {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}

	m := make(map[string]string, len(base)+len(override))

	for _, nv := range base {
		m[strings.ToLower(nv.Name)] = nv.Value
	}
	for _, nv := range override {
		m[strings.ToLower(nv.Name)] = nv.Value
	}

	result := make([]common.NameValue, 0, len(m))
	for name, value := range m {
		result = append(result, common.NameValue{Name: name, Value: value})
	}
	return result
}
