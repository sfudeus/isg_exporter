package main

import (
	"regexp"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/PuerkitoBio/goquery"
	"github.com/headzoo/surf"
	"github.com/headzoo/surf/browser"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// map of only the flag gauges
	flagGaugesMap map[string]prometheus.Gauge
)

var (
	bow                 *browser.Browser
	browserUsageCounter int64

	loginDuration = promauto.NewSummary(prometheus.SummaryOpts{
		Namespace: "isg",
		Name:      "loginduration",
		Help:      "The duration of login",
	})
)

func prepareScraping() {

	flagGaugesMap = make(map[string]prometheus.Gauge)

	timer := prometheus.NewTimer(loginDuration)
	defer timer.ObserveDuration()

	log.Info("Performing Login for ISG")

	bow = surf.NewBrowser()
	err := bow.Open(options.URL + "?s=1,0")
	if err != nil {
		log.Fatal(err)
	}
	browserUsageCounter = 1

	fm, err := bow.Form("form#werte")
	if err != nil {
		log.Fatal(err)
	}
	fm.Input("user", options.User)
	fm.Input("pass", options.Password)
	err = fm.Submit()
	if err != nil {
		log.Fatal(err)
	}
}

func gatherScrapingData() {

	if browserUsageCounter > options.BrowserRollover {
		log.Info("Redo prepare because of browser rollover")
		prepare()
	}

	flagRemovalList := make(map[string]prometheus.Gauge)
	for key, gauge := range flagGaugesMap {
		flagRemovalList[key] = gauge
	}

	parsePage("1,0", flagRemovalList) // Info->System
	parsePage("1,1", flagRemovalList) // Info->HeatPump
	parsePage("2,0", flagRemovalList) // Diagnosis->Status
	parsePage("2,1", flagRemovalList) // Diagnosis->Commissioning
	parsePage("2,3", flagRemovalList) // Diagnosis->Contractor
	parsePage("2,4", flagRemovalList) // Diagnosis->ISG-Debug
	parsePage("4,7", flagRemovalList) // Settings->EM-DEBUG-INFOS

	for _, gauge := range flagRemovalList {
		gauge.Set(0)
	}
}

func parsePage(page string, flagRemovalList map[string]prometheus.Gauge) {

	err := bow.Open(options.URL + "?s=" + page)
	browserUsageCounter++
	if err != nil {
		log.Info("Redo prepare because of error: " + err.Error())
		prepareScraping()
	}

	bow.Find("form#werte table.info").Each(func(_ int, table *goquery.Selection) {
		table.Find("tr.even,tr.odd").Each(func(_ int, s *goquery.Selection) {
			key := s.Find("td.key").Text()
			value := strings.TrimSpace(s.Find("td.value").Text())

			header := table.Find("th.round-top").Text()
			label := normalizeLabel(header + "_" + key)

			if strings.Contains(label, "hk2") && options.SkipCircuit2 {
				return
				/* TODO
				} else if string.index(label, kuehlen) > -1 && options.SkipCooling {
					return
				*/
			}

			if value != "" {
				isgValue := normalizeValue(value)
				valuesMap[label] = append(valuesMap[label], isgValue)
				createOrRetrieve(label, isgValue.Unit, nil).Set(isgValue.Value)
			} else {
				label = "flag_" + label
				flagGauge := createOrRetrieve(label, "", nil)
				flagGauge.Set(1)
				flagGaugesMap[label] = flagGauge
				delete(flagRemovalList, label)
			}
		})
	})
}
func normalizeLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case (r == ' ' || r == '-' || r == '/'):
			// canonical separator "_"
			return '_'
		case r == '.' || r == '(' || r == ')' || r == '*':
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
	// in some cases like "FIXED VALUE OPERATION" instead of boolean value its used "On" and "Off" to prevent exceptions the switch is used
	switch s {
	case "Aus", "Off", "Apagado", "Uit", "Spento", "Av", "Wyłączony", "Vyp", "Kikapcsolva", "Apagat", "Pois":
		s = "0"
	case "Ein", "On", "Allumé", "Aan", "Acceso", "På", "Włączony", "Zap", "Bekapcsolva", "Encendido", "Päällä", "Tændt":
		s = "1"
	}
	re := regexp.MustCompile(`(?P<value>[0-9,.-]+)( ?)(?P<unit>[a-zA-Z°%/²³.]*)`)
	matches := re.FindStringSubmatch(s)
	// Fix to prevent exception if s can not be converted into float
	if len(matches) == 0 {
		return IsgValue{Value: 0, Unit: "Error: is no float"}
	}
	// ISG exports numbers with decimal separator ",", even with language setting english
	// needs to be converted to be parsed as float
	value := strings.Replace(matches[re.SubexpIndex("value")], ",", ".", -1)
	unit := ""
	float, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Fatalf("Failed to parse value %s", value)
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

