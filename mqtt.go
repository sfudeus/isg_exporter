package main

import (
	json "encoding/json"
	"fmt"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

var mqttClient mqtt.Client

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Info("MQTT connected")
}

var connectionLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Warnf("MQTT connection lost: %v", err)
}

func connectMqtt() {
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker(fmt.Sprintf("tcp://%s:%d", options.MqttHost, options.MqttPort))
	clientOptions.SetOnConnectHandler(connectHandler)
	clientOptions.SetConnectionLostHandler(connectionLostHandler)
	mqttClient = mqtt.NewClient(clientOptions)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Errorf("Connect to MQTT failed: %s", token.Error())
	}
}

func publishMqtt() {
	for name, value := range valuesMap {
		topic := options.MqttTopicPrefix + "/" + strings.Replace(strings.ToLower(name), ".", "/", -1)
		content, _ := json.Marshal(value)

		log.Debugf("publishing %s to %s", content, topic)
		mqttClient.Publish(topic, 0, false, content)
	}
}
