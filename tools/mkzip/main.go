// mkzip 把目录打包成 zip，并保留 Unix 可执行位（供 go:embed 使用）。
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkzip <srcdir> <out.zip>")
		os.Exit(2)
	}
	src, out := os.Args[1], os.Args[2]

	var files []string
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		fatal(err)
	}
	sort.Strings(files)

	f, err := os.Create(out)
	if err != nil {
		fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			fatal(err)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			fatal(err)
		}
		hdr := &zip.FileHeader{
			Name:     filepath.ToSlash(rel),
			Method:   zip.Deflate,
			Modified: info.ModTime(),
		}
		hdr.SetMode(info.Mode())
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			fatal(err)
		}
		in, err := os.Open(path)
		if err != nil {
			fatal(err)
		}
		if _, err := io.Copy(w, in); err != nil {
			in.Close()
			fatal(err)
		}
		in.Close()
	}
	if err := zw.Close(); err != nil {
		fatal(err)
	}
	if err := f.Close(); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d files)\n", out, len(files))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkzip:", err)
	os.Exit(1)
}
