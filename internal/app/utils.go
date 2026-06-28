package app

import (
	"majmun/internal/config"
	"majmun/internal/config/common"
	"majmun/internal/config/proxy"
	"time"
)

type httpClientSettings struct {
	CacheEnabled bool
	TTL          time.Duration
	Retention    time.Duration
	Compression  bool
	Headers      []common.NameValue
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
		Headers:      pr.HTTPClient.Headers,
	}
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
		if len(p.HTTPClient.Headers) > 0 {
			result.HTTPClient.Headers = p.HTTPClient.Headers
		}

		result.Stream = mergeHandlers(result.Stream, p.Stream)
		mergeSegmenter(&result.Segmenter, p.Segmenter)

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
		result.TemplateVars = common.MergeNameValues(result.TemplateVars, h.TemplateVars)
		result.EnvVars = common.MergeNameValues(result.EnvVars, h.EnvVars)
	}

	return result
}

func mergeSegmenter(dst *proxy.Segmenter, src proxy.Segmenter) {
	if len(src.Command) > 0 {
		dst.Command = src.Command
	}
	dst.TemplateVars = common.MergeNameValues(dst.TemplateVars, src.TemplateVars)
	dst.EnvVars = common.MergeNameValues(dst.EnvVars, src.EnvVars)
	if src.InitSegments != 0 {
		dst.InitSegments = src.InitSegments
	}
	if src.ReadyTimeout != 0 {
		dst.ReadyTimeout = src.ReadyTimeout
	}
}

// mergePlayouts cascades playout config global -> playlist -> channel, each level overriding
// the previous field by field. Template/env vars merge by name; scheduling/listing fields
// keep their pointer/empty zero value when unset so the lower level inherits.
func mergePlayouts(playouts ...config.Playout) config.Playout {
	result := config.Playout{}
	for _, p := range playouts {
		if len(p.Command) > 0 {
			result.Command = p.Command
		}
		result.TemplateVars = common.MergeNameValues(result.TemplateVars, p.TemplateVars)
		result.EnvVars = common.MergeNameValues(result.EnvVars, p.EnvVars)
		if p.InitSegments != 0 {
			result.InitSegments = p.InitSegments
		}
		if p.ReadyTimeout != 0 {
			result.ReadyTimeout = p.ReadyTimeout
		}
		if p.StateDir != "" {
			result.StateDir = p.StateDir
		}
		if p.Logo != "" {
			result.Logo = p.Logo
		}
		if len(p.Extensions) > 0 {
			result.Extensions = p.Extensions
		}
		if p.Order != "" {
			result.Order = p.Order
		}
		if len(p.SeasonPatterns) > 0 {
			result.SeasonPatterns = p.SeasonPatterns
		}
		if len(p.EpisodePatterns) > 0 {
			result.EpisodePatterns = p.EpisodePatterns
		}
		if p.RefreshInterval != nil {
			result.RefreshInterval = p.RefreshInterval
		}
		if p.EPGDuration != 0 {
			result.EPGDuration = p.EPGDuration
		}
		if p.ScheduleSwapAt != "" {
			result.ScheduleSwapAt = p.ScheduleSwapAt
		}
		if p.Metadata.Title != nil {
			result.Metadata.Title = p.Metadata.Title
		}
		if p.Metadata.Description != nil {
			result.Metadata.Description = p.Metadata.Description
		}
		if p.Metadata.Category != nil {
			result.Metadata.Category = p.Metadata.Category
		}
	}
	return result
}
