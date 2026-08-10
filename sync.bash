#!/bin/bash

GOOS=linux GOARCH=arm64 go build && while ! rsync -Pvh trimui trimui:/mnt/SDCARD/Ports/fbui/fbui; do sleep .5; done
