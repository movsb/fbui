#!/bin/bash

set -eu

package() {
	rm -rf fbui
	mkdir -p fbui
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o fbui/fbui
	cat > fbui/config.json << 'JSON'
{
    "package":"fbui",
    "label":"小桃的吹米",
    "icontop":"logo.png",
    "launch":"./fbui"
}
JSON
	cp logo.png fbui/
	rm -f fbui.zip
	zip -r9 fbui.zip fbui
	rm -rf fbui
}

# if [ "$1" == "package" ]; then
	package
# fi
