package main

import (
	"crypto/tls"
	json "encoding/json"
	"fmt"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

var mqttClient mqtt.Client
var discoveryNodeId string
var iteration int

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Info("MQTT connected")
}

var connectionLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Warnf("MQTT connection lost: %v", err)
}

func connectMqtt() {
	clientOptions := mqtt.NewClientOptions()
	var protocol string
	if options.MqttTls {
		protocol = "tls"
	} else {
		protocol = "tcp"
	}
	clientOptions.AddBroker(fmt.Sprintf("%s://%s:%d", protocol, options.MqttHost, options.MqttPort))
	if options.MqttTlsInsecure && options.MqttTls {
		clientOptions.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}
	if len(options.MqttUser) > 0 && len(options.MqttPassword) > 0 {
		clientOptions.SetUsername(options.MqttUser)
		clientOptions.SetPassword(options.MqttPassword)
	}
	clientOptions.SetOnConnectHandler(connectHandler)
	clientOptions.SetConnectionLostHandler(connectionLostHandler)
	mqttClient = mqtt.NewClient(clientOptions)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Errorf("Connect to MQTT failed: %s", token.Error())
	}
	initHomeAssistant()
}

func generateDiscoveryTopic(deviceType string, identifier string) string {
	return strings.Join([]string{options.MqttDiscoveryTopicPrefix, deviceType, discoveryNodeId, identifier, "config"}, "/")
}

func generateDevice() map[string]interface{} {
	return map[string]interface{}{
		"identifiers":  []string{discoveryNodeId},
		"manufacturer": "Stiebel Eltron",
		"name":         "ISG",
		"model":        "LWZ",
	}
}

func generateFanDiscovery(topicSuffix string, name string) {
	topic := options.MqttTopicPrefix + "/" + topicSuffix
	state := strings.Replace(strings.ToLower(name), " ", "_", -1)
	oid := discoveryNodeId + "_" + state
	fan := map[string]interface{}{
		"name":                      name,
		"state_topic":               topic,
		"state_value_template":      "{{ iif(int(value_json.Value) > 0, 'ON', 'OFF') }}",
		"command_topic":             topic + "/set",
		"command_template":          "{{ iif(value == 'ON', state_attr('fan." + oid + "', 'preset_mode'), '0') }}",
		"preset_mode_state_topic":   topic + "/preset_mode",
		"preset_mode_command_topic": topic + "/preset_mode",
		"preset_modes":              []string{"1", "2", "3"},
		"device":                    generateDevice(),
		"unique_id":                 oid,
		"object_id":                 oid,
		"enabled_by_default":        "true",
	}
	discoveryContent, _ := json.Marshal(fan)
	mqttClient.Publish(generateDiscoveryTopic("fan", state), 0, false, discoveryContent)

}

func initHomeAssistant() {
	discoveryNodeId = strings.Replace(strings.TrimPrefix(options.URL, "http://"), ".", "_", -1)

	generateFanDiscovery("stufe_nacht", "Ventilation Night")
	generateFanDiscovery("stufe_tag", "Ventilation Day")
	generateFanDiscovery("stufe_party", "Ventilation Party")
	generateFanDiscovery("stufe_hand", "Ventilation Manual")
}

func mapUnitToDeviceClass(unit string) interface{} {

	switch unit {
	case "°C":
		return "temperature"
	case "Hz":
		return "frequency"
	case "bar":
		return "pressure"
	case "%":
		return "humidity"
	case "kWh":
		return "energy"
	default:
		return nil
	}
}

func sendDiscoveryData(identifier string, stateTopic string, unit string) {

	if len(unit) == 0 {
		return
	}

	discoveryTopic := strings.Join([]string{options.MqttDiscoveryTopicPrefix, "sensor", discoveryNodeId, identifier, "config"}, "/")

	sensorConfigPayload := map[string]interface{}{
		"device_class":        mapUnitToDeviceClass(unit),
		"state_topic":         stateTopic,
		"unit_of_measurement": unit,
		"value_template":      "{{ value_json.Value }}",
		"name":                identifier,
		"unique_id":           discoveryNodeId + "_" + identifier,
		"object_id":           discoveryNodeId + "_" + identifier,
		"enabled_by_default":  "true",
		"device":              generateDevice(),
	}

	discoveryContent, _ := json.Marshal(sensorConfigPayload)
	mqttClient.Publish(discoveryTopic, 0, false, discoveryContent)
}

func publishMqtt() {
	withDiscoveryData := (iteration%10 == 0)
	log.Debugf("publishing in iteration %d, with discovery set to %t", iteration, withDiscoveryData)
	iteration++

	for name, value := range valuesMap {
		baseTopic := options.MqttTopicPrefix + "/" + strings.Replace(strings.ToLower(name), ".", "/", -1)
		var content []byte
		var topic string
		var identifier string

		if len(value) > 1 {
			for _, entry := range value {
				topic = baseTopic
				identifier = strings.ToLower(name)
				for label, label_value := range entry.Labels {
					topic = fmt.Sprintf("%s/%s_%s", topic, label, label_value)
					identifier = identifier + "_" + label + label_value
					content, _ = json.Marshal(entry)

					log.Debugf("publishing %s to %s", content, topic)
					mqttClient.Publish(topic, 0, false, content)

					if withDiscoveryData {
						sendDiscoveryData(identifier, topic, entry.Unit)
					}
				}
			}
		} else {
			topic = baseTopic
			entry := value[0]
			content, _ = json.Marshal(entry)
			log.Debugf("publishing %s to %s", content, topic)
			mqttClient.Publish(topic, 0, false, content)

			if withDiscoveryData {
				sendDiscoveryData(strings.ToLower(name), topic, entry.Unit)
			}
		}
	}
}
