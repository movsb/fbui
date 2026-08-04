#!/bin/bash

GOOS=linux GOARCH=arm64 go build && while ! rsync -Pvh trimui trimui:/tmp/fbtest; do sleep .5; done

