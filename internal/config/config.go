// Package config 读取命令行参数并确定数据目录。
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const DefaultAddr = "127.0.0.1:7788"

type Config struct {
	Addr      string
	DataDir   string
	NoBrowser bool
	JSONLog   bool
}

func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法确定用户配置目录，请用 -data 指定数据目录: %w", err)
	}
	return filepath.Join(base, "ScribeKit"), nil
}

// Parse 解析 args（不含程序名）。usage 输出写到 out。
func Parse(args []string, out io.Writer) (Config, error) {
	var c Config
	fs := flag.NewFlagSet("scribekit", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&c.Addr, "addr", DefaultAddr, "监听地址")
	fs.StringVar(&c.DataDir, "data", "", "数据目录（默认为用户配置目录下的 ScribeKit）")
	fs.BoolVar(&c.NoBrowser, "no-browser", false, "启动后不自动打开浏览器")
	fs.BoolVar(&c.JSONLog, "jsonlog", false, "输出 JSON 日志（默认输出彩色控制台日志）")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if c.DataDir == "" {
		dir, err := DefaultDataDir()
		if err != nil {
			return Config{}, err
		}
		c.DataDir = dir
	}
	return c, nil
}
