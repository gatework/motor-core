package user

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
)

func contextWith(claims jwt.MapClaims) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(jwt.JwtPayloadKey, claims)
	return c
}

// Every reader here used a bare type assertion, which panics on any type but
// the one it named — in a request handler, over claims whose shape a caller
// can influence. A claim that is not what was expected now reads as absent.
func TestClaimsOfTheWrongTypeDoNotPanic(t *testing.T) {
	c := contextWith(jwt.MapClaims{
		"identity": "not a number at all",
		"nice":     42.0,
		"rolekey":  []string{"unexpected"},
		"roleid":   map[string]int{"a": 1},
		"deptid":   true,
		"deptkey":  99.0,
	})

	if got := UserID(c); got != 0 {
		t.Errorf("UserID = %d, want 0", got)
	}
	if got := UserName(c); got != "" {
		t.Errorf("UserName = %q, want empty", got)
	}
	if got := RoleName(c); got != "" {
		t.Errorf("RoleName = %q, want empty", got)
	}
	if got := RoleID(c); got != 0 {
		t.Errorf("RoleID = %d, want 0", got)
	}
	if got := DepartmentID(c); got != 0 {
		t.Errorf("DepartmentID = %d, want 0", got)
	}
	if got := DepartmentName(c); got != "" {
		t.Errorf("DepartmentName = %q, want empty", got)
	}
}

// A float64 cannot hold an identity above 2^53 exactly, which is where
// snowflake ids live. json.Number keeps the digits, and a parser configured
// with UseNumber hands one over.
func TestAnIdentityBeyondFloat64PrecisionSurvives(t *testing.T) {
	const big = int64(1<<53) + 1

	if float64(big) == float64(big-1) {
		// Stated rather than assumed: this is the property that makes the
		// float64 path lossy in the first place.
		t.Logf("float64 cannot separate %d from %d", big, big-1)
	} else {
		t.Fatalf("%d is not beyond float64 precision; the test needs a larger value", big)
	}

	c := contextWith(jwt.MapClaims{"identity": json.Number("9007199254740993")})
	if got := UserIDString(c); got != "9007199254740993" {
		t.Errorf("UserIDString = %q, want the digits unchanged", got)
	}
}

func TestOrdinaryClaimsStillRead(t *testing.T) {
	c := contextWith(jwt.MapClaims{
		"identity": float64(7),
		"nice":     "alice",
		"rolekey":  "admin",
		"roleid":   float64(3),
		"deptid":   float64(5),
		"deptkey":  "hq",
	})

	if got := UserID(c); got != 7 {
		t.Errorf("UserID = %d, want 7", got)
	}
	if got := UserName(c); got != "alice" {
		t.Errorf("UserName = %q, want alice", got)
	}
	if got := RoleID(c); got != 3 {
		t.Errorf("RoleID = %d, want 3", got)
	}
	if got := DepartmentID(c); got != 5 {
		t.Errorf("DepartmentID = %d, want 5", got)
	}
	if got := DepartmentName(c); got != "hq" {
		t.Errorf("DepartmentName = %q, want hq", got)
	}
}

// Accepting a number for a claim that has to be text would make the reading
// depend on how the parser was configured: json.Number where UseNumber is on,
// float64 where it is off, and only one of those was being accepted.
func TestNumericClaimsAreNotNames(t *testing.T) {
	for _, claims := range []jwt.MapClaims{
		{"nice": json.Number("42"), "rolekey": json.Number("7"), "deptkey": json.Number("9")},
		{"nice": float64(42), "rolekey": float64(7), "deptkey": float64(9)},
	} {
		c := contextWith(claims)
		if got := UserName(c); got != "" {
			t.Errorf("UserName = %q, want empty", got)
		}
		if got := RoleName(c); got != "" {
			t.Errorf("RoleName = %q, want empty", got)
		}
		if got := DepartmentName(c); got != "" {
			t.Errorf("DepartmentName = %q, want empty", got)
		}
	}
}

func TestExtractClaimsRejectsForeignContextValue(t *testing.T) {
	c := contextWith(nil)
	c.Set(jwt.JwtPayloadKey, map[string]any{"identity": float64(42)})
	if claims := ExtractClaims(c); len(claims) != 0 {
		t.Fatalf("accepted foreign claims: %v", claims)
	}
}
