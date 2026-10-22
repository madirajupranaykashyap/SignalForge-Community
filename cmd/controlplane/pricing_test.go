package main

import "testing"

func TestResolvePricingModelDefaultsToLinearGB(t *testing.T) {
	model, err := resolvePricingModel("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if model.ID() != "linear-gb" {
		t.Fatalf("model ID = %q, want %q", model.ID(), "linear-gb")
	}
}

func TestResolvePricingModelRejectsUnknownProfile(t *testing.T) {
	_, err := resolvePricingModel("unknown-vendor")
	if err == nil {
		t.Fatal("expected an error for an unsupported profile")
	}
}

func TestLinearGBPricing(t *testing.T) {
	model := LinearGBPricing{}

	got := model.CalculateCost(2.5, 0.10)
	want := 0.25

	if got != want {
		t.Errorf("cost = %v, want %v", got, want)
	}
}

func TestCalculateEstimateUsesSelectedProfile(t *testing.T) {
	report := sampleReport(15346, 8656)

	req := EstimateRequest{
		RunID:          "test-run",
		PricingProfile: "linear-gb",
		PricePerGBUSD:  0.10,
		BatchesPerDay:  1000,
		DaysPerMonth:   30,
	}

	got, err := calculateEstimate(report, req)
	if err != nil {
		t.Fatalf("calculateEstimate returned error: %v", err)
	}

	if got.PricingProfile != "linear-gb" {
		t.Errorf("profile = %q, want %q", got.PricingProfile, "linear-gb")
	}

	if got.Monthly.SavingsPercent != 43.59 {
		t.Errorf("savings percentage = %v, want 43.59",
			got.Monthly.SavingsPercent)
	}
}

func TestDatadogProfileUsesPublishedDefaultRate(t *testing.T) {
	model := DatadogLogIngestionPricing{}

	price, err := model.ResolvePrice(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if price != 0.10 {
		t.Fatalf("price = %v, want 0.10", price)
	}
}

func TestDatadogProfileRejectsCustomRate(t *testing.T) {
	model := DatadogLogIngestionPricing{}

	_, err := model.ResolvePrice(0.25)
	if err == nil {
		t.Fatal("expected a custom rate to be rejected")
	}
}

func TestDatadogProfileCalculatesIngestionCost(t *testing.T) {
	model := DatadogLogIngestionPricing{}

	price, err := model.ResolvePrice(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := model.CalculateCost(100, price)
	want := 10.0

	if got != want {
		t.Errorf("cost = %v, want %v", got, want)
	}
}

func TestResolvePricingModelSupportsDatadog(t *testing.T) {
	model, err := resolvePricingModel("datadog-logs-ingestion")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if model.ID() != "datadog-logs-ingestion" {
		t.Errorf("model ID = %q", model.ID())
	}
}
