# isg_exporter

A simple tool to extract relevant status data from the Internet Service Gateway of Stiebel Eltron heat pumps like LWZ 404 and provide them as prometheus metrics.

`isg_exporter` is written in go and provided as a single binary.

`isg_exporter` will listen on a freely defineable port and emit data in prometheus format at `/metrics` which is periodically fetched from your ISG by scraping the web interface.
Acccess credentials can be configured as either cli parameters or environment variables.

## Usage

```bash
Usage:
  isg_exporter [OPTIONS]

Application Options:
      --port=                     The address to listen on for HTTP requests. (default: 8080) [$EXPORTER_PORT]
      --interval=                 The frequency in seconds in which to gather data (default: 60) [$INTERVAL]
      --url=                      URL for ISG [$ISG_URL]
      --user=                     username for ISG [$ISG_USER]
      --password=                 password for ISG [$ISG_PASSWORD]
      --browserRollover=          number of iterations until the internal browser is recreated (default: 60)
      --skipCircuit2              Toogle to skip data for circuit 2 [$SKIP_CIRCUIT_2]
      --debug
      --loglevel=                 logLevel (trace,debug,info,warn(ing),error,fatal,panic) (default: warn)
      --mode=                     Gathering mode (webscraping|modbus) (default: webscraping)
      --modbusSlaveId=            slaveId to use for modbus communication (default: 1)
      --mqttHost=                 MQTT host to send data to (optional)
      --mqttPort=                 MQTT port to send data to (optional) (default: 1883)
      --mqttTls                   Activate TLS for MQTT
      --mqttTlsInsecure           Allow insecure TLS for MQTT
      --mqttTopicPrefix=          Topic prefix for MQTT (default: isg)
      --mqttDiscoveryTopicPrefix= Topic prefix for homeassistant discovery (default: homeassistant)
      --mqttUser=                 Username to use for the MQTT connection [$MQTT_USER]
      --mqttPassword=             Password to use for the MQTT connection [$MQTT_PASSWORD]
      --metricsSectionPrefix      Prefix metrics with their section identifier in webscraping mode [$METRICS_SECTION_PREFIX]

Help Options:
  -h, --help             Show this help message
```

## Gathering "engines"

This exporter currently support two distinctively different modes to gather data from the ISG.
One (originally implemented) is about logging interactively into the web interface and scraping the different HTML pages and extracting data there. It makes available exactly those metrics which are available in the web frontend.

Later on, the ISG received a modbus implementation. The modbus gathering mode exposes all metrics available via Modbus - funnily, the available data on both interfaces is not the same and not with the same granularity. And this is not only about non-relevant data like the logged-in user.

You can find more infos on the distinction [here](SCRAPING_MODBUS.md).

You can choose the gathering mode by using the `--mode` switch. Default is `webscraping` for now.

## Metrics

For the webscraping mode, metric names are based on __the configured language in the ISG__ and normalized, sample names for a german instance are:

* `isg_verdichter_ww`
* `isg_verdichter_heizen`
* `isg_vorlauftemp`
* ...

Metrics without values are provided as flags (e.g. when compressor is running, heat pump is running, filters need to be changed). The are emitted with a status of 0/1.
Examples:

* `isg_flag_heizkreispumpe 0.0`
* `isg_flag_schaltprogramm_aktiv 1.0`
* `isg_flag_status_ok 1.0`
* ...

Metrics for circuit 2 can be skipped (e.g. when not in use to reduce number of metrics) by providing `--skipCircuit2` on the command line.

Some ISG do have non-unique names for their metrics in webscraping mode. It is configurable if the section names should be provided in front.
This was made configurable to not break existing configurations (e.g. Grafana dashboards), because this was only noted later.
Always adding the section prefix can be configured using the flag `--metricsWithSectionPrefix`. The default is to not use the prefix.

For the modbus mode, metric names are based on explicitly defined names in a configuration file. A [default config](modbus-mapping.yaml) is packaged in the Docker image.

## RESTFul status

So far, `isg_exporter` support a single additional restful `/status` endpoint, which provides all data in a JSON format. Eventually, it might support more interactions with the ISG and might be able to reconfigure ISG params.

## Systemd

When using systemd, a sample unit file is provided in `resources/systemd`. This refers to an installation of the single go binary in /usr/local/bin. Environment is configured to be configured in `/etc/default/isg_exporter`.

## Docker

A from-scratch docker image is automatically built for each new version and pushed to `sfudeus/isg_exporter:${version}` and `sfudeus/isg_exporter:latest`.

## Grafana

Sample Grafana dashboards (using prometheus, with and without modbus) can be found at `resources/grafana`.

## SGReady webhook support

This exporter supports setting the SGReady mode by consuming Alertmanager webhook calls.
The content of the webhook in status `firing` should provide a `target` label, which controls if the SGReady status is set to 3 (active, label value is `active`) or 1 (inactive, label value is `inactive`).
If the alert status is `resolved`, SGReady status is set back to status 2 (normal).

With this, you are free to define an arbitrary alert in Prometheus (e.g. when your power grid feed is > 3kW) which, when triggered, causes your heat pump to get active. When the alerts is lifted it will return.
Within Alertmanager, you can register isg-exporter as a receiver to only receive these alerts.
The URL endpoint for alertmanager webhooks is `/webhooks/alertmanager`.
This is available only in modbus mode, setting the SGReady status is not (yet?) implemented for webscraping mode.

There are additional prometheus metrics with counters for the amount of activations per level and the amount of errors encountered.

```text
# HELP isg_sgready_action_total Amount of successful invocations per action-level
# TYPE isg_sgready_action_total counter
isg_sgready_action_total{action="normal"} 1
# HELP isg_sgready_error_total Amount of failed invocations
# TYPE isg_sgready_error_total counter
isg_sgready_error_total 0
```

## Build

Checkout the sources and run `go build` in the main directory. There are testcases which are run with `go test`.

### Minimum go requirements

Go 1.22 is required as a minimum.
