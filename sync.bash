#!/bin/bash

GOEXPERIMENT=simd GOOS=linux GOARCH=arm64 go build && while ! rsync -Pvh fbui trimui:/mnt/SDCARD/Ports/fbui/fbui; do sleep .5; done
