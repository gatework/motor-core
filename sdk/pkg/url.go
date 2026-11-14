package pkg

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// IDsStrToIDsIntGroup parses a comma-separated list of IDs.
func IDsStrToIDsIntGroup(key string, c *gin.Context) []int {
	return IDsStrToIDsIntGroupStr(c.Param(key))
}

// IDs holds a set of integer identifiers.
type IDs struct {
	IDs []int
}

// IDsStrToIDsIntGroupStr parses a comma-separated identifier list.
func IDsStrToIDsIntGroupStr(keys string) []int {
	parsed := make([]int, 0)
	ids := strings.Split(keys, ",")
	for i := range ids {
		ID, _ := StringToInt(ids[i])
		parsed = append(parsed, ID)
	}
	return parsed
}
