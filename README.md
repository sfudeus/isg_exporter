# isg_exporter

A simple tool to extract relevant status data from the Internet Service Gateway of Stiebel Eltron heat pumps like LWZ 404 and provide them as prometheus metrics.

`isg_exporter` is written in go and provided as a single binary.

`isg_exporter` will listen on a freely defineable port and emit data in prometheus format at `/metrics` which is periodically fetched from your ISG by scraping the web interface.
Acccess credentials can be configured as either cli parameters or environment parameters.

## Usage

```bash
> $ ./isg_exporter --help

Usage:
  isg_exporter [OPTIONS]

Application Options:
      --port=         The address to listen on for HTTP requests. (default: 8080) [$EXPORTER_PORT]
      --interval=     The frequency in seconds in which to gather data (default: 60) [$INTERVAL]
      --url=          URL for ISG [$ISG_URL]
      --user=         username for ISG [$ISG_USER]
      --password=     password for ISG [$ISG_PASSWORD]
      --skipCircuit2  Toogle to skip data for circuit 2 [$SKIP_CIRCUIT_2]
      --debug

Help Options:
  -h, --help          Show this help message
```

## Metrics

Metric names are based on __the configured language in the ISG__ and normalized, sample names for a german instance are:

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

## RESTFul status

So far, `isg_exporter` support a single additional restful `/status` endpoint, which provides all data in a JSON format. Eventually, it might support more interactions with the ISG and might be able to reconfigure ISG params.

## Systemd

When using systemd, a sample unit file is provided in resources/systemd. This refers to an installation of the single go binary in /usr/local/bin. Environment is configured to be configured in `/etc/default/isg_exporter`.

## Docker

A from-scratch docker image is automatically built for each new version and pushed to `sfudeus/isg_exporter:${version}` and `sfudeus/isg_exporter:latest`.

## Grafana

A sample Grafana dashboard (using prometheus) can be found at `resources/grafana`.
