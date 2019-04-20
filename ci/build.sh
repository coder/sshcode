#!/bin/bash
export GOARCH=amd64

build(){
	go build -o bin/sshcode-$GOOS-$GOARCH
}

GOOS=darwin build
GOOS=linux build
