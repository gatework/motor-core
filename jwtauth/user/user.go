package user

import (
	"fmt"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/pkg"
)

// claimString reads a claim that has to be text.
//
// A number is not accepted in either encoding it can arrive in. Accepting
// json.Number and not float64 would make the reading depend on how the parser
// was configured: the same token would give a name of "42" with UseNumber on
// and no name at all with it off.
func claimString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// ExtractClaims reads claims using the middleware's validated representation.
func ExtractClaims(c *gin.Context) jwt.MapClaims {
	return jwt.ExtractClaims(c)
}

func Get(c *gin.Context, key string) any {
	data := ExtractClaims(c)
	if data[key] != nil {
		return data[key]
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " Get 缺少 " + key)
	return nil
}

func UserID(c *gin.Context) int {
	data := ExtractClaims(c)
	if data["identity"] != nil {
		if id, ok := data.Int64("identity"); ok {
			return int(id)
		}
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " UserID: identity is not a number")
		return 0
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " UserID 缺少 identity")
	return 0
}

func UserIDString(c *gin.Context) string {
	data := ExtractClaims(c)
	if data["identity"] != nil {
		if id, ok := data.Int64("identity"); ok {
			return pkg.Int64ToString(id)
		}
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " UserIDString: identity is not a number")
		return ""
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " UserIDString 缺少 identity")
	return ""
}

func UserName(c *gin.Context) string {
	data := ExtractClaims(c)
	if data["nice"] != nil {
		if s, ok := claimString(data["nice"]); ok {
			return s
		}
		return ""
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " UserName 缺少 nice")
	return ""
}

func RoleName(c *gin.Context) string {
	data := ExtractClaims(c)
	if data["rolekey"] != nil {
		if s, ok := claimString(data["rolekey"]); ok {
			return s
		}
		return ""
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " RoleName 缺少 rolekey")
	return ""
}

func RoleID(c *gin.Context) int {
	data := ExtractClaims(c)
	if data["roleid"] != nil {
		id, ok := data.Int64("roleid")
		if !ok {
			return 0
		}
		i := int(id)
		return i
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " RoleID 缺少 roleid")
	return 0
}

func DepartmentID(c *gin.Context) int {
	data := ExtractClaims(c)
	if data["deptid"] != nil {
		id, ok := data.Int64("deptid")
		if !ok {
			return 0
		}
		i := int(id)
		return i
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " DepartmentID 缺少 deptid")
	return 0
}

func DepartmentName(c *gin.Context) string {
	data := ExtractClaims(c)
	if data["deptkey"] != nil {
		if s, ok := claimString(data["deptkey"]); ok {
			return s
		}
		return ""
	}
	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " DepartmentName 缺少 deptkey")
	return ""
}
