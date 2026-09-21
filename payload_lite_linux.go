//go:build linux && amd64 && lite

package main

// 省空间部署版：不内嵌任何依赖，embeddedPayload 为 nil。
var embeddedPayload []byte

func isWindows() bool { return false }
