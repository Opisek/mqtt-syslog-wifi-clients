# mqtt-syslog-wifi-clients
Detect devices connected to a wifi network via syslog messages.
Useful for easy presence detection with virtually no overhead.

# Table of Contents
- [Use-Case Scenario](#use-case-scenario)
- [Installation](#installation)
- [Usage](#usage)
- [Future Prospect](#future-prospect)

# Use-Case Scenario
I use a *"Digitalisierungsbox Smart"* router as my home router, which is a rebranded "*Bintec-elmeg be.IP plus*".
Although I'm very satisfied with this device, for some reason the rebranded version does not provide any API.

This becomes a problem when I want to programmically check which devices are connected to my wifi networks at any given point in time.

Thankfully, the router lets my forward syslog messages to other servers.
Among them there are logs from the CAPWAP controller:
```
XX:XX:XX:XX:XX:XX: New station YY:YY:YY:YY:YY:YY connected to VSS:OpiNet at WTP:OpiAP on Radio1
XX:XX:XX:XX:XX:XX: Station YY:YY:YY:YY:YY:YY disassociated from VSS:OpiNet at WTP:OpiAP on Radio1
```

The scope of this project is to parse such messages and relay the formatted information to an MQTT broker.

This way, I'm able to detect when I'm home based on my devices' wifi connection and automate things around that.

# Installation
1. Prerequisites
    - *[Golang](https://go.dev/doc/install)* must be installed
    - *[rsyslog](https://www.rsyslog.com/)* must be installed

2. Clone this repository
    ```sh
    cd ~
    git clone https://github.com/Opisek/mqtt-syslog-wifi-clients.git
    cd mqtt-syslog-wifi-clients/src
    ```

3. Environmental Variables
    - Create the `.env` file

        ```sh
        cat .env.template > .env
        ```
    - Adjust the environmental variables using your text editor of choice
    
        Variable|Meaning
        -|-
        MQTT_HOST|The IP Address of your MQTT broker
        MQTT_PORT|The port number your MQTT broker is listening on
        MQTT_USER|The username used to authenticate with the MQTT broker
        MQTT_PASS|The password used to authenticate with the MQTT broker
        MQTT_TOPIC|The base MQTT topic that will be published to

4. Compile the go code using the provided `Makefile`:
    ```sh
    make
    ```

5. *rsyslog* configuration
    - *rsyslog* must be listening on UDP (optionally TCP if you know what you're doing) for your router to communicate with it.

        This is typically done by uncommenting the following two lines in `/etc/rsyslog.conf`:

        ```c
        module(load="imudp")
        input(type="imudp" port="514")
        ```

    - *rsyslog* must have the module [*omprog*](https://rsyslog.readthedocs.io/en/latest/configuration/modules/omprog.html) activated. This is so that we can forward the logs to external programs.

        This is typically done by adding the following line to the beginning of `/etc/rsyslog.conf`:

        ```c
        module(load="omprog")
        ```

    - Open the `wificlients.conf` configuration file from the cloned repository and adjust the following:
        - Change `HOST` to the IP address of your router e.g., `10.0.0.1`
        - Change `BINARY` to the path of the compiled go file e.g, `/home/opisek/mqtt-syslog-wifi-clients/src/mqtt-syslog-wifi-clients`
    
    - Copy the edited `wificlient.conf` file to `/etc/rsyslog.d/`

6. Set up your router to use your syslog server via UDP (or TCP)

7. Restart *rsyslog*:
    ```sh
    sudo systemctl restart rsyslog

# Usage
*[Homeassistant](https://www.home-assistant.io/)* now automatically detects devices on your network, provided it's connected to the same MQTT broker.

For every MAC address on your network, a new device will be created e.g., `Wifi Client XX:XX:XX:XX:XX:XX`.

Every device exposes the following entities:

Name|ID|Meaning
-|-|-
Ap|`sensor.wifi_client_xx_xx_xx_xx_xx_xx_ap`|The name of the access point that the device is connected to
Connected|`sensor.wifi_client_xx_xx_xx_xx_xx_xx_connected`|`online` or `offline`
Radio|`sensor.wifi_client_xx_xx_xx_xx_xx_xx_radio`|The number of the access point's radio that the device is connected to
Ssid|`sensor.wifi_client_xx_xx_xx_xx_xx_xx_ssid`|The SSID that the device is connected to
Station|`sensor.wifi_client_xx_xx_xx_xx_xx_xx_station`|The MAC address of the access poitn that the device is connected to

Every entity other than `Connected` is only available when `Connected` is `online`.

Since every MAC address gets its own device, make sure to turn off your devices' ability to randomize MAC addresses every time you connect to your wifi.

# Future Prospect
In the future I'd like to automate the configuration and copying of various files by expanding on the `Makefile`.
