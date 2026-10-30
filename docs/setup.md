# First boot, settings and MQTT

## First boot: WiFi setup

With no WiFi configured, the device starts its own open access point,
`tty2oled-XXXX` (last two bytes of its MAC address). Connect to it with
a phone or laptop — a captive-portal setup page should open
automatically, or browse to `http://192.168.4.1/`. It can scan for
nearby networks or you can type the SSID/password in by hand.

While in AP mode, pressing the wake button shows a scannable WiFi-join
QR code on the round display (encoding the open network's SSID) instead
of its normal screensaver-wake behavior.

Once configured, the device joins your network and is reachable at
`tty2oled-XXXX.local` (mDNS) — the round display's "Connected" screen
shows this hostname, along with the current IP. Visiting that address in
a browser shows a small status page with a "Forget WiFi" button (which
restarts the device back into AP mode) and, once configured, MQTT broker
settings (see [MQTT notifications](#mqtt-notifications) below).

If the saved network isn't available at power-up, the device tries it three
times before falling back to its own access point, and from the access point
it retries the saved network by itself every couple of minutes while nobody is
connected to it. `http://<device>/status` shows the signal strength
(`rssi`, in dBm; around -55 is good, below -75 is marginal).

## Device settings (v0.5.0)

Once the device is on your network, `http://tty2oled-XXXX.local/settings` has a
settings page: status (firmware, signal, time, free memory), brightness and
orientation defaults, an **idle clock** (on by default: the time replaces the picture after 15 minutes idle,
once the time is known; set 0 to turn it off; optionally blank after a further Y minutes; any command or the BOOT button
brings the picture back), the status-OLED idle blank, **NTP** time with a
timezone list (or a custom POSIX TZ string), an optional admin password
(user `admin`) and **tint for monochrome pictures**: the MiSTer's black-and-white
marquees (and grayscale pictures from the web app) can be coloured with a fixed colour, a gradient, or a random colour
per picture. These are the defaults at power-up; a MiSTer's own
`CMDCON`/`CMDROT`/`CMDSAVER` commands override them until the next reboot.

## MQTT notifications

Point the device at an MQTT broker (Home Assistant's built-in Mosquitto
add-on, or any other local broker) via the web status page's MQTT
section, and it'll show:

- A temporary text banner on any message published to `<prefix>/text`.
- A fetched image on any message published to `<prefix>/image` (the
  payload is a URL to a JPEG, not the image bytes themselves).

`<prefix>` defaults to `tty2oled/<device name>` and is configurable.
Both notification types show for a configurable duration (default 8s),
wake the display if the screensaver had blanked it, and then revert to
whatever marquee art was showing before them. Plain MQTT only (port
1883, no TLS) — suited to a local-network broker, not a cloud one.
