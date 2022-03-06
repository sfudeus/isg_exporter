package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"net/http/httptest"

	"github.com/prometheus/client_golang/prometheus"
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

	res = normalizeValue("18.321kWh")
	if res.Unit != "kWh" {
		t.Errorf("Expected kWh as unit, but got %s", res.Unit)
	}
	if res.Value != 18.321 {
		t.Errorf("Expected 18.321, but got %f", res.Value)
	}

	res = normalizeValue("15,8°C")
	if res.Unit != "°C" {
		t.Errorf("Expected °C as unit, but got %s", res.Unit)
	}
	if res.Value != 15.8 {
		t.Errorf("Expected 15.8, but got %f", res.Value)
	}

	res = normalizeValue("-15,8°C")
	if res.Unit != "°C" {
		t.Errorf("Expected °C as unit, but got %s", res.Unit)
	}
	if res.Value != -15.8 {
		t.Errorf("Expected -15.8, but got %f", res.Value)
	}
	res = normalizeValue("1 l/min")
	res = normalizeValue("1 %")
	res = normalizeValue("1 m³/h")

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
	res = normalizeLabel("M*1E6")
	if res != "m1e6" {
		t.Errorf("Expected m1e6, but got %s", res)
	}
}

func TestPage(t *testing.T) {

	options.Mode = MODE_WEBSCRAPING
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		page := req.URL.Query().Get("s")

		if page == "1,0" {
			content, err := os.ReadFile("test_resources/sample_1_0.html")
			if err != nil {
				t.Errorf("Failed delivering sample for 1,0")
			}
			fmt.Fprint(w, string(content))
		} else if page == "1,1" {
			content, err := os.ReadFile("test_resources/sample_1_1.html")
			if err != nil {
				t.Errorf("Failed delivering sample for 1,1")
			}
			fmt.Fprint(w, string(content))
		} else if page == "4,7" {
			content, err := os.ReadFile("test_resources/sample_4_7.html")
			if err != nil {
				t.Errorf("Failed delivering sample for 4,7")
			}
			fmt.Fprint(w, string(content))
		} else {
			fmt.Fprint(w, "")
		}
	}))
	defer ts.Close()

	options.URL = ts.URL
	gaugesMap = make(map[string]*prometheus.GaugeVec)
	flagGaugesMap = make(map[string]prometheus.Gauge)
	valuesMap = make(map[string][]IsgValue)
	prepare()

	flagRemovalList := make(map[string]prometheus.Gauge)
	parsePage("1,1", flagRemovalList)
	parsePage("4,7", flagRemovalList)

	json, _ := json.Marshal(valuesMap)
	fmt.Println(string(json))
}
