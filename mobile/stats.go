// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package geth

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/metrics"
)

const float64EqualityThreshold = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= float64EqualityThreshold
}

type Stats struct {
	mutex sync.RWMutex
	m     map[string]string
}

func NewStats() *Stats {
	return &Stats{m: make(map[string]string)}
}

func (s *Stats) GetStatsKeys() *Strings {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	keys := make([]string, len(s.m))

	i := 0
	for k := range s.m {
		keys[i] = k
		i++
	}
	return &Strings{keys}
}

func (s *Stats) GetValue(key string) string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.m[key]
}

func (stats *Stats) SetInt(name string, number int64) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	stats.setUnlockedInt(name, number)
}

func (stats *Stats) SetUInt(name string, number uint64) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	stats.setUnlockedUInt(name, number)
}

func (stats *Stats) SetFloat(name string, number float64) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	stats.setUnlockedFloat(name, number)
}

func (stats *Stats) SetBool(name string, boolean bool) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	stats.setUnlockedBool(name, boolean)
}

// This function assumes that was performed a previous write lock
func (stats *Stats) setUnlockedInt(name string, number int64) {
	if number != 0 {
		stats.m[name] = formatInt(number)
	} else {
		delete(stats.m, name)
	}
}

// This function assumes that was performed a previous write lock
func (stats *Stats) setUnlockedUInt(name string, number uint64) {
	if number != 0 {
		stats.m[name] = formatUInt(number)
	} else {
		delete(stats.m, name)
	}
}

// This function assumes that was performed a previous write lock
func (stats *Stats) setUnlockedFloat(name string, number float64) {
	if !almostEqual(number, 0) {
		stats.m[name] = formatFloat(number)
	} else {
		delete(stats.m, name)
	}
}

// This function assumes that was performed a previous write lock
func (stats *Stats) setUnlockedBool(name string, boolean bool) {
	stats.m[name] = strconv.FormatBool(boolean)
}

func formatFloat(number float64) string {
	return strconv.FormatFloat(number, 'f', 6, 64)
}

func formatInt(number int64) string {
	return strconv.FormatInt(number, 10)
}

func formatUInt(number uint64) string {
	return strconv.FormatUint(number, 10)
}

func (stats *Stats) publishCounter(name string, metric metrics.Counter) {
	stats.setUnlockedInt(name, metric.Count())
}

func (stats *Stats) publishGauge(name string, metric metrics.Gauge) {
	stats.setUnlockedInt(name, metric.Value())
}

func (stats *Stats) publishGaugeFloat64(name string, metric metrics.GaugeFloat64) {
	stats.setUnlockedFloat(name, metric.Value())
}

func (stats *Stats) publishHistogram(name string, metric metrics.Histogram) {
	h := metric.Snapshot()
	ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
	stats.setUnlockedInt(name+".count", h.Count())
	stats.setUnlockedFloat(name+".min", float64(h.Min()))
	stats.setUnlockedFloat(name+".max", float64(h.Max()))
	stats.setUnlockedFloat(name+".mean", h.Mean())
	stats.setUnlockedFloat(name+".std-dev", h.StdDev())
	stats.setUnlockedFloat(name+".50-percentile", ps[0])
	stats.setUnlockedFloat(name+".75-percentile", ps[1])
	stats.setUnlockedFloat(name+".95-percentile", ps[2])
	stats.setUnlockedFloat(name+".99-percentile", ps[3])
	stats.setUnlockedFloat(name+".999-percentile", ps[4])
}

func (stats *Stats) publishMeter(name string, metric metrics.Meter) {
	m := metric.Snapshot()
	stats.setUnlockedInt(name+".count", m.Count())
	stats.setUnlockedFloat(name+".one-minute", m.Rate1())
	stats.setUnlockedFloat(name+".five-minute", m.Rate5())
	stats.setUnlockedFloat(name+".fifteen-minute", m.Rate15())
	stats.setUnlockedFloat(name+".mean", m.RateMean())
}

func (stats *Stats) publishTimer(name string, metric metrics.Timer) {
	t := metric.Snapshot()
	ps := t.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
	stats.setUnlockedInt(name+".count", t.Count())
	stats.setUnlockedFloat(name+".min", float64(t.Min()))
	stats.setUnlockedFloat(name+".max", float64(t.Max()))
	stats.setUnlockedFloat(name+".mean", t.Mean())
	stats.setUnlockedFloat(name+".std-dev", t.StdDev())
	stats.setUnlockedFloat(name+".50-percentile", ps[0])
	stats.setUnlockedFloat(name+".75-percentile", ps[1])
	stats.setUnlockedFloat(name+".95-percentile", ps[2])
	stats.setUnlockedFloat(name+".99-percentile", ps[3])
	stats.setUnlockedFloat(name+".999-percentile", ps[4])
	stats.setUnlockedFloat(name+".one-minute", t.Rate1())
	stats.setUnlockedFloat(name+".five-minute", t.Rate5())
	stats.setUnlockedFloat(name+".fifteen-minute", t.Rate15())
	stats.setUnlockedFloat(name+".mean-rate", t.RateMean())
}

func (stats *Stats) publishResettingTimer(name string, metric metrics.ResettingTimer) {
	t := metric.Snapshot()
	ps := t.Percentiles([]float64{50, 75, 95, 99})
	stats.setUnlockedInt(name+".count", int64(len(t.Values())))
	stats.setUnlockedFloat(name+".mean", t.Mean())
	stats.setUnlockedInt(name+".50-percentile", ps[0])
	stats.setUnlockedInt(name+".75-percentile", ps[1])
	stats.setUnlockedInt(name+".95-percentile", ps[2])
	stats.setUnlockedInt(name+".99-percentile", ps[3])
}

func (stats *Stats) SyncToRegistryStats() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	metrics.DefaultRegistry.Each(func(name string, i interface{}) {
		switch i := i.(type) {
		case metrics.Counter:
			stats.publishCounter(name, i)
		case metrics.Gauge:
			stats.publishGauge(name, i)
		case metrics.GaugeFloat64:
			stats.publishGaugeFloat64(name, i)
		case metrics.Histogram:
			stats.publishHistogram(name, i)
		case metrics.Meter:
			stats.publishMeter(name, i)
		case metrics.Timer:
			stats.publishTimer(name, i)
		case metrics.ResettingTimer:
			stats.publishResettingTimer(name, i)
		default:
			panic(fmt.Sprintf("unsupported type for '%s': %T", name, i))
		}
	})
}
