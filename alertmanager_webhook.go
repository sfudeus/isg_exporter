package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

type KV map[string]string
type Alerts []Alert

type Alert struct {
	Status       string    `json:"status"`
	Labels       KV        `json:"labels"`
	Annotations  KV        `json:"annotations"`
	StartAt      time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	GeneratorUrl string    `json:"generatorURL"`
	Fingerprint  string    `json:"fingerprint"`
}

type Webhook struct {
	Version           string `json:"version"`
	GroupKey          string `json:"groupKey"`
	TruncatedAlerts   uint64 `json:"truncatedAlerts"`
	Status            string `json:"status"`
	Receiver          string `json:"receiver"`
	GroupLabels       KV     `json:"groupLabels"`
	CommonLabels      KV     `json:"commonLabels"`
	CommonAnnotations KV     `json:"commonAnnotations"`
	ExternalUrl       string `json:"externalURL"`
	Alerts            Alerts `json:"alerts"`
}

const (
	STATUS_RESOLVED string = "resolved"
	STATUS_FIRING   string = "firing"
	LABEL_ACTIVE    string = "active"
	LABEL_INACTIVE  string = "inactive"
)

var (
	actions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "sgready_action_total",
		Namespace: "isg",
		Help:      "Amount of successful invocations per action-level",
	}, []string{"action"})
	error = promauto.NewCounter(prometheus.CounterOpts{
		Name:      "sgready_error_total",
		Namespace: "isg",
		Help:      "Amount of failed invocations",
	})
)

func callAlertmanagerWebhook(c *gin.Context) {
	var webhook Webhook

	log.Info("Webhook called")
	if err := c.ShouldBindJSON(&webhook); err != nil {
		error.Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Info(webhook)
	for _, alert := range webhook.Alerts {
		switch alert.Status {
		case STATUS_RESOLVED:
			log.Info("Triggering sgReady 2")
			SetSGReadyLevel(Normal)
			actions.WithLabelValues(Normal).Inc()
		case STATUS_FIRING:
			switch alert.Labels["target"] {
			case LABEL_ACTIVE:
				log.Info("Triggering sgReady 3")
				SetSGReadyLevel(Active)
				actions.WithLabelValues(Active).Inc()
			case LABEL_INACTIVE:
				log.Info("Triggering sgReady 1")
				SetSGReadyLevel(Lock)
				actions.WithLabelValues(Lock).Inc()
			default:
				log.Warnf("Received unexpected target label %s", alert.Labels["target"])
				actions.WithLabelValues("unknown").Inc()
			}
		default:
			log.Warnf("Received unexpected status %s", webhook.Status)
			actions.WithLabelValues("unknown").Inc()
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "webhook received"})
}
