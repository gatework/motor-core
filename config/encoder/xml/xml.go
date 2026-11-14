package xml

import (
	"encoding/xml"

	"github.com/gatework/motor-core/config/encoder"
)

type xmlEncoder struct{}

func (x xmlEncoder) Encode(v any) ([]byte, error) {
	return xml.Marshal(v)
}

func (x xmlEncoder) Decode(d []byte, v any) error {
	return xml.Unmarshal(d, v)
}

func (x xmlEncoder) String() string {
	return "xml"
}

func NewEncoder() encoder.Encoder {
	return xmlEncoder{}
}
