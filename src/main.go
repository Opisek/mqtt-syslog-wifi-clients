package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	godotenv "github.com/joho/godotenv"
)

type State struct {
	mac       string
	connected bool
	ssid      string
	station   string
	ap        string
	radio     int
}

type Vars struct {
	host  string
	port  int
	user  string
	pass  string
	topic string
}

type MqttDevice struct {
	deviceId               string
	deviceName             string
	propertyId             func(string) string
	propertyName           func(string) string
	baseDeviceTopic        string
	propertyStateTopic     func(string) string
	propertyDiscoveryTopic func(string) string
	state                  State
}

func parseEnv() Vars {
	envErr := godotenv.Load(".env")
	if envErr != nil {
		log.Fatalf(`Could not loed .env: "%s"`, envErr)
	}

	host := os.Getenv("MQTT_HOST")
	if host == "" {
		panic("Malformed .env: MQTT_HOST missing")
	}
	port := os.Getenv("MQTT_PORT")
	if port == "" {
		panic("Malformed .env: MQTT_PORT missing")
	}
	portNumber, portError := strconv.Atoi(port)
	if portError != nil {
		panic("Malformed .env: MQTT_PORT must be numeric")
	}
	user := os.Getenv("MQTT_USER")
	if user == "" {
		panic("Malformed .env: MQTT_USER missing")
	}
	pass := os.Getenv("MQTT_PASS")
	if pass == "" {
		panic("Malformed .env: MQTT_PASS missing")
	}
	topic := os.Getenv("MQTT_TOPIC")
	if topic == "" {
		panic("Malformed .env: MQTT_TOPIC missing")
	}

	return Vars{host, portNumber, user, pass, topic}
}

func parseSyslog(args []string) State {
	if len(args) != 2 {
		panic(`Malformed command line arguments: Usage: "<filename> <syslog message>"`)
	}

	syslog := args[1]

	connectedRegex := regexp.MustCompile("connected|disassociated")
	connectedMatch := connectedRegex.FindString(syslog)
	if connectedMatch == "" {
		panic(`Malformed syslog: Must either include "connected" or "disassociated"`)
	}
	connected := connectedMatch == "connected"

	macsRegex := regexp.MustCompile("(([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2}))")
	macsMatch := macsRegex.FindAllString(syslog, 2)
	if len(macsMatch) != 2 {
		panic("Malformed syslog: Must include station and client MAC addresses")
	}
	stationMac := macsMatch[0]
	clientMac := macsMatch[1]

	ssidRegex := regexp.MustCompile("VSS:(\\w+)")
	ssidMatch := ssidRegex.FindStringSubmatch(syslog)
	if ssidMatch == nil {
		panic(`Malformed syslog: Must include the SSID e.g., "VSS:OpiNet"`)
	}
	ssid := ssidMatch[1]

	apRegex := regexp.MustCompile("WTP:(\\w+)")
	apMatch := apRegex.FindStringSubmatch(syslog)
	if apMatch == nil {
		panic(`Malformed syslog: Must include the AP name e.g., "WTP:OpiAP"`)
	}
	ap := ssidMatch[1]

	radioRegex := regexp.MustCompile("Radio(\\d+)")
	radioMatch := radioRegex.FindStringSubmatch(syslog)
	if radioMatch == nil {
		panic(`Malformed syslog: Must include the radio number e.g. "Radio1"`)
	}
	radio, _ := strconv.Atoi(radioMatch[1])

	return State{clientMac, connected, ssid, stationMac, ap, radio}
}

func connectMqtt(vars Vars) mqtt.Client {
	options := mqtt.NewClientOptions()
	options.AddBroker(fmt.Sprintf("mqtt://%s:%d", vars.host, vars.port))
	options.SetClientID("wifi_presence")
	options.SetUsername(vars.user)
	options.SetPassword(vars.pass)

	client := mqtt.NewClient(options)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(fmt.Sprintf(`Could not connect to MQTT broker: "%s"`, token.Error()))
	}

	return client
}

func formatDevice(state State, vars Vars) MqttDevice {
	deviceId := fmt.Sprintf("wifi_client_%s", strings.ReplaceAll(state.mac, ":", "-"))
	deviceName := fmt.Sprintf("Wifi Client %s", state.mac)

	propertyId := func(property string) string {
		return fmt.Sprintf("%s_%s", deviceId, property)
	}
	propertyName := func(property string) string {
		r := []rune(property)
		r[0] = unicode.ToUpper(r[0])
		capitalizedProperty := string(r)
		return fmt.Sprintf("%s %s", deviceName, capitalizedProperty)
	}

	baseDeviceTopic := fmt.Sprintf("%s/%s", vars.topic, state.mac)
	baseDiscoveryTopic := fmt.Sprintf("homeassistant/sensor/%s", deviceId)

	propertyStateTopic := func(property string) string {
		return fmt.Sprintf("%s/%s", baseDeviceTopic, property)
	}
	propertyDiscoveryTopic := func(property string) string {
		return fmt.Sprintf("%s/%s/config", baseDiscoveryTopic, property)
	}

	return MqttDevice{deviceId, deviceName, propertyId, propertyName, baseDeviceTopic, propertyStateTopic, propertyDiscoveryTopic, state}
}

func publish(client mqtt.Client, topic string, payload interface{}) {
	token := client.Publish(topic, 1, true, payload)
	token.Wait()
}

func publishState(client mqtt.Client, device MqttDevice) {
	publish(client, device.propertyStateTopic("connected"), strconv.FormatBool(device.state.connected))
	publish(client, device.propertyStateTopic("ssid"), device.state.ssid)
	publish(client, device.propertyStateTopic("station"), device.state.station)
	publish(client, device.propertyStateTopic("ap"), device.state.ap)
	publish(client, device.propertyStateTopic("radio"), strconv.Itoa(device.state.radio))
}

func publishDiscoveryProperty(client mqtt.Client, device MqttDevice, property string) {
	topic := device.propertyDiscoveryTopic(property)

	discoveryData := map[string]interface{}{
		"unique_id": device.propertyId(property),
		"name":      device.propertyName(property),
		"device": map[string]interface{}{
			"identifiers": []string{device.deviceId},
			"name":        device.deviceName,
		},
		"state_topic": device.propertyStateTopic(property),
	}

	if property != "connected" {
		discoveryData["availability"] = map[string]interface{}{
			"topic":                 device.propertyStateTopic("connected"),
			"payload_available":     true,
			"payload_not_available": false,
		}
	}

	jsonData, jsonErr := json.Marshal(discoveryData)
	if jsonErr != nil {
		panic(fmt.Sprintf(`Error occured during payload marshalling: "%s"`, jsonErr))
	}

	publish(client, topic, string(jsonData))
}

func publishDiscovery(client mqtt.Client, device MqttDevice) {
	publishDiscoveryProperty(client, device, "connected")
	publishDiscoveryProperty(client, device, "ssid")
	publishDiscoveryProperty(client, device, "station")
	publishDiscoveryProperty(client, device, "ap")
	publishDiscoveryProperty(client, device, "radio")
}

func main() {
	vars := parseEnv()
	state := parseSyslog(os.Args)
	client := connectMqtt(vars)
	device := formatDevice(state, vars)

	publishState(client, device)
	publishDiscovery(client, device)

	client.Disconnect(0)
}
