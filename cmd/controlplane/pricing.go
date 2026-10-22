package main

import "fmt"

// PricingModel calculates estimated cost for a telemetry volume.
// Vendor-specific models can implement their own billing rules later.
type PricingModel interface {
	ID() string
	CalculateCost(volumeGB, pricePerGBUSD float64) float64
}

// LinearGBPricing is the generic baseline model.
// It assumes every decimal GB is charged at the same rate.
type LinearGBPricing struct{}

func (LinearGBPricing) ID() string {
	return "linear-gb"
}

func (LinearGBPricing) CalculateCost(
	volumeGB float64,
	pricePerGBUSD float64,
) float64 {
	return volumeGB * pricePerGBUSD
}

var pricingModels = map[string]PricingModel{
	"linear-gb": LinearGBPricing{},
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
