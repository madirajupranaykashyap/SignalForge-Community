package main

import (
	"errors"
	"math"
)

const bytesPerGB int64 = 1_000_000_000

// calculateEstimate keeps the pricing calculation separate from HTTP handling.
// It currently supports linear pricing per decimal GB.
func calculateEstimate(
	report BenchmarkReport,
	req EstimateRequest,
) (EstimateResponse, error) {
	if req.PricePerGBUSD <= 0 ||
		math.IsNaN(req.PricePerGBUSD) ||
		math.IsInf(req.PricePerGBUSD, 0) ||
		req.PricePerGBUSD > 1_000_000 {
		return EstimateResponse{}, errors.New("price_per_gb_usd must be greater than 0 and at most 1000000")
	}

	if req.BatchesPerDay < 1 || req.BatchesPerDay > 1_000_000 {
		return EstimateResponse{}, errors.New("benchmark_batches_per_day must be between 1 and 1000000")
	}

	if req.DaysPerMonth < 1 || req.DaysPerMonth > 31 {
		return EstimateResponse{}, errors.New("days_per_month must be between 1 and 31")
	}

	originalBytes := report.Results.OriginalOTLPJSONBytes
	filteredBytes := report.Results.FilteredOTLPJSONEquivalentBytes

	if originalBytes <= 0 {
		return EstimateResponse{}, errors.New("original payload bytes must be greater than zero")
	}

	if filteredBytes < 0 || filteredBytes > originalBytes {
		return EstimateResponse{}, errors.New("filtered payload bytes must be between zero and original payload bytes")
	}

	originalMonthlyBytes :=
		float64(originalBytes) *
			float64(req.BatchesPerDay) *
			float64(req.DaysPerMonth)

	filteredMonthlyBytes :=
		float64(filteredBytes) *
			float64(req.BatchesPerDay) *
			float64(req.DaysPerMonth)

	originalGB := originalMonthlyBytes / float64(bytesPerGB)
	filteredGB := filteredMonthlyBytes / float64(bytesPerGB)
	savedGB := originalGB - filteredGB

	originalCost := linearGBPrice(originalGB, req.PricePerGBUSD)
	filteredCost := linearGBPrice(filteredGB, req.PricePerGBUSD)
	savings := originalCost - filteredCost

	var result EstimateResponse
	result.RunID = report.RunID
	result.Currency = "USD"
	result.PricingUnit = "decimal GB (1 GB = 1,000,000,000 bytes)"

	result.Assumptions.PricePerGBUSD = req.PricePerGBUSD
	result.Assumptions.BatchesPerDay = req.BatchesPerDay
	result.Assumptions.DaysPerMonth = req.DaysPerMonth
	result.Assumptions.BytesPerGB = bytesPerGB

	result.Monthly.OriginalEstimatedGB = roundTo2(originalGB)
	result.Monthly.FilteredEstimatedGB = roundTo2(filteredGB)
	result.Monthly.EstimatedGBSaved = roundTo2(savedGB)
	result.Monthly.OriginalCostUSD = roundTo2(originalCost)
	result.Monthly.FilteredCostUSD = roundTo2(filteredCost)
	result.Monthly.EstimatedSavingsUSD = roundTo2(savings)

	if originalCost > 0 {
		result.Monthly.SavingsPercent =
			roundTo2(savings / originalCost * 100)
	}

	result.Disclaimer = "Illustrative estimate based on equivalent OTLP JSON payload sizes. Not actual vendor wire bytes, verified billable ingestion, or a vendor quote. Assumes the configured price applies linearly to every decimal GB."

	return result, nil
}

// This is the initial pricing strategy. Vendor-specific pricing models
// can be implemented separately without changing the HTTP handler.
func linearGBPrice(volumeGB, pricePerGBUSD float64) float64 {
	return volumeGB * pricePerGBUSD
}

func roundTo2(value float64) float64 {
	return math.Round(value*100) / 100
}
