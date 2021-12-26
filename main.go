package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/headzoo/surf/browser"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// IsgValue is a wrapper for a single data value with its unit
type IsgValue struct {
	Value float64
	Unit  string
}

var options struct {
	Port            int64  `long:"port" default:"8080" description:"The address to listen on for HTTP requests." env:"EXPORTER_PORT"`
	Interval        int64  `long:"interval" default:"60" env:"INTERVAL" description:"The frequency in seconds in which to gather data"`
	URL             string `long:"url" env:"ISG_URL" description:"URL for ISG"`
	User            string `long:"user" env:"ISG_USER" description:"username for ISG"`
	Password        string `long:"password" env:"ISG_PASSWORD" description:"password for ISG"`
	BrowserRollover int64  `long:"browserRollover" default:"60" description:"number of iterations until the internal browser is recreated"`
	SkipCircuit2    bool   `long:"skipCircuit2" description:"Toogle to skip data for circuit 2" env:"SKIP_CIRCUIT_2"`
	UseModbus       bool   `long:"modbus" description:"Use modbus communication, web scraping otherwise"`
	Debug           bool   `long:"debug"`
	// TODO: SkipCooling  bool   `long:"skipCooling" description:"Toggle to skip data for cooling" env:"SKIP_COOLING"`
}

var (
	valuesMap map[string]IsgValue
)

var (
	bow                 *browser.Browser
	browserUsageCounter int64
)

var (
	loginDuration = promauto.NewSummary(prometheus.SummaryOpts{
		Namespace: "isg",
		Name:      "loginduration",
		Help:      "The duration of login",
	})
	gatheringDuration = promauto.NewSummary(prometheus.SummaryOpts{
		Namespace: "isg",
		Name:      "gatheringduration",
		Help:      "The duration of data gatherings",
	})
	statusDuration = promauto.NewSummary(prometheus.SummaryOpts{
		Namespace: "isg",
		Name:      "statusduration",
		Help:      "The duration is status requests",
	})

	// map of all gauges (normal and flags)
	gaugesMap map[string]prometheus.Gauge

	// map of only the flag gauges
	flagGaugesMap map[string]prometheus.Gauge
)

func main() {
	_, err := flags.Parse(&options)
	if err != nil {
		os.Exit(1)
	}

	validate()

	gaugesMap = make(map[string]prometheus.Gauge)
	flagGaugesMap = make(map[string]prometheus.Gauge)
	valuesMap = make(map[string]IsgValue)

	prepare()

	go func() {
		for {
			gatherData()
			time.Sleep(time.Duration(options.Interval) * time.Second)
			if options.Debug {
				log.Println(valuesMap)
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/status", getData)
	if options.Debug {
		http.HandleFunc("/debug/pprof", pprof.Index)
	}
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", options.Port), nil))

}

func validate() {
	if options.URL == "" {
		log.Fatalln("Missing URL")
	}
	if !options.UseModbus {
		// No credentials for modbus
		if options.User == "" {
			log.Fatalln("Missing username")
		}
		if options.Password == "" {
			log.Fatalln("Missing password")
		}
	}
}

func prepare() {

	if options.UseModbus {
		prepareModbus()
	} else {
		prepareScraping()
	}

}

func gatherData() {
	timer := prometheus.NewTimer(gatheringDuration)
	defer timer.ObserveDuration()

	flagRemovalList := make(map[string]prometheus.Gauge)
	for key, gauge := range flagGaugesMap {
		flagRemovalList[key] = gauge
	}

	if options.UseModbus {
		gatherModbusData(flagRemovalList)
	} else {
		gatherScrapingData(flagRemovalList)
	}

	for _, gauge := range flagRemovalList {
		gauge.Set(0)
	}
}

func createOrRetrieve(label string, unit string) prometheus.Gauge {
	val, exists := gaugesMap[label]
	if !exists {
		help := ""
		if len(unit) > 0 {
			help = "Metric " + label + " in " + unit
		} else {
			help = "Metric " + label
		}
		val = promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "isg",
			Name:      label,
			Help:      help,
		})
		gaugesMap[label] = val
	}
	return val
}

func getData(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(statusDuration)
	defer timer.ObserveDuration()
	json, _ := json.Marshal(valuesMap)
	w.Header().Add("Content-Type", "application/json")
	w.Write(json)
}
