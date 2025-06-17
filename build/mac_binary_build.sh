#!/bin/sh -e
# This script is only for MacOS Developer
# Prerequisite: x86_64-unknown-linux-gnu-gcc
# brew tap SergioBenitez/osxct
# brew install x86_64-unknown-linux-gnu
cd "$(dirname "$0")/.."
CC=x86_64-unknown-linux-gnu-gcc CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make go-build
