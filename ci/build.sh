#!/bin/bash
export GOARCH=amd64

build(){
	tmpdir=$(mktemp -d)
	go build -o $tmpdir/sshcode

	pushd $tmpdir
	tarname=sshcode-$GOOS-$GOARCH.tar
	tar -cf $tarname sshcode
	popd	
	cp $tmpdir/$tarname bin
	rm -rf $tmpdir
}

GOOS=darwin build
GOOS=linux build
