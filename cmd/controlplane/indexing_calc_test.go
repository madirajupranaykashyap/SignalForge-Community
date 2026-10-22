package main

import (
	"math"
	"testing"
)

func TestCalculateIndexingEstimate(t *testing.T) {
	report := BenchmarkReport{}
	report.Results.LogEventsSubmitted = 100
	report.Results.LogEventsSentToDebugExporter = 50

	req := EstimateRequest{
		PricePerMillionEventsUSD: 1.00,
		BatchesPerDay:            1000,
		DaysPerMonth:             30,
	}

	got, err := calculateIndexingEstimate(report, req)
	if err != nil {
		t.Fatalf("calculateIndexingEstimate() error = %v", err)
	}

	if got.Monthly.OriginalIndexedEvents != 3_000_000 {
		t.Errorf("original events = %d, want 3000000",
			got.Monthly.OriginalIndexedEvents)
	}
	if got.Monthly.FilteredIndexedEvents != 1_500_000 {
		t.Errorf("filtered events = %d, want 1500000",
			got.Monthly.FilteredIndexedEvents)
	}
	if got.Monthly.EstimatedEventsAvoided != 1_500_000 {
		t.Errorf("events avoided = %d, want 1500000",
			got.Monthly.EstimatedEventsAvoided)
	}
	if got.Monthly.OriginalCostUSD != 3.00 {
		t.Errorf("original cost = %v, want 3.00",
			got.Monthly.OriginalCostUSD)
	}
	if got.Monthly.FilteredCostUSD != 1.50 {
		t.Errorf("filtered cost = %v, want 1.50",
			got.Monthly.FilteredCostUSD)
	}
	if got.Monthly.SavingsPercent != 50 {
		t.Errorf("savings percent = %v, want 50",
			got.Monthly.SavingsPercent)
	}
}

func TestCalculateIndexingEstimateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		report BenchmarkReport
		req    EstimateRequest
	}{
		{
			name:   "missing submitted events",
			report: BenchmarkReport{},
			req: EstimateRequest{
				PricePerMillionEventsUSD: 1,
				BatchesPerDay:            1000,
				DaysPerMonth:             30,
			},
		},
		{
			name: "forwarded events exceed submitted events",
			report: func() BenchmarkReport {
				var r BenchmarkReport
				r.Results.LogEventsSubmitted = 100
				r.Results.LogEventsSentToDebugExporter = 101
				return r
			}(),
			req: EstimateRequest{
				PricePerMillionEventsUSD: 1,
				BatchesPerDay:            1000,
				DaysPerMonth:             30,
			},
		},
		{
			name: "negative price",
			report: func() BenchmarkReport {
				var r BenchmarkReport
				r.Results.LogEventsSubmitted = 100
				r.Results.LogEventsSentToDebugExporter = 50
				return r
			}(),
			req: EstimateRequest{
				PricePerMillionEventsUSD: -1,
				BatchesPerDay:            1000,
				DaysPerMonth:             30,
			},
		},
		{
			name: "zero batches per day",
			report: func() BenchmarkReport {
				var r BenchmarkReport
				r.Results.LogEventsSubmitted = 100
				r.Results.LogEventsSentToDebugExporter = 50
				return r
			}(),
			req: EstimateRequest{
				PricePerMillionEventsUSD: 1,
				BatchesPerDay:            0,
				DaysPerMonth:             30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculateIndexingEstimate(tt.report, tt.req)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestIndexingEstimateSavingsNeverNegative(t *testing.T) {
	var report BenchmarkReport
	report.Results.LogEventsSubmitted = 100
	report.Results.LogEventsSentToDebugExporter = 100

	req := EstimateRequest{
		PricePerMillionEventsUSD: 1,
		BatchesPerDay:            10,
		DaysPerMonth:             30,
	}

	got, err := calculateIndexingEstimate(report, req)
	if err != nil {
		t.Fatalf("calculateIndexingEstimate() error = %v", err)
	}

	if got.Monthly.EstimatedSavingsUSD != 0 {
		t.Errorf("savings = %v, want 0",
			got.Monthly.EstimatedSavingsUSD)
	}
	if math.Abs(got.Monthly.SavingsPercent) > 0.001 {
		t.Errorf("savings percent = %v, want 0",
			got.Monthly.SavingsPercent)
	}
}
