package utils

import (
	"encoding/base64"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IsStringEmpty reports whether a string contains only spaces.
func IsStringEmpty(str string) bool {
	return strings.Trim(str, " ") == ""
}

// GetUUID returns a random UUID without separators.
func GetUUID() string {
	u := uuid.New()
	return strings.ReplaceAll(u.String(), "-", "")
}

// PathExists reports whether an entry exists at a path.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}

	if os.IsNotExist(err) {
		return false
	}

	return false
}

// Base64ToImage decodes a base64 image payload.
func Base64ToImage(imageBase64 string) ([]byte, error) {
	image, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return nil, err
	}

	return image, nil
}

// GetDirFiles returns the regular file paths under a directory.
func GetDirFiles(dir string) ([]string, error) {
	dirList, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	filesRet := make([]string, 0)

	for _, file := range dirList {
		if file.IsDir() {
			files, err := GetDirFiles(dir + string(os.PathSeparator) + file.Name())
			if err != nil {
				return nil, err
			}

			filesRet = append(filesRet, files...)
		} else {
			filesRet = append(filesRet, dir+string(os.PathSeparator)+file.Name())
		}
	}

	return filesRet, nil
}

// GetCurrentTimeStamp returns the current Unix time in milliseconds.
func GetCurrentTimeStamp() int64 {
	return time.Now().UnixNano() / 1e6
}

// RemoveRepByMap returns slc without its duplicates, keeping first order.
func RemoveRepByMap(slc []string) []string {
	result := []string{}
	tempMap := map[string]byte{}
	for _, e := range slc {
		l := len(tempMap)
		tempMap[e] = 0
		if len(tempMap) != l {
			result = append(result, e)
		}
	}
	return result
}
