// Package jsonconv converts JSON keys into camel case.
package jsonconv

import (
	"encoding/json"
	"strings"
)

// Marshal converts x into JSON, while also normalizing the keys.
func Marshal(x any) ([]byte, error) {
	data, err := json.Marshal(x)
	if err != nil {
		return nil, err
	}

	var jdata any
	if err := json.Unmarshal(data, &jdata); err != nil {
		return nil, err
	}
	processed := normalize(jdata)
	return json.MarshalIndent(processed, "", "  ")
}

func normalize(v any) any {
	switch obj := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(obj))
		for k, val := range obj {
			m[camelCase(k)] = normalize(val)
		}
		return m
	case []any:
		for i, val := range obj {
			obj[i] = normalize(val)
		}
		return obj
	}
	return v
}

// camelCase converts 'public_key' to 'PublicKey'. Words like id, url are recognized and converted to ID and
// URL respectively. If a key contains dots (mostly labels, it is left alone).
func camelCase(s string) string {
	if strings.Contains(s, ".") {
		return s
	}

	if strings.ToLower(s) != s { // probably alredy in camelcase
		return s
	}

	splits := strings.Split(s, "_")
	sb := strings.Builder{}
	for _, split := range splits {
		switch split {
		case "id", "Id":
			fallthrough
		case "url", "Url":
			fallthrough
		case "tcp", "Tcp":
			sb.WriteString(strings.ToUpper(split))
		default:
			sb.WriteString(strings.ToUpper(split[0:1]))
			sb.WriteString(strings.ToLower(split[1:]))
		}
	}
	return sb.String()
}
