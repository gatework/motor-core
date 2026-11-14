package json

import (
	"encoding/json"

	"github.com/gatework/motor-core/config/encoder"
)

type jsonEncoder struct{}

func (j jsonEncoder) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (j jsonEncoder) Decode(d []byte, v any) error {
	return json.Unmarshal(d, v)
}

func (j jsonEncoder) String() string {
	return "json"
}

func NewEncoder() encoder.Encoder {
	return jsonEncoder{}
}
