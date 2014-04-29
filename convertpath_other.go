// +build !windows

package main

import (
	"strings"
)

func convertPath(pathFromTest string) string {
	return pathFromTest
}

func getPieces(path string) []string {
	return strings.Split(path, "/")
}

func reform(pieces ...string) string {
	return strings.Join(pieces, "/")
}
