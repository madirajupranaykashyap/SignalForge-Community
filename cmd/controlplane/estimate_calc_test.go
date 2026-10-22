package main

import (
	"math"
	"testing"
)

func sampleReport(original, filtered int64) BenchmarkReport {
	var report BenchmarkReport
	report.RunID = "test-run"
	report.SchemaVersion = "1.0"
	report.Results.OriginalOTLPJSONBytes = original
	report.Results.FilteredOTLPJSONEquivalentBytes = filtered
	return report
}

func TestCalculateEstimate(t *testing.T) {
	report := sampleReport(15346, 8656)

	req := EstimateRequest{
		RunID:         "test-run",
		PricePerGBUSD: 0.10,
		BatchesPerDay: 1000,
		DaysPerMonth:  30,
	}

	got, err := calculateEstimate(report, req)
	if err != nil {
		t.Fatalf("calculateEstimate returned error: %v", err)
	}

	if got.Monthly.OriginalEstimatedGB != 0.46 {
		t.Errorf("original GB = %v, want 0.46",
			got.Monthly.OriginalEstimatedGB)
	}

	if got.Monthly.FilteredEstimatedGB != 0.26 {
		t.Errorf("filtered GB = %v, want 0.26",
			got.Monthly.FilteredEstimatedGB)
	}

	if got.Monthly.EstimatedGBSaved != 0.20 {
		t.Errorf("saved GB = %v, want 0.20",
			got.Monthly.EstimatedGBSaved)
	}

	if got.Monthly.SavingsPercent != 43.59 {
		t.Errorf("savings percentage = %v, want 43.59",
			got.Monthly.SavingsPercent)
	}

	if got.Monthly.OriginalCostUSD != 0.05 {
		t.Errorf("original cost = %v, want 0.05",
			got.Monthly.OriginalCostUSD)
	}

	if got.Monthly.FilteredCostUSD != 0.03 {
		t.Errorf("filtered cost = %v, want 0.03",
			got.Monthly.FilteredCostUSD)
	}

	if got.Monthly.EstimatedSavingsUSD != 0.02 {
		t.Errorf("savings = %v, want 0.02",
			got.Monthly.EstimatedSavingsUSD)
	}
}

func TestCalculateEstimateRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		report BenchmarkReport
		req    EstimateRequest
	}{
		{
			name:   "zero price",
			report: sampleReport(1000, 500),
			req: EstimateRequest{
				PricePerGBUSD: 0,
				BatchesPerDay: 1,
				DaysPerMonth:  30,
			},
		},
		{
			name:   "NaN price",
			report: sampleReport(1000, 500),
			req: EstimateRequest{
				PricePerGBUSD: math.NaN(),
				BatchesPerDay: 1,
				DaysPerMonth:  30,
			},
		},
		{
			name:   "zero batches",
			report: sampleReport(1000, 500),
			req: EstimateRequest{
				PricePerGBUSD: 0.10,
				BatchesPerDay: 0,
				DaysPerMonth:  30,
			},
		},
		{
			name:   "too many days",
			report: sampleReport(1000, 500),
			req: EstimateRequest{
				PricePerGBUSD: 0.10,
				BatchesPerDay: 1,
				DaysPerMonth:  32,
			},
		},
		{
			name:   "zero original bytes",
			report: sampleReport(0, 0),
			req: EstimateRequest{
				PricePerGBUSD: 0.10,
				BatchesPerDay: 1,
				DaysPerMonth:  30,
			},
		},
		{
			name:   "filtered bytes exceed original",
			report: sampleReport(100, 101),
			req: EstimateRequest{
				PricePerGBUSD: 0.10,
				BatchesPerDay: 1,
				DaysPerMonth:  30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculateEstimate(tt.report, tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCalculateEstimateZeroSavings(t *testing.T) {
	report := sampleReport(1000, 1000)

	req := EstimateRequest{
		RunID:         "test-run",
		PricePerGBUSD: 0.10,
		BatchesPerDay: 100,
		DaysPerMonth:  30,
	}

	got, err := calculateEstimate(report, req)
	if err != nil {
		t.Fatalf("calculateEstimate returned error: %v", err)
	}

	if got.Monthly.EstimatedSavingsUSD != 0 {
		t.Errorf("savings = %v, want 0",
			got.Monthly.EstimatedSavingsUSD)
	}

	if got.Monthly.SavingsPercent != 0 {
		t.Errorf("savings percentage = %v, want 0",
			got.Monthly.SavingsPercent)
	}
}

func TestBenchmarkReportContainsLogEventCounts(t *testing.T) {
	report := sampleReport(15346, 8656)

	report.Results.LogEventsSubmitted = 100
	report.Results.LogEventsFiltered = 50
	report.Results.LogEventsSentToDebugExporter = 50
	report.Results.LogEventsArchived = 100

	if report.Results.LogEventsSubmitted != 100 {
		t.Fatal("expected 100 submitted log events")
	}

	if report.Results.LogEventsFiltered != 50 {
		t.Fatal("expected 50 filtered log events")
	}

	if report.Results.LogEventsSentToDebugExporter != 50 {
		t.Fatal("expected 50 output log events")
	}

	if report.Results.LogEventsArchived != 100 {
		t.Fatal("expected 100 archived log events")
	}
}
