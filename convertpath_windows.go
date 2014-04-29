// +build windows

package main

import (
	"strings"
)

func convertPath(pathFromTest string) string {
	return strings.Replace(pathFromTest, "/", "\\", -1)
}

func getPieces(path string) []string {
	return strings.Split(path, "\\")
}

func reform(pieces ...string) string {
	return strings.Join(pieces, "\\")
}
