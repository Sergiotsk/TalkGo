package main

import (
	"math"
	"testing"
)

func TestLoadgen_ReportComputation(t *testing.T) {
	rtts := []float64{85, 95, 100, 115, 130, 200, 450}
	report := &Report{
		AllRTTMs: rtts,
	}
	report.computeStats()

	// Expected values from TASK-047 spec:
	//   avg ≈ 167.857, min=85, max=450, p50=115 (idx 3), p90=200 (idx 5)
	expectedAvg := (85 + 95 + 100 + 115 + 130 + 200 + 450) / 7.0
	expectedMin := int64(85)
	expectedMax := int64(450)
	expectedP50 := int64(115)
	expectedP90 := int64(200)

	if math.Abs(report.AvgRTTMs-expectedAvg) > 0.1 {
		t.Errorf("avg_rtt_ms = %.1f, want %.1f", report.AvgRTTMs, expectedAvg)
	}
	if report.MinRTTMs != expectedMin {
		t.Errorf("min_rtt_ms = %d, want %d", report.MinRTTMs, expectedMin)
	}
	if report.MaxRTTMs != expectedMax {
		t.Errorf("max_rtt_ms = %d, want %d", report.MaxRTTMs, expectedMax)
	}
	if report.P50RTTMs != expectedP50 {
		t.Errorf("p50_rtt_ms = %d, want %d", report.P50RTTMs, expectedP50)
	}
	if report.P90RTTMs != expectedP90 {
		t.Errorf("p90_rtt_ms = %d, want %d", report.P90RTTMs, expectedP90)
	}
}

func TestReport_StatusLogic_OK(t *testing.T) {
	report := &Report{
		TotalMessages: 100,
		ErrorRatePct:  2.0,
		P90RTTMs:      800,
	}
	report.computeStatus()
	if report.Status != "ok" {
		t.Errorf("status = %q, want %q", report.Status, "ok")
	}
}

func TestReport_StatusLogic_Degraded_ErrorRate(t *testing.T) {
	report := &Report{
		TotalMessages: 100,
		ErrorRatePct:  10.0,
		P90RTTMs:      800,
	}
	report.computeStatus()
	if report.Status != "degraded" {
		t.Errorf("status = %q, want %q", report.Status, "degraded")
	}
}

func TestReport_StatusLogic_Degraded_Latency(t *testing.T) {
	report := &Report{
		TotalMessages: 100,
		ErrorRatePct:  2.0,
		P90RTTMs:      1800,
	}
	report.computeStatus()
	if report.Status != "degraded" {
		t.Errorf("status = %q, want %q", report.Status, "degraded")
	}
}

func TestReport_StatusLogic_Failed_ErrorRate(t *testing.T) {
	report := &Report{
		TotalMessages: 100,
		ErrorRatePct:  20.0,
		P90RTTMs:      800,
	}
	report.computeStatus()
	if report.Status != "failed" {
		t.Errorf("status = %q, want %q", report.Status, "failed")
	}
}

func TestReport_StatusLogic_Failed_Latency(t *testing.T) {
	report := &Report{
		TotalMessages: 100,
		ErrorRatePct:  2.0,
		P90RTTMs:      3000,
	}
	report.computeStatus()
	if report.Status != "failed" {
		t.Errorf("status = %q, want %q", report.Status, "failed")
	}
}

func TestReport_StatusLogic_Failed_NoMeasurements(t *testing.T) {
	report := &Report{
		TotalMessages: 0,
		Errors:        []string{"connection refused"},
	}
	report.computeStatus()
	if report.Status != "failed" {
		t.Errorf("status = %q, want %q", report.Status, "failed")
	}
}

func TestPercentile_Empty(t *testing.T) {
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("percentile(nil, 50) = %f, want 0", got)
	}
}

func TestPercentile_SingleElement(t *testing.T) {
	if got := percentile([]float64{42}, 50); got != 42 {
		t.Errorf("percentile([42], 50) = %f, want 42", got)
	}
}
