package graphqlws

import (
	"strings"
)

// ObjectPayload represents object-typed data.
type ObjectPayload map[string]interface{}

// String returns the value associated with the specified key.
//
// Key comparison is case-insensitive.
func (p ObjectPayload) String(key string) string {
	for k, v := range p {
		if strings.EqualFold(k, key) {
			value, _ := v.(string)
			return value
		}
	}

	return ""
}
