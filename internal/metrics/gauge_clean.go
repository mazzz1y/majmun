package metrics

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	graceTime   = 5 * time.Minute
	cleanupTime = 5 * time.Minute
)

type AutoCleanGauge struct {
	desc        *prometheus.Desc
	values      map[string]float64
	zeroTimes   map[string]time.Time
	mu          *sync.RWMutex
	labelNames  []string
	labelValues []string
	stopCleanup chan struct{}
	cleanupOnce sync.Once
}

func NewAutoCleanGauge(name, help string, labelNames []string) *AutoCleanGauge {
	g := &AutoCleanGauge{
		desc:        prometheus.NewDesc(name, help, labelNames, nil),
		values:      make(map[string]float64),
		zeroTimes:   make(map[string]time.Time),
		mu:          &sync.RWMutex{},
		labelNames:  labelNames,
		stopCleanup: make(chan struct{}),
	}

	go g.cleanupRoutine()

	return g
}

func (g *AutoCleanGauge) WithLabelValues(labelValues ...string) *AutoCleanGauge {
	g.validateLabelValues(labelValues)

	return &AutoCleanGauge{
		desc:        g.desc,
		values:      g.values,
		zeroTimes:   g.zeroTimes,
		mu:          g.mu,
		labelNames:  g.labelNames,
		labelValues: labelValues,
		stopCleanup: g.stopCleanup,
	}
}

func (g *AutoCleanGauge) Set(value float64) {
	if len(g.labelValues) == 0 {
		panic("Set() called on unbound AutoCleanGauge")
	}

	key := g.labelKey(g.labelValues...)

	g.mu.Lock()
	defer g.mu.Unlock()

	if value == 0 {
		g.values[key] = value
		g.zeroTimes[key] = time.Now()
	} else {
		g.values[key] = value
		delete(g.zeroTimes, key)
	}
}

func (g *AutoCleanGauge) Inc() {
	if len(g.labelValues) == 0 {
		panic("Inc() called on unbound AutoCleanGauge")
	}

	key := g.labelKey(g.labelValues...)

	g.mu.Lock()
	defer g.mu.Unlock()

	newValue := g.values[key] + 1
	g.values[key] = newValue
	delete(g.zeroTimes, key)
}

func (g *AutoCleanGauge) Dec() {
	if len(g.labelValues) == 0 {
		panic("Dec() called on unbound AutoCleanGauge")
	}

	key := g.labelKey(g.labelValues...)

	g.mu.Lock()
	defer g.mu.Unlock()

	newValue := g.values[key] - 1
	if newValue <= 0 {
		g.values[key] = 0
		g.zeroTimes[key] = time.Now()
	} else {
		g.values[key] = newValue
		delete(g.zeroTimes, key)
	}
}

func (g *AutoCleanGauge) Add(delta float64) {
	if len(g.labelValues) == 0 {
		panic("Add() called on unbound AutoCleanGauge")
	}

	key := g.labelKey(g.labelValues...)

	g.mu.Lock()
	defer g.mu.Unlock()

	newValue := g.values[key] + delta
	if newValue <= 0 {
		g.values[key] = 0
		g.zeroTimes[key] = time.Now()
	} else {
		g.values[key] = newValue
		delete(g.zeroTimes, key)
	}
}

func (g *AutoCleanGauge) Describe(ch chan<- *prometheus.Desc) {
	ch <- g.desc
}

func (g *AutoCleanGauge) Collect(ch chan<- prometheus.Metric) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for key, value := range g.values {
		labelValues := strings.Split(key, "\x00")
		ch <- prometheus.MustNewConstMetric(g.desc, prometheus.GaugeValue, value, labelValues...)
	}
}

func (g *AutoCleanGauge) Stop() {
	g.cleanupOnce.Do(func() {
		close(g.stopCleanup)
	})
}

func (g *AutoCleanGauge) validateLabelValues(labelValues []string) {
	if len(labelValues) != len(g.labelNames) {
		panic(fmt.Sprintf("expected %d label values, got %d", len(g.labelNames), len(labelValues)))
	}
}

func (g *AutoCleanGauge) labelKey(labelValues ...string) string {
	return strings.Join(labelValues, "\x00")
}

func (g *AutoCleanGauge) cleanupRoutine() {
	tick := time.Tick(cleanupTime)

	for {
		select {
		case <-tick:
			g.performCleanup()
		case <-g.stopCleanup:
			return
		}
	}
}

func (g *AutoCleanGauge) performCleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for key, zeroTime := range g.zeroTimes {
		if now.Sub(zeroTime) >= graceTime {
			delete(g.values, key)
			delete(g.zeroTimes, key)
		}
	}
}
