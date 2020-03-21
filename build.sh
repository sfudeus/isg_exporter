#!/usr/bin/env bash

docker buildx build --platform linux/amd64 -t sfudeus/isg_exporter:latest --push .
