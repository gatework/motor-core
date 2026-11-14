package user

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth"
)

// ExtractClaims reads claims using the middleware's validated representation.
func ExtractClaims(c *gin.Context) jwtauth.MapClaims {
	return jwtauth.ExtractClaims(c)
}

// Get returns a validated claim by key, or nil when it is absent.
func Get(c *gin.Context, key string) any {
	return ExtractClaims(c)[key]
}

// ID returns the numeric identity claim, or zero when absent or invalid.
func ID(c *gin.Context) int {
	return number(c, "identity")
}

// IDString returns the identity claim as decimal digits, or an empty string.
func IDString(c *gin.Context) string {
	id, ok := ExtractClaims(c).Int64("identity")
	if !ok {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

// Name returns the textual display-name claim.
func Name(c *gin.Context) string {
	return text(c, "nice")
}

// RoleName returns the role-key claim.
func RoleName(c *gin.Context) string {
	return text(c, "rolekey")
}

// RoleID returns the numeric role claim, or zero when absent or invalid.
func RoleID(c *gin.Context) int {
	return number(c, "roleid")
}

// DepartmentID returns the numeric department claim, or zero when absent or invalid.
func DepartmentID(c *gin.Context) int {
	return number(c, "deptid")
}

// DepartmentName returns the textual department-key claim.
func DepartmentName(c *gin.Context) string {
	return text(c, "deptkey")
}

func text(c *gin.Context, key string) string {
	value, _ := Get(c, key).(string)
	return value
}

func number(c *gin.Context, key string) int {
	value, ok := ExtractClaims(c).Int64(key)
	if !ok {
		return 0
	}
	id := int(value)
	if int64(id) != value {
		return 0
	}
	return id
}
