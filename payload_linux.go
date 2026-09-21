//go:build linux && amd64

package main

import _ "embed"

//go:embed payload/adbctl-payload-linux-amd64.zip
var embeddedPayload []byte

func isWindows() bool { return false }
