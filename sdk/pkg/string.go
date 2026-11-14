package pkg

import (
	"encoding/json"
	"strconv"
	"time"
)

// StringToInt parses a signed decimal integer.
func StringToInt(e string) (int, error) {
	return strconv.Atoi(e)
}

// GetCurrentTimeStr formats the current local time to second precision.
func GetCurrentTimeStr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// GetCurrentTime returns the current local time.
func GetCurrentTime() time.Time {
	return time.Now()
}

// StructToJSONStr encodes a value as a JSON string.
func StructToJSONStr(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
