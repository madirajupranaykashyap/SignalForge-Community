package main

import (
	"fmt"
	"math"
)

type IndexingEstimate struct {
	PricingUnit string `json:"pricing_unit"`

	Assumptions IndexingAssumptions `json:"assumptions"`
	Monthly     IndexingMonthly     `json:"monthly"`

	Disclaimer string `json:"disclaimer"`
}

type IndexingAssumptions struct {
	PricePerMillionEventsUSD float64 `json:"price_per_million_events_usd"`
	BenchmarkBatchesPerDay   int64   `json:"benchmark_batches_per_day"`
	DaysPerMonth             int64   `json:"days_per_month"`
	EventsPerBenchmarkBatch  int64   `json:"events_per_benchmark_batch"`
}

type IndexingMonthly struct {
	OriginalIndexedEvents  int64   `json:"original_indexed_events"`
	FilteredIndexedEvents  int64   `json:"filtered_indexed_events"`
	EstimatedEventsAvoided int64   `json:"estimated_events_avoided"`
	OriginalCostUSD        float64 `json:"original_cost_usd"`
	FilteredCostUSD        float64 `json:"filtered_cost_usd"`
	EstimatedSavingsUSD    float64 `json:"estimated_savings_usd"`
	SavingsPercent         float64 `json:"savings_percent"`
}

func calculateIndexingEstimate(
	report BenchmarkReport,
	req EstimateRequest,
) (*IndexingEstimate, error) {
	price := req.PricePerMillionEventsUSD

	if math.IsNaN(price) || math.IsInf(price, 0) ||
		price <= 0 || price > 1_000_000 {
		return nil, fmt.Errorf(
			"price_per_million_events_usd must be greater than 0 and at most 1000000",
		)
	}

	submitted := report.Results.LogEventsSubmitted
	forwarded := report.Results.LogEventsSentToDebugExporter

	// Older reports may not contain event-count fields.
	if submitted <= 0 {
		return nil, fmt.Errorf(
			"benchmark report has no valid log event counts; rerun the benchmark",
		)
	}

	if forwarded < 0 || forwarded > submitted {
		return nil, fmt.Errorf(
			"benchmark report contains invalid forwarded event counts",
		)
	}

	originalMonthly, err := multiplyEventCounts(
		submitted,
		req.BatchesPerDay,
		req.DaysPerMonth,
	)
	if err != nil {
		return nil, err
	}

	filteredMonthly, err := multiplyEventCounts(
		forwarded,
		req.BatchesPerDay,
		req.DaysPerMonth,
	)
	if err != nil {
		return nil, err
	}

	avoided := originalMonthly - filteredMonthly

	originalCost := float64(originalMonthly) / 1_000_000 * price
	filteredCost := float64(filteredMonthly) / 1_000_000 * price
	savings := originalCost - filteredCost

	var savingsPercent float64
	if originalCost > 0 {
		savingsPercent = savings / originalCost * 100
	}

	return &IndexingEstimate{
		PricingUnit: "USD per million indexed log events",
		Assumptions: IndexingAssumptions{
			PricePerMillionEventsUSD: price,
			BenchmarkBatchesPerDay:   req.BatchesPerDay,
			DaysPerMonth:             req.DaysPerMonth,
			EventsPerBenchmarkBatch:  submitted,
		},
		Monthly: IndexingMonthly{
			OriginalIndexedEvents:  originalMonthly,
			FilteredIndexedEvents:  filteredMonthly,
			EstimatedEventsAvoided: avoided,
			OriginalCostUSD:        roundIndexingMoney(originalCost),
			FilteredCostUSD:        roundIndexingMoney(filteredCost),
			EstimatedSavingsUSD:    roundIndexingMoney(savings),
			SavingsPercent:         roundIndexingMoney(savingsPercent),
		},
		Disclaimer: "Illustrative event-based indexing estimate. Assumes every event forwarded to the vendor is indexed and billed at the supplied rate. Actual indexing eligibility, billing, exclusions, retention and contract pricing may differ.",
	}, nil
}

func multiplyEventCounts(
	eventsPerBatch int64,
	batchesPerDay int64,
	daysPerMonth int64,
) (int64, error) {
	if eventsPerBatch < 0 || batchesPerDay < 1 || daysPerMonth < 1 {
		return 0, fmt.Errorf("event counts and estimate periods must be valid")
	}

	maxInt64 := int64(math.MaxInt64)

	if eventsPerBatch > maxInt64/batchesPerDay {
		return 0, fmt.Errorf("monthly event count overflows int64")
	}

	dailyEvents := eventsPerBatch * batchesPerDay

	if dailyEvents > maxInt64/daysPerMonth {
		return 0, fmt.Errorf("monthly event count overflows int64")
	}

	return dailyEvents * daysPerMonth, nil
}

func roundIndexingMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
