#!/bin/bash
export GOARCH=amd64

tag=$(git describe  --tags)

mkdir -p bin

build(){
	tmpdir=$(mktemp -d)
	go build -ldflags "-X main.version=${tag}" -o $tmpdir/sshcode

	pushd $tmpdir
	tarname=sshcode-$GOOS-$GOARCH.tar.gz
	tar -czf $tarname sshcode
	popd	
	cp $tmpdir/$tarname bin
	rm -rf $tmpdir
}

GOOS=darwin build
GOOS=linux build
