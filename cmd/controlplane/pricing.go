package main

import (
	"errors"
	"fmt"
	"math"
)

const datadogLogIngestionPriceUSD = 0.10

const datadogLogPricingURL = "https://www.datadoghq.com/pricing/?product=log-management"

type PricingModel interface {
	ID() string
	PricingUnit() string
	ResolvePrice(requestedPrice float64) (float64, error)
	CalculateCost(volumeGB, pricePerGBUSD float64) float64
	SourceURL() string
	Note() string
}

type LinearGBPricing struct{}

func (LinearGBPricing) ID() string {
	return "linear-gb"
}

func (LinearGBPricing) PricingUnit() string {
	return "decimal GB of estimated payload"
}

func (LinearGBPricing) ResolvePrice(price float64) (float64, error) {
	if math.IsNaN(price) || math.IsInf(price, 0) ||
		price <= 0 || price > 1_000_000 {
		return 0, errors.New(
			"price_per_gb_usd must be greater than 0 and at most 1000000",
		)
	}

	return price, nil
}

func (LinearGBPricing) CalculateCost(
	volumeGB, pricePerGBUSD float64,
) float64 {
	return volumeGB * pricePerGBUSD
}

func (LinearGBPricing) SourceURL() string {
	return ""
}

func (LinearGBPricing) Note() string {
	return "Uses the custom price supplied in the request."
}

// DatadogLogIngestionPricing estimates the ingestion meter only.
// It does not include indexing, retention, or other Datadog products.
type DatadogLogIngestionPricing struct{}

func (DatadogLogIngestionPricing) ID() string {
	return "datadog-logs-ingestion"
}

func (DatadogLogIngestionPricing) PricingUnit() string {
	return "decimal GB of uncompressed ingested logs"
}

func (DatadogLogIngestionPricing) ResolvePrice(
	requestedPrice float64,
) (float64, error) {
	if math.IsNaN(requestedPrice) || math.IsInf(requestedPrice, 0) {
		return 0, errors.New("price_per_gb_usd must be a finite number")
	}

	if requestedPrice != 0 &&
		requestedPrice != datadogLogIngestionPriceUSD {
		return 0, fmt.Errorf(
			"this profile uses the published USD %.2f/GB list rate; omit price_per_gb_usd or supply %.2f",
			datadogLogIngestionPriceUSD,
			datadogLogIngestionPriceUSD,
		)
	}

	return datadogLogIngestionPriceUSD, nil
}

func (DatadogLogIngestionPricing) CalculateCost(
	volumeGB, pricePerGBUSD float64,
) float64 {
	return volumeGB * pricePerGBUSD
}

func (DatadogLogIngestionPricing) SourceURL() string {
	return datadogLogPricingURL
}

func (DatadogLogIngestionPricing) Note() string {
	return "Datadog log ingestion list-price estimate only. Excludes indexing, retention, negotiated discounts, taxes, and other charges. Actual billable volume may differ from the estimated OTLP JSON payload."
}

var pricingModels = map[string]PricingModel{
	"linear-gb":              LinearGBPricing{},
	"datadog-logs-ingestion": DatadogLogIngestionPricing{},
}

func resolvePricingModel(id string) (PricingModel, error) {
	if id == "" {
		id = "linear-gb"
	}

	model, ok := pricingModels[id]
	if !ok {
		return nil, fmt.Errorf("unsupported pricing_profile %q", id)
	}

	return model, nil
}
