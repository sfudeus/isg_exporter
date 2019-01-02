package main

import (
	"testing"
)

func TestNormalizeValue(t *testing.T) {

	res := normalizeValue("10,123 kWh")
	if res.Unit != "kWh" {
		t.Errorf("Expected kWh as unit, but got %s", res.Unit)
	}
	if res.Value != 10.123 {
		t.Errorf("Expected 10.123 as value, but got %f", res.Value)
	}

	res = normalizeValue("10")
	if res.Unit != "" {
		t.Error("Expected empty unit")
	}
	if res.Value != 10 {
		t.Fail()
	}

	res = normalizeValue("3,345 MWh")
	if res.Unit != "kWh" {
		t.Errorf("Expected conversion to kWh, but got %s", res.Unit)
	}
	if res.Value != 3345 {
		t.Errorf("Expected 3345, but got %f", res.Value)
	}
}

func TestNormalizeLabel(t *testing.T) {

	res := normalizeLabel("RAUMISTTEMP. HK1")
	if res != "raumisttemp_hk1" {
		t.Fail()
	}

	res = normalizeLabel("FORTLUFT IST LÜFTERDREHZAHL  ")
	if res != "fortluft_ist_luefterdrehzahl" {
		t.Errorf("Expected fortluft_ist_luefterdrehzahl, but got %s", res)
	}
	res = normalizeLabel("WW-SOLLTEMP.")
	if res != "ww_solltemp" {
		t.Errorf("Expected ww_solltemp, but got %s", res)
	}
}
