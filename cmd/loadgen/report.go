package main

import (
	"math/rand"
)

// Report holds the consolidated load test results.
type Report struct {
	Profile       string    `json:"profile"`
	DurationSec   float64   `json:"duration_sec"`
	TotalMessages int       `json:"total_messages"`
	AvgRTTMs      float64   `json:"avg_rtt_ms"`
	MinRTTMs      int64     `json:"min_rtt_ms"`
	MaxRTTMs      int64     `json:"max_rtt_ms"`
	P50RTTMs      int64     `json:"p50_rtt_ms"`
	P90RTTMs      int64     `json:"p90_rtt_ms"`
	PacketLossPct float64   `json:"packet_loss_pct"`
	Status        string    `json:"status"`
	Errors        []string  `json:"errors,omitempty"`
	ErrorRatePct  float64   `json:"error_rate_pct,omitempty"`
	AllRTTMs      []float64 `json:"-"` // raw measurements, not exported
}

// computeStats calculates all statistics from the raw RTT measurements.
// Must be called after AllRTTMs is populated.
func (r *Report) computeStats() {
	n := len(r.AllRTTMs)
	if n == 0 {
		return
	}

	// Sort a copy for percentile calculation.
	sorted := make([]float64, n)
	copy(sorted, r.AllRTTMs)
	quickSort(sorted)

	var sum float64
	r.MinRTTMs = int64(sorted[0])
	r.MaxRTTMs = int64(sorted[n-1])
	for _, v := range sorted {
		sum += v
	}
	r.AvgRTTMs = sum / float64(n)
	r.P50RTTMs = int64(percentile(sorted, 50))
	r.P90RTTMs = int64(percentile(sorted, 90))
}

// percentile returns the p-th percentile value from a sorted slice using the
// nearest-rank method: idx = floor(p/100 * (n-1)). This matches the expected
// values in the spec test dataset where [85,95,100,115,130,200,450] yields
// p50=115 (idx 3) and p90=200 (idx 5).
func percentile(sorted []float64, p int) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(float64(p) / 100.0 * float64(n-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// quickSort sorts a float64 slice in-place using a random-pivot quicksort.
func quickSort(a []float64) {
	if len(a) < 2 {
		return
	}
	pivotIdx := rand.Intn(len(a))
	a[0], a[pivotIdx] = a[pivotIdx], a[0]
	pivot := a[0]
	i := 1
	j := len(a) - 1
	for i <= j {
		for i <= j && a[i] <= pivot {
			i++
		}
		for i <= j && a[j] > pivot {
			j--
		}
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	a[0], a[j] = a[j], a[0]
	quickSort(a[:j])
	quickSort(a[j+1:])
}

// computeStatus determines the overall test status based on error rate and latency.
// Must be called after computeStats and ErrorRatePct are set.
//
// Status rules:
//   - "ok":       error_rate <= 5% AND latency_p90 <= 1500ms
//   - "degraded": error_rate 5-15% OR latency_p90 1500-2500ms
//   - "failed":   error_rate > 15% OR latency_p90 > 2500ms OR no measurements
func (r *Report) computeStatus() {
	// Fail if no measurements and errors exist.
	if r.TotalMessages == 0 && len(r.Errors) > 0 {
		r.Status = "failed"
		return
	}

	// Fail if 100% packet loss (connection failed).
	if r.TotalMessages == 0 {
		r.Status = "failed"
		return
	}

	// Determine status from error rate and latency.
	if r.ErrorRatePct > 15 || r.P90RTTMs > 2500 {
		r.Status = "failed"
	} else if r.ErrorRatePct > 5 || r.P90RTTMs > 1500 {
		r.Status = "degraded"
	} else {
		r.Status = "ok"
	}
}
