//go:build !lite

package main

// 非 lite（内嵌）构建：依赖已内嵌，不会走到外部解析。
func resolveExternal() error { return nil }

func scanDeps() depsReport { return depsReport{} }
