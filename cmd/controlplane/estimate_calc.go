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

	model, err := resolvePricingModel(req.PricingProfile)
	if err != nil {
		return EstimateResponse{}, err
	}

	effectivePrice, err := model.ResolvePrice(req.PricePerGBUSD)
	if err != nil {
		return EstimateResponse{}, err
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

	originalCost := model.CalculateCost(originalGB, effectivePrice)
	filteredCost := model.CalculateCost(filteredGB, effectivePrice)
	savings := originalCost - filteredCost

	var result EstimateResponse
	result.RunID = report.RunID
	result.Currency = "USD"

	result.Assumptions.PricePerGBUSD = effectivePrice
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
	result.PricingProfile = model.ID()
	result.PricingUnit = model.PricingUnit()
	result.PricingSourceURL = model.SourceURL()
	result.PricingNote = model.Note()
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
