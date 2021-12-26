package main

import (
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/headzoo/surf"
	"github.com/prometheus/client_golang/prometheus"
)

func prepareScraping() {

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

func gatherScrapingData(flagRemovalList map[string]prometheus.Gauge) {

	if browserUsageCounter > options.BrowserRollover {
		log.Println("Redo prepare because of browser rollover")
		prepare()
	}
	parsePage("1,0", flagRemovalList) // Info->System
	parsePage("1,1", flagRemovalList) // Info->HeatPump
	parsePage("2,0", flagRemovalList) // Diagnosis->Status
	parsePage("2,1", flagRemovalList) // Diagnosis->Commissioning
	parsePage("2,3", flagRemovalList) // Diagnosis->Contractor
	parsePage("2,4", flagRemovalList) // Diagnosis->ISG-Debug
	parsePage("4,7", flagRemovalList) // Settings->EM-DEBUG-INFOS

}

func parsePage(page string, flagRemovalList map[string]prometheus.Gauge) {

	err := bow.Open(options.URL + "?s=" + page)
	browserUsageCounter++
	if err != nil {
		log.Println("Redo prepare because of error: " + err.Error())
		prepareScraping()
	}

	bow.Find("form#werte table.info tr.even,tr.odd").Each(func(_ int, s *goquery.Selection) {
		key := s.Find("td.key").Text()
		value := strings.TrimSpace(s.Find("td.value").Text())

		label := normalizeLabel(key)

		if strings.Contains(label, "hk2") && options.SkipCircuit2 {
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
	re := regexp.MustCompile(`(?P<value>[0-9,.-]+)( ?)(?P<unit>[a-zA-Z°%/²³.]*)`)
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
