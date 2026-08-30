package metrics

import (
	"maps"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestNewAutoCleanGauge(t *testing.T) {
	t.Run("creates gauge with correct properties", func(t *testing.T) {
		name := "test_metric"
		help := "Test metric description"
		labelNames := []string{"label1", "label2"}

		gauge := NewAutoCleanGauge(name, help, labelNames)
		defer gauge.Stop()

		assert.NotNil(t, gauge)
		assert.NotNil(t, gauge.desc)
		assert.NotNil(t, gauge.values)
		assert.NotNil(t, gauge.zeroTimes)
		assert.NotNil(t, gauge.mu)
		assert.Equal(t, labelNames, gauge.labelNames)
		assert.Empty(t, gauge.labelValues)
		assert.Empty(t, gauge.values)
		assert.NotNil(t, gauge.stopCleanup)
	})
}

func TestAutoCleanGauge_WithLabelValues(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1", "label2"})
	defer gauge.Stop()

	t.Run("creates bound gauge with correct label values", func(t *testing.T) {
		boundGauge := gauge.WithLabelValues("value1", "value2")

		assert.NotNil(t, boundGauge)
		assert.Equal(t, []string{"value1", "value2"}, boundGauge.labelValues)
		assert.Same(t, gauge.desc, boundGauge.desc)
		assert.Equal(t, gauge.values, boundGauge.values)
		assert.Equal(t, gauge.zeroTimes, boundGauge.zeroTimes)
		assert.Same(t, gauge.mu, boundGauge.mu)
		assert.Equal(t, gauge.labelNames, boundGauge.labelNames)
	})

	t.Run("panics with wrong number of label values", func(t *testing.T) {
		assert.Panics(t, func() {
			gauge.WithLabelValues("value1")
		})

		assert.Panics(t, func() {
			gauge.WithLabelValues("value1", "value2", "value3")
		})
	})
}

func TestAutoCleanGauge_Set(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	t.Run("sets non-zero value", func(t *testing.T) {
		boundGauge.Set(42.5)

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 42.5, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("sets zero value and tracks time", func(t *testing.T) {
		before := time.Now()
		boundGauge.Set(0.0)
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("panics when called on unbound gauge", func(t *testing.T) {
		assert.PanicsWithValue(t, "Set() called on unbound AutoCleanGauge", func() {
			gauge.Set(42.0)
		})
	})
}

func TestAutoCleanGauge_Inc(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	t.Run("increments from zero", func(t *testing.T) {
		boundGauge.Inc()

		gauge.mu.RLock()
		value := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.Equal(t, 1.0, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("increments existing value", func(t *testing.T) {
		boundGauge.Set(5.0)
		boundGauge.Inc()

		gauge.mu.RLock()
		value := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.Equal(t, 6.0, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("panics when called on unbound gauge", func(t *testing.T) {
		assert.PanicsWithValue(t, "Inc() called on unbound AutoCleanGauge", func() {
			gauge.Inc()
		})
	})
}

func TestAutoCleanGauge_Dec(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	t.Run("decrements existing value", func(t *testing.T) {
		boundGauge.Set(5.0)
		boundGauge.Dec()

		gauge.mu.RLock()
		value := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.Equal(t, 4.0, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("sets zero and tracks time when decremented to zero", func(t *testing.T) {
		boundGauge.Set(1.0)
		before := time.Now()
		boundGauge.Dec()
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("sets zero and tracks time when decremented below zero", func(t *testing.T) {
		boundGauge.Set(0.5)
		before := time.Now()
		boundGauge.Dec()
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("decrements from zero creates zero value and tracks time", func(t *testing.T) {
		before := time.Now()
		boundGauge.Dec()
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("panics when called on unbound gauge", func(t *testing.T) {
		assert.PanicsWithValue(t, "Dec() called on unbound AutoCleanGauge", func() {
			gauge.Dec()
		})
	})
}

func TestAutoCleanGauge_Add(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	t.Run("adds positive delta", func(t *testing.T) {
		boundGauge.Set(5.0)
		boundGauge.Add(2.5)

		gauge.mu.RLock()
		value := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.Equal(t, 7.5, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("adds negative delta", func(t *testing.T) {
		boundGauge.Set(5.0)
		boundGauge.Add(-2.0)

		gauge.mu.RLock()
		value := gauge.values["value1"]
		_, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.Equal(t, 3.0, value)
		assert.False(t, zeroTimeExists)
	})

	t.Run("sets zero and tracks time when result is zero", func(t *testing.T) {
		boundGauge.Set(5.0)
		before := time.Now()
		boundGauge.Add(-5.0)
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("sets zero and tracks time when result is negative", func(t *testing.T) {
		boundGauge.Set(3.0)
		before := time.Now()
		boundGauge.Add(-5.0)
		after := time.Now()

		gauge.mu.RLock()
		value, exists := gauge.values["value1"]
		zeroTime, zeroTimeExists := gauge.zeroTimes["value1"]
		gauge.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, 0.0, value)
		assert.True(t, zeroTimeExists)
		assert.True(t, zeroTime.After(before) || zeroTime.Equal(before))
		assert.True(t, zeroTime.Before(after) || zeroTime.Equal(after))
	})

	t.Run("panics when called on unbound gauge", func(t *testing.T) {
		assert.PanicsWithValue(t, "Add() called on unbound AutoCleanGauge", func() {
			gauge.Add(1.0)
		})
	})
}

func TestAutoCleanGauge_Describe(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()

	ch := make(chan *prometheus.Desc, 1)
	gauge.Describe(ch)
	close(ch)

	desc := <-ch
	assert.Same(t, gauge.desc, desc)
}

func TestAutoCleanGauge_Collect(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1", "label2"})
	defer gauge.Stop()

	boundGauge1 := gauge.WithLabelValues("val1", "val2")
	boundGauge2 := gauge.WithLabelValues("val3", "val4")

	boundGauge1.Set(10.0)
	boundGauge2.Set(20.0)

	ch := make(chan prometheus.Metric, 10)
	gauge.Collect(ch)
	close(ch)

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}

	assert.Len(t, metrics, 2)

	for _, metric := range metrics {
		assert.NotNil(t, metric)
	}
}

func TestAutoCleanGauge_LabelKey(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1", "label2"})
	defer gauge.Stop()

	tests := []struct {
		name        string
		labelValues []string
		expected    string
	}{
		{
			name:        "single label",
			labelValues: []string{"value1"},
			expected:    "value1",
		},
		{
			name:        "multiple labels",
			labelValues: []string{"value1", "value2"},
			expected:    "value1\x00value2",
		},
		{
			name:        "empty values",
			labelValues: []string{"", ""},
			expected:    "\x00",
		},
		{
			name:        "values with pipe",
			labelValues: []string{"val|ue1", "value2"},
			expected:    "val|ue1\x00value2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gauge.labelKey(tt.labelValues...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAutoCleanGauge_ConcurrentAccess(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	done := make(chan bool, 100)

	for range 10 {
		go func() {
			defer func() { done <- true }()
			for j := range 10 {
				boundGauge.Inc()
				boundGauge.Dec()
				boundGauge.Set(float64(j))
				boundGauge.Add(0.5)
			}
		}()
	}

	for range 10 {
		<-done
	}

	boundGauge.Set(42.0)

	gauge.mu.RLock()
	value := gauge.values["value1"]
	gauge.mu.RUnlock()

	assert.Equal(t, 42.0, value)
}

func TestAutoCleanGauge_MultipleLabels(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"service", "method", "status"})
	defer gauge.Stop()

	gauge1 := gauge.WithLabelValues("api", "get", "200")
	gauge2 := gauge.WithLabelValues("api", "post", "201")
	gauge3 := gauge.WithLabelValues("db", "query", "success")

	gauge1.Set(10.0)
	gauge2.Set(5.0)
	gauge3.Set(15.0)

	gauge.mu.RLock()
	values := maps.Clone(gauge.values)
	gauge.mu.RUnlock()

	expected := map[string]float64{
		"api\x00get\x00200":      10.0,
		"api\x00post\x00201":     5.0,
		"db\x00query\x00success": 15.0,
	}

	assert.Equal(t, expected, values)

	gauge2.Set(0.0)

	gauge.mu.RLock()
	values = maps.Clone(gauge.values)
	zeroTimes := maps.Clone(gauge.zeroTimes)
	gauge.mu.RUnlock()

	expectedAfterZero := map[string]float64{
		"api\x00get\x00200":      10.0,
		"api\x00post\x00201":     0.0,
		"db\x00query\x00success": 15.0,
	}

	assert.Equal(t, expectedAfterZero, values)
	assert.Contains(t, zeroTimes, "api\x00post\x00201")
}

func TestAutoCleanGauge_PerformCleanup(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	boundGauge.Set(0.0)

	gauge.mu.Lock()
	gauge.zeroTimes["value1"] = time.Now().Add(-2 * time.Hour)
	gauge.mu.Unlock()

	gauge.performCleanup()

	gauge.mu.RLock()
	_, exists := gauge.values["value1"]
	_, zeroTimeExists := gauge.zeroTimes["value1"]
	gauge.mu.RUnlock()

	assert.False(t, exists)
	assert.False(t, zeroTimeExists)
}

func TestAutoCleanGauge_PerformCleanupKeepsRecent(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})
	defer gauge.Stop()
	boundGauge := gauge.WithLabelValues("value1")

	boundGauge.Set(0.0)

	gauge.performCleanup()

	gauge.mu.RLock()
	value, exists := gauge.values["value1"]
	_, zeroTimeExists := gauge.zeroTimes["value1"]
	gauge.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, 0.0, value)
	assert.True(t, zeroTimeExists)
}

func TestAutoCleanGauge_Stop(t *testing.T) {
	gauge := NewAutoCleanGauge("test", "help", []string{"label1"})

	gauge.Stop()
	gauge.Stop()

	select {
	case <-gauge.stopCleanup:
	default:
		t.Error("stopCleanup channel should be closed")
	}
}
