# Scraping vs. Modbus gathering

This exporter currently support two distinctively different modes to gather data from the ISG.
One (originally implemented) is about logging interactively into the web interface and scraping the different HTML pages and extracting data there. It makes available exactly those metrics which are available in the web frontend.

Later on, the ISG received a modbus implementation. The modbus gathering mode exposes all metrics available via Modbus - funnily, the available data on both interfaces is not the same and not with the same granularity. And this is not only about non-relevant data like the logged-in user.

## General differences

The modbus interface is designed for propgrammatic use and will only change in defined and usually compatible ways. There is a specific definition about the granularity and the correct unit of the metric.
The webinterface is meant for human use and might change (even significantly) from one versiojn of the ISG to another. This means that an ISG update might completely break the exporter at any time. So far, there is no multi-version support in the isg-exporter as well, so it can neither be told nor find out itself which version it parses and consume them accordingly (and I have no plans of implementing that).
So Modbus is meant to be the "stable" interface / stable parsing mode.

## Benefits of the modbus mode

* Stability (see above)
* Performance<br>
  The performance of the modbus mode is by nature significantly faster and less resource consuming.
* Available metrics
  * Since the modbus interface does support setting some values and it is just the same parsing them as pure metrics, the modbus interface exposes some setting parameters as well. Among these are
  * Gradient and Low End of heating curves for both circuits
  * Day, Night and Standby target temperatures for both circuits and hot water (no party or manual)
  Day, Night and Standby target stage for ventilation (no party and manual)
  * Current operating mode

## Benefits of the webscraping mode
* Available metrics<br>
  The webinterface contains a bunch of metrics the modbus interface does not contain. There are
  * Expelled air (fan speed and target flow)
  * Temperatures of evaporator and condenser
  * Current heating stage
  * Version numbers of the ISG software
  * Several diagnostic values (process values and analysis data) from diagnosis/contractor (page 2,3)

* Granularity of metrics
  * Even daily heat meters and power consumption meters are integer kWh granularity only, where the webinterface has floating point numbers