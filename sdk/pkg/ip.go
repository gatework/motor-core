package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// GetLocation 获取外网ip地址
func GetLocation(ctx context.Context, ip, key string) string {
	if ip == "127.0.0.1" || ip == "localhost" {
		return "内部IP"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://restapi.amap.com/v5/ip", nil)
	if err != nil {
		return "\u672a\u77e5\u4f4d\u7f6e"
	}
	query := req.URL.Query()
	query.Set("ip", ip)
	query.Set("type", "4")
	query.Set("key", key)
	req.URL.RawQuery = query.Encode()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "\u672a\u77e5\u4f4d\u7f6e"
	}
	defer resp.Body.Close()

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		return "\u672a\u77e5\u4f4d\u7f6e"
	}

	var m map[string]string
	err = json.Unmarshal(s, &m)
	if err != nil {
		return "\u672a\u77e5\u4f4d\u7f6e"
	}

	// Construct the location string from the response map
	location := fmt.Sprintf("%s-%s-%s-%s-%s", m["country"], m["province"], m["city"], m["district"], m["isp"])
	return location
}

// GetLocalHost 获取局域网ip地址
func GetLocalHost() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, netInterface := range netInterfaces {
		if (netInterface.Flags & net.FlagUp) != 0 {
			addrs, err := netInterface.Addrs()
			if err != nil {
				continue
			}
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String()
					}
				}
			}
		}
	}
	return ""
}
