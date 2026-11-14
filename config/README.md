###

测试用例：
```
import (
	"fmt"
	"testing"
	
	"github.com/gatework/motor-core/config"
	"github.com/gatework/motor-core/config/source/file"
)

func TestApp(t *testing.T)  {
	c, err := config.New()
	if err != nil {
		t.Error(err)
	}
	err = c.Load(file.NewSource(file.WithPath("config/settings.yml")))
	if err != nil {
		t.Error(err)
	}
	fmt.Println(c.Map())
}
```

## Updating values

`Config.Set(value, path...)` and `reader.Values.Set(value, path...)` return an
error when a value cannot be represented as JSON. A failed update leaves the
previous value intact. Handle this error instead of assuming every arbitrary
Go value is a valid configuration value.

The manager serializes caller values before acquiring its configuration mutex.
Custom JSON marshalers may read the same manager without re-entering its lock.
The stored tree is owned by the manager; maps, slices, and large JSON numbers
are copied without losing precision.

## Environment placeholders

`${NAME}` placeholders expand only in JSON string values, including nested arrays
and objects. Variable names use letters, digits, and underscores. Missing values
expand to an empty string; malformed placeholders remain literal. JSON object
keys are not expanded. Quotes, backslashes, and newlines in an environment value
remain string data, and unrelated numbers retain their precision.
