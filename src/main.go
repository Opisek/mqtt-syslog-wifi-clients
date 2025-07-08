package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	executable, executableErr := os.Executable()
	if executableErr != nil {
		log.Fatalf(`Could not determine executable directory: "%s"\n`, executableErr)
	}
	executablePath := filepath.Dir(executable)
	envPath := filepath.Join(executablePath, ".env")

	envErr := godotenv.Load(envPath)
	if envErr != nil {
		log.Fatalf(`Could not load .env: "%s"\n`, envErr)
	}

	host := os.Getenv("MQTT_HOST")
	if host == "" {
		log.Fatalln("Malformed .env: MQTT_HOST missing")
	}
	port := os.Getenv("MQTT_PORT")
	if port == "" {
		log.Fatalln("Malformed .env: MQTT_PORT missing")
	}
	portNumber, portError := strconv.Atoi(port)
	if portError != nil {
		log.Fatalln("Malformed .env: MQTT_PORT must be numeric")
	}
	user := os.Getenv("MQTT_USER")
	if user == "" {
		log.Fatalln("Malformed .env: MQTT_USER missing")
	}
	pass := os.Getenv("MQTT_PASS")
	if pass == "" {
		log.Fatalln("Malformed .env: MQTT_PASS missing")
	}
	topic := os.Getenv("MQTT_TOPIC")
	if topic == "" {
		log.Fatalln("Malformed .env: MQTT_TOPIC missing")
	}

	return Vars{host, portNumber, user, pass, topic}
}

// Digitalisierungsbox Smart
//func parseSyslog(syslog string) State {
//	connectedRegex := regexp.MustCompile("connected|disassociated")
//	connectedMatch := connectedRegex.FindString(syslog)
//	if connectedMatch == "" {
//		log.Fatalln(`Malformed syslog: Must either include "connected" or "disassociated"`)
//	}
//	connected := connectedMatch == "connected"
//
//	macsRegex := regexp.MustCompile("(([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2}))")
//	macsMatch := macsRegex.FindAllString(syslog, 2)
//	if len(macsMatch) != 2 {
//		log.Fatalln("Malformed syslog: Must include station and client MAC addresses")
//	}
//	stationMac := strings.ToUpper(macsMatch[0])
//	clientMac := strings.ToUpper(macsMatch[1])
//
//	ssidRegex := regexp.MustCompile("VSS:(\\w+)")
//	ssidMatch := ssidRegex.FindStringSubmatch(syslog)
//	if ssidMatch == nil {
//		log.Fatalln(`Malformed syslog: Must include the SSID e.g., "VSS:OpiNet"`)
//	}
//	ssid := ssidMatch[1]
//
//	apRegex := regexp.MustCompile("WTP:(\\w+)")
//	apMatch := apRegex.FindStringSubmatch(syslog)
//	if apMatch == nil {
//		log.Fatalln(`Malformed syslog: Must include the AP name e.g., "WTP:OpiAP"`)
//	}
//	ap := apMatch[1]
//
//	radioRegex := regexp.MustCompile("Radio(\\d+)")
//	radioMatch := radioRegex.FindStringSubmatch(syslog)
//	if radioMatch == nil {
//		log.Fatalln(`Malformed syslog: Must include the radio number e.g. "Radio1"`)
//	}
//	radio, _ := strconv.Atoi(radioMatch[1])
//
//	return State{clientMac, connected, ssid, stationMac, ap, radio}
//}

// Flint 3 (GL-BE9300)
func parseSyslog(syslog string) State {
	connectedRegex := regexp.MustCompile("associated|disassociated")
	connectedMatch := connectedRegex.FindString(syslog)
	if connectedMatch == "" {
		log.Fatalln(`Malformed syslog: Must either include "associated" or "disassociated"`)
	}
	connected := connectedMatch == "associated"

	macsRegex := regexp.MustCompile("(([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2}))")
	macsMatch := macsRegex.FindAllString(syslog, 1)
	if len(macsMatch) != 1 {
		log.Fatalln("Malformed syslog: Must include client MAC addresses")
	}
	stationMac := "unknown"
	clientMac := strings.ToUpper(macsMatch[0])

	ssid := "unknown"

	apRegex := regexp.MustCompile(`\d\d:\d\d:\d\d\s(\w+)`)
	apMatch := apRegex.FindStringSubmatch(syslog)
	if apMatch == nil {
		log.Fatalln(`Malformed syslog: Must include the AP name e.g., "OpiRouter"`)
	}
	ap := apMatch[1]

	radioRegex := regexp.MustCompile(`wlan(\d+)`)
	radioMatch := radioRegex.FindStringSubmatch(syslog)
	if radioMatch == nil {
		log.Fatalln(`Malformed syslog: Must include the radio number e.g. "wlan2"`)
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
		log.Fatalf(`Could not connect to MQTT broker: "%s"\n`, token.Error())
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
	var connectedString string
	if device.state.connected {
		connectedString = "online"
	} else {
		connectedString = "offline"
	}
	publish(client, device.propertyStateTopic("connected"), connectedString)
	publish(client, device.propertyStateTopic("ssid"), device.state.ssid)
	publish(client, device.propertyStateTopic("station"), device.state.station)
	publish(client, device.propertyStateTopic("ap"), device.state.ap)
	publish(client, device.propertyStateTopic("radio"), strconv.Itoa(device.state.radio))
}

func publishDiscoveryProperty(client mqtt.Client, device MqttDevice, property string, available bool) {
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

	if !available {
		discoveryData["availability_topic"] = device.propertyStateTopic("dummy")
	} else if property != "connected" {
		discoveryData["availability_topic"] = device.propertyStateTopic("connected")
	}

	jsonData, jsonErr := json.Marshal(discoveryData)
	if jsonErr != nil {
		log.Fatalf(`Error occured during payload marshalling: "%s"\n`, jsonErr)
	}

	publish(client, topic, string(jsonData))
}

// Set values to available or unavailable depending on what information the router in use provides
func publishDiscovery(client mqtt.Client, device MqttDevice) {
	publishDiscoveryProperty(client, device, "connected", true)
	publishDiscoveryProperty(client, device, "ssid", false)
	publishDiscoveryProperty(client, device, "station", false)
	publishDiscoveryProperty(client, device, "ap", true)
	publishDiscoveryProperty(client, device, "radio", true)
}

func main() {
	vars := parseEnv()
	client := connectMqtt(vars)

	reader := bufio.NewReader(os.Stdin)
	for {
		syslog, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		state := parseSyslog(syslog)
		device := formatDevice(state, vars)

		publishState(client, device)
		publishDiscovery(client, device)
	}

	client.Disconnect(0)
}
