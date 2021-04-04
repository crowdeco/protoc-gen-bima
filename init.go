package main

import "google.golang.org/protobuf/compiler/protogen"

type fileInfo struct {
	*protogen.File
	hasTimestamp bool
}

func newFileInfo(file *protogen.File, hasTimestamp bool) *fileInfo {
	f := &fileInfo{File: file}
	f.hasTimestamp = hasTimestamp

	return f
}
