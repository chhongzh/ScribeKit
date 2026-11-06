// Package web 内嵌前端构建产物。web/dist 未构建时只有 .gitkeep，后端仍可编译运行。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist 返回以 dist 目录为根的文件系统。
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
