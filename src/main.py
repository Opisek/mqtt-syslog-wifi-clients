import os
import sys
import dotenv
import paho.mqtt.client as mqtt
import re
import json

class State:
	def __init__(self, mac: str, connected: bool, station: str, ap: str, radio: str, ssid: str):
		self.mac = mac
		self.connected = connected
		self.station = station
		self.ap = ap
		self.radio = radio
		self.ssid = ssid

class Vars:
	def __init__(self, host: str, port: int, user: str, password: str, topic: str):
		self.host = host
		self.port = port
		self.user = user
		self.password = password
		self.topic = topic

def getVars() -> Vars:
	dotenv.load_dotenv()

	mqttHost = os.getenv("MQTT_HOST")
	if mqttHost == None: raise Exception("Malformed .env provided: MQTT_HOST missing")
	mqttPort = os.getenv("MQTT_PORT")
	if mqttPort == None: raise Exception("Malformed .env provided: MQTT_PORT missing")
	mqttUser = os.getenv("MQTT_USER")
	if mqttUser == None: raise Exception("Malformed .env provided: MQTT_USER missing")
	mqttPass = os.getenv("MQTT_PASS")
	if mqttPass == None: raise Exception("Malformed .env provided: MQTT_PASS missing")
	mqttTopic = os.getenv("MQTT_TOPIC")
	if mqttTopic == None: raise Exception("Malformed .env provided: MQTT_TOPIC missing")

	return Vars(mqttHost, int(mqttPort), mqttUser, mqttPass, mqttTopic)

def getSyslog() -> str:
	args = sys.argv
	if len(args) != 2:
		print("a")
		raise Exception('Wrong amount of parameters\n Usage: "python3 main.py <syslog message>"')
	return args[1]

def parseSyslog(syslog: str) -> State:
	connected = re.search("connected|disassociated", syslog)
	if connected == None:
		raise ValueError('Malformed syslog provided: Must either include "connected" or "disassociated"')
	isConnected = connected.string == "connected"

	macs = re.findall("(([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2}))", syslog)
	if len(macs) != 2:
		raise ValueError('Malformed syslog provided: Must include exactly 2 MAC addresses')
	
	ssid = re.search("VSS:(\w+)", syslog)
	if ssid == None:
		raise ValueError('Malformed syslog provided: Must include the SSID e.g., "VSS:OpiNet"')

	ap = re.search("WTP:(\w+)", syslog)
	if ap == None:
		raise ValueError('Malformed syslog provided: Must include the AP name e.g., "WTP:OpiAP"')

	radio = re.search("Radio(\d+)", syslog)
	if radio == None:
		raise ValueError('Malformed syslog provided: Must include the radio number e.g., "Radio1"')

	return State(macs[1][0], isConnected, macs[0][0], ap.group(1), radio.group(1), ssid.group(1))


def connect(host: str, port: int, user: str, password: str) -> mqtt.Client:
	mqttClient = mqtt.Client("presence")
	mqttClient.username_pw_set(user, password)
	mqttClient.connect(host, port)
	return mqttClient

def publishState(mqttClient: mqtt.Client, state: State, vars: Vars):
	mqttClient.publish(
		f"{vars.topic}/{state.mac}/connected",
		"true" if state.connected else "false",
		retain=True
	)

	keys = [key for key in state.__dict__.keys() if not key == "connected"]
	for key in keys:
		mqttClient.publish(
			f"{vars.topic}/{state.mac}/{key}",
			state.__dict__[key],
			retain=True
		)

def formatDeviceId(state: State) -> str:
	return f"wifi_client_{state.mac}"

def formatDeviceName(state: State) -> str:
	return f"Wifi Client {state.mac}"

def formatDiscoveryPropertyPath(vars: Vars, key: str) -> str:
	return f"homeassistant/sensor/{formatDeviceId(state)}/{key}/config"

def formatDiscoveryPayload(state: State, vars: Vars, key: str, additional: str = None) -> str:
	id = formatDeviceId(state)
	name = formatDeviceName(state)
	return (
		f'{{'
			f'"unique_id":"{id}_{key}",'
			f'"name":"{name} {key.title()}",'
			f'"device":{{'
				f'"identifiers":"{id}",'
				f'"name":"{name}"'
			f'}},'
			f'"state_topic":"{vars.topic}/{state.mac}/{key}"'
			f'{"" if additional == None else f",{additional}"}'
		f'}}'
	)

def publishDiscovery(mqttClient: mqtt.Client, state: State, vars: Vars):
	mqttClient.publish(
		formatDiscoveryPropertyPath(vars, "connected"),
		formatDiscoveryPayload(state, vars, "connected"),
		retain=True
	)

	keys = [key for key in state.__dict__.keys() if not key == "connected"]
	for key in keys:
		mqttClient.publish(
		formatDiscoveryPropertyPath(vars, key),
		formatDiscoveryPayload(state, vars, key, f'"availability_topic":"{vars.topic}/{state.mac}/connected"'),
		retain=True
	)

if __name__ == "__main__":
	vars = getVars()
	state = parseSyslog(getSyslog())
	mqttClient = connect(vars.host, vars.port, vars.user, vars.password)

	publishState(mqttClient, state, vars)
	publishDiscovery(mqttClient, state, vars)

	mqttClient.disconnect()