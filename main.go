package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/headzoo/surf"
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
	//TODO: SkipCooling  bool   `long:"skipCooling" description:"Toggle to skip data for cooling" env:"SKIP_COOLING"`
	Debug bool `long:"debug"`
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
	if "" == options.URL {
		log.Fatalln("Missing URL")
	}
	if "" == options.User {
		log.Fatalln("Missing username")
	}
	if "" == options.Password {
		log.Fatalln("Missing password")
	}
}

func prepare() {
	timer := prometheus.NewTimer(loginDuration)
	defer timer.ObserveDuration()

	log.Println("Performing Login for ISG")

	bow = surf.NewBrowser()
	err := bow.Open(options.URL + "?s=1,0")
	if err != nil {
		log.Panicln(err)
	}
	browserUsageCounter = 1

	fm, err := bow.Form("form#werte")
	if err != nil {
		log.Panicln(err)
	}
	fm.Input("user", options.User)
	fm.Input("pass", options.Password)
	err = fm.Submit()
	if err != nil {
		log.Panicln(err)
	}
}

func gatherData() {
	timer := prometheus.NewTimer(gatheringDuration)
	defer timer.ObserveDuration()

	flagRemovalList := make(map[string]prometheus.Gauge)
	for key, gauge := range flagGaugesMap {
		flagRemovalList[key] = gauge
	}

	if browserUsageCounter > options.BrowserRollover {
		log.Println("Redo prepare because of browser rollover")
		prepare()
	}

	err := bow.Open(options.URL + "?s=1,0")
	browserUsageCounter++
	if err != nil {
		log.Println("Redo prepare because of error: " + err.Error())
		prepare()
	}

	bow.Find("form#werte table.info tr.even,tr.odd").Each(func(_ int, s *goquery.Selection) {
		key := s.Find("td.key").Text()
		value := strings.TrimSpace(s.Find("td.value").Text())

		label := normalizeLabel(key)

		if strings.Index(label, "hk2") > -1 && options.SkipCircuit2 {
			return
			/* TODO
			} else if string.index(label, kuehlen) > -1 && options.SkipCooling {
				return
			*/
		}

		if value != "" {
			isgValue := normalizeValue(value)
			valuesMap[label] = isgValue
			createOrRetrieve(label, isgValue.Unit).Set(isgValue.Value)
		} else {
			label = "flag_" + label
			flagGauge := createOrRetrieve(label, "")
			flagGauge.Set(1)
			flagGaugesMap[label] = flagGauge
			delete(flagRemovalList, label)
		}
	})

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

func normalizeLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case (r == ' ' || r == '-'):
			// canonical separator "_"
			return '_'
		case r == '.' || r == '(' || r == ')':
			// ignore other special characters or abbreviation signals
			return -1
		}
		return r
	}, strings.TrimSpace(s))

	s = strings.ToLower(s)

	// need to convert umlaut for german output since they aren't valid prometheus metric names
	s = strings.Replace(s, "ü", "ue", -1)
	s = strings.Replace(s, "ä", "ae", -1)
	s = strings.Replace(s, "ö", "oe", -1)

	return s
}

func normalizeValue(s string) IsgValue {
	re := regexp.MustCompile(`(?P<value>[0-9,.]+)( ?)(?P<unit>[a-zA-Z]*)`)
	matches := re.FindStringSubmatch(s)
	// ISG exports numbers with decimal separator ",", even with language setting english
	// needs to be converted to be parsed as float
	value := strings.Replace(matches[re.SubexpIndex("value")], ",", ".", -1)
	unit := ""
	float, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Panicln("Failed to parse value " + value)
	}

	if len(matches) > 2 {
		unit = matches[re.SubexpIndex("unit")]
		if unit == "MWh" {
			float *= 1000
			unit = "kWh"
		}
	}

	return IsgValue{Value: float, Unit: unit}
}

func getData(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(statusDuration)
	defer timer.ObserveDuration()
	json, _ := json.Marshal(valuesMap)
	w.Header().Add("Content-Type", "application/json")
	w.Write(json)
}
