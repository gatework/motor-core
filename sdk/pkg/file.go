package pkg

import (
	"bytes"
	"os"
	"strings"
)

// PathCreate creates the directory and missing parents with private permissions.
func PathCreate(dir string) error {
	return os.MkdirAll(dir, 0o750)
}

// PathExist 判断目录是否存在
func PathExist(addr string) bool {
	info, err := os.Stat(addr)
	return err == nil && info.IsDir()
}

// FileCreate writes content to name, creating the file or truncating it. It
// returns the error from creating, writing or closing the file. It used to
// return nothing: it logged those errors, and when the file could not be
// created it went on to close a nil file and ended the process.
func FileCreate(content bytes.Buffer, name string) error {
	return os.WriteFile(name, content.Bytes(), 0o600)
}

// GetFileSize 获取文件大小
func GetFileSize(filename string) int64 {
	info, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return info.Size()
}

// GetCurrentPath 获取当前路径，比如：E:/abc/data/test
func GetCurrentPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(dir, "\\", "/")
}
