package rating

import (
	"math"
	"testing"

	"github.com/osac-project/cost-event-consumer/internal/inventory"
)

func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func ptr(f float64) *float64 { return &f }

func TestApplyRate_Flat(t *testing.T) {
	rate := inventory.RateRecord{PricePerUnit: 0.01}

	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{"zero value", 0, 0},
		{"unit value", 1, 0.01},
		{"typical VM uptime (3600s)", 3600, 36.0},
		{"fractional", 0.5, 0.005},
		{"large value (1M tokens)", 1_000_000, 10_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyRate(tc.value, rate)
			if got != tc.want {
				t.Errorf("ApplyRate(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestApplyRate_Tiered(t *testing.T) {
	rate := inventory.RateRecord{
		Tiers: []inventory.Tier{
			{UpTo: ptr(100), PricePerUnit: 0.10},
			{UpTo: ptr(500), PricePerUnit: 0.05},
			{UpTo: nil, PricePerUnit: 0.01},
		},
	}

	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{"within first tier", 50, 5.0},
		{"exactly at first tier boundary", 100, 10.0},
		{"into second tier", 200, 10.0 + 5.0},
		{"exactly at second tier boundary", 500, 10.0 + 20.0},
		{"into unlimited tier", 1000, 10.0 + 20.0 + 5.0},
		{"zero", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyRate(tc.value, rate)
			if got != tc.want {
				t.Errorf("ApplyRate(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestApplyRate_SingleUnlimitedTier(t *testing.T) {
	rate := inventory.RateRecord{
		Tiers: []inventory.Tier{
			{UpTo: nil, PricePerUnit: 0.02},
		},
	}
	got := ApplyRate(500, rate)
	if got != 10.0 {
		t.Errorf("got %v, want 10.0", got)
	}
}

func TestApplyRate_FreeTierThenPaid(t *testing.T) {
	rate := inventory.RateRecord{
		Tiers: []inventory.Tier{
			{UpTo: ptr(20), PricePerUnit: 0},
			{UpTo: ptr(120), PricePerUnit: 0.08},
			{UpTo: nil, PricePerUnit: 0.07},
		},
	}

	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{"within free tier", 10, 0},
		{"exactly at free tier", 20, 0},
		{"into paid tier", 50, 0 + 30*0.08},
		{"into final tier", 200, 0 + 100*0.08 + 80*0.07},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyRate(tc.value, rate)
			if !approxEq(got, tc.want) {
				t.Errorf("ApplyRate(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestApplyRate_TieredOverridesFlat(t *testing.T) {
	rate := inventory.RateRecord{
		PricePerUnit: 999.0,
		Tiers: []inventory.Tier{
			{UpTo: nil, PricePerUnit: 0.01},
		},
	}
	got := ApplyRate(100, rate)
	if got != 1.0 {
		t.Errorf("tiers should override PricePerUnit; got %v, want 1.0", got)
	}
}

func TestApplyRate_EmptyTiers(t *testing.T) {
	rate := inventory.RateRecord{
		PricePerUnit: 0.05,
		Tiers:        []inventory.Tier{},
	}
	got := ApplyRate(100, rate)
	if got != 5.0 {
		t.Errorf("empty tiers should fall back to flat; got %v, want 5.0", got)
	}
}

var thresholds = []float64{50, 70, 90, 100}

func TestThresholdLevels(t *testing.T) {
	if len(thresholdLevels) != 4 {
		t.Fatalf("expected 4 threshold levels, got %d", len(thresholdLevels))
	}
	for i, want := range []float64{50, 70, 90, 100} {
		if thresholdLevels[i] != want {
			t.Errorf("thresholdLevels[%d] = %v, want %v", i, thresholdLevels[i], want)
		}
	}
}
