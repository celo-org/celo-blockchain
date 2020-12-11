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
	sync.RWMutex
	m map[string]string
}

func (s *Stats) GetStatsKeys() *Strings {
	s.RLock()
	keys := make([]string, len(s.m))

	i := 0
	for k := range s.m {
		keys[i] = k
		i++
	}
	s.RUnlock()
	return &Strings{keys}
}

func (s *Stats) GetValue(key string) string {
	s.RLock()
	val := s.m[key]
	s.RUnlock()

	return val
}

func (stats *Stats) addInt(name string, number int64) {
	if number != 0 {
		stats.m[name] = FormatInt(number)
	}
}

func (stats *Stats) addFloat(name string, number float64) {
	if !almostEqual(number, 0) {
		stats.m[name] = FormatFloat(number)
	}
}

func FormatFloat(number float64) string {
	return strconv.FormatFloat(number, 'f', 6, 64)
}

func FormatInt(number int64) string {
	return strconv.FormatInt(number, 10)
}

func (stats *Stats) publishCounter(name string, metric metrics.Counter) {
	stats.addInt(name, metric.Count())
}

func (stats *Stats) publishGauge(name string, metric metrics.Gauge) {
	stats.addInt(name, metric.Value())
}

func (stats *Stats) publishGaugeFloat64(name string, metric metrics.GaugeFloat64) {
	stats.addFloat(name, metric.Value())
}

func (stats *Stats) publishHistogram(name string, metric metrics.Histogram) {
	h := metric.Snapshot()
	ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
	stats.addInt(name+".count", h.Count())
	stats.addFloat(name+".min", float64(h.Min()))
	stats.addFloat(name+".max", float64(h.Max()))
	stats.addFloat(name+".mean", h.Mean())
	stats.addFloat(name+".std-dev", h.StdDev())
	stats.addFloat(name+".50-percentile", ps[0])
	stats.addFloat(name+".75-percentile", ps[1])
	stats.addFloat(name+".95-percentile", ps[2])
	stats.addFloat(name+".99-percentile", ps[3])
	stats.addFloat(name+".999-percentile", ps[4])
}

func (stats *Stats) publishMeter(name string, metric metrics.Meter) {
	m := metric.Snapshot()
	stats.addInt(name+".count", m.Count())
	stats.addFloat(name+".one-minute", m.Rate1())
	stats.addFloat(name+".five-minute", m.Rate5())
	stats.addFloat(name+".fifteen-minute", m.Rate15())
	stats.addFloat(name+".mean", m.RateMean())
}

func (stats *Stats) publishTimer(name string, metric metrics.Timer) {
	t := metric.Snapshot()
	ps := t.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
	stats.addInt(name+".count", t.Count())
	stats.addFloat(name+".min", float64(t.Min()))
	stats.addFloat(name+".max", float64(t.Max()))
	stats.addFloat(name+".mean", t.Mean())
	stats.addFloat(name+".std-dev", t.StdDev())
	stats.addFloat(name+".50-percentile", ps[0])
	stats.addFloat(name+".75-percentile", ps[1])
	stats.addFloat(name+".95-percentile", ps[2])
	stats.addFloat(name+".99-percentile", ps[3])
	stats.addFloat(name+".999-percentile", ps[4])
	stats.addFloat(name+".one-minute", t.Rate1())
	stats.addFloat(name+".five-minute", t.Rate5())
	stats.addFloat(name+".fifteen-minute", t.Rate15())
	stats.addFloat(name+".mean-rate", t.RateMean())
}

func (stats *Stats) publishResettingTimer(name string, metric metrics.ResettingTimer) {
	t := metric.Snapshot()
	ps := t.Percentiles([]float64{50, 75, 95, 99})
	stats.addInt(name+".count", int64(len(t.Values())))
	stats.addFloat(name+".mean", t.Mean())
	stats.addInt(name+".50-percentile", ps[0])
	stats.addInt(name+".75-percentile", ps[1])
	stats.addInt(name+".95-percentile", ps[2])
	stats.addInt(name+".99-percentile", ps[3])
}

func (stats *Stats) syncToRegistryStats() {
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
