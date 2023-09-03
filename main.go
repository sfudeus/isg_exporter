package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var options struct {
	Port            int64  `long:"port" default:"8080" description:"The address to listen on for HTTP requests." env:"EXPORTER_PORT"`
	Interval        int64  `long:"interval" default:"60" env:"INTERVAL" description:"The frequency in seconds in which to gather data"`
	URL             string `long:"url" env:"ISG_URL" description:"URL for ISG"`
	User            string `long:"user" env:"ISG_USER" description:"username for ISG"`
	Password        string `long:"password" env:"ISG_PASSWORD" description:"password for ISG"`
	BrowserRollover int64  `long:"browserRollover" default:"60" description:"number of iterations until the internal browser is recreated"`
	SkipCircuit2    bool   `long:"skipCircuit2" description:"Toogle to skip data for circuit 2" env:"SKIP_CIRCUIT_2"`
	Debug           bool   `long:"debug"`
	Loglevel        string `long:"loglevel" default:"warn" description:"logLevel (trace,debug,info,warn(ing),error,fatal,panic)"`
	Mode            string `long:"mode" default:"webscraping" description:"Gathering mode (webscraping|modbus)"`
	ModbusSlaveId   int64  `long:"modbusSlaveId" default:"1" description:"slaveId to use for modbus communication"`
	MqttHost        string `long:"mqttHost" description:"MQTT host to send data to (optional)"`
	MqttPort        int64  `long:"mqttPort" description:"MQTT port to send data to (optional)" default:"1883"`
	MqttTopicPrefix string `long:"mqttTopicPrefix" description:"Topic prefix for MQTT" default:"isg"`
	// TODO: SkipCooling  bool   `long:"skipCooling" description:"Toggle to skip data for cooling" env:"SKIP_COOLING"`
}

const (
	MODE_MODBUS      string = "modbus"
	MODE_WEBSCRAPING string = "webscraping"
)

var (
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
	gaugesMap map[string]*prometheus.GaugeVec
	valuesMap map[string][]IsgValue
)

// IsgValue is a wrapper for a single data value with its unit
type IsgValue struct {
	Value  float64
	Unit   string            `json:",omitempty"`
	Labels map[string]string `json:",omitempty"`
}

func main() {
	_, err := flags.Parse(&options)
	if err != nil {
		os.Exit(1)
	}

	if options.Debug {
		log.SetLevel(log.DebugLevel)
	}

	level, err := log.ParseLevel(options.Loglevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(level)

	validate()

	gaugesMap = make(map[string]*prometheus.GaugeVec)
	valuesMap = make(map[string][]IsgValue)

	prepare()

	go func() {
		for {
			gatherData()
			time.Sleep(time.Duration(options.Interval) * time.Second)
			log.Debug(valuesMap)
		}
	}()

	startWebserver(options.Port, options.Debug, options.Mode == MODE_MODBUS)
}

func startWebserver(port int64, debug bool, withModbus bool) {
	log.Info("Starting webserver")
	if debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()
	router.SetTrustedProxies(nil)

	if debug {
		pprof.Register(router)
	}

	router.GET("/ready", func(c *gin.Context) { c.String(http.StatusOK, "ready") })
	router.GET("/status", getStatusData)
	router.GET("/metrics", prometheusHandler())

	if withModbus {
		router.POST("/webhooks/alertmanager", callAlertmanagerWebhook)
	}
	router.Run(fmt.Sprintf(":%d", port))
}

func prometheusHandler() gin.HandlerFunc {
	h := promhttp.Handler()

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func validate() {
	if options.URL == "" {
		log.Fatal("Missing URL")
	}
	switch options.Mode {
	case MODE_MODBUS:
	case MODE_WEBSCRAPING:
		// Credentials only for webscraping
		if options.User == "" {
			log.Fatal("Missing username")
		}
		if options.Password == "" {
			log.Fatal("Missing password")
		}
	default:
		log.Fatalf("Unknown scraping mode %s", options.Mode)
	}
}

func prepare() {

	switch options.Mode {
	case MODE_MODBUS:
		prepareModbus()
	case MODE_WEBSCRAPING:
		prepareScraping()
	}
	if len(options.MqttHost) > 0 {
		connectMqtt()
	}
}

func gatherData() {
	timer := prometheus.NewTimer(gatheringDuration)
	defer timer.ObserveDuration()

	switch options.Mode {
	case MODE_MODBUS:
		gatherModbusData()
	case MODE_WEBSCRAPING:
		gatherScrapingData()
	}
	if len(options.MqttHost) > 0 {
		publishMqtt()
	}
}

func createOrRetrieve(name string, unit string, labels map[string]string) prometheus.Gauge {
	val, exists := gaugesMap[name]
	labelKeys := make([]string, 0, len(labels))
	labelValues := make([]string, 0, len(labels))

	if len(unit) > 0 {
		labelKeys = append(labelKeys, "unit")
		labelValues = append(labelValues, unit)
	}

	for k, v := range labels {
		labelKeys = append(labelKeys, k)
		labelValues = append(labelValues, v)
	}

	if !exists {
		help := ""
		if len(unit) > 0 {
			help = "Metric " + name + " in " + unit
		} else {
			help = "Metric " + name
		}

		val = promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "isg",
				Name:      strings.ToLower(name),
				Help:      help,
			},
			labelKeys)

		gaugesMap[name] = val
	}
	return val.WithLabelValues(labelValues...)
}

func getStatusData(c *gin.Context) {
	timer := prometheus.NewTimer(statusDuration)
	defer timer.ObserveDuration()
	c.JSON(http.StatusOK, valuesMap)
}
