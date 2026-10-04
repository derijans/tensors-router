package proxy

import (
	"bytes"
	"encoding/json"
)

func withoutJSONMember(object []byte, path []string) ([]byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(object))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return object, false
	}
	var rebuilt bytes.Buffer
	rebuilt.WriteByte('{')
	removed := false
	for decoder.More() {
		token, err := decoder.Token()
		name, isName := token.(string)
		if err != nil || !isName {
			return object, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return object, false
		}
		if name == path[0] {
			if len(path) == 1 {
				removed = true
				continue
			}
			var nestedRemoved bool
			value, nestedRemoved = withoutJSONMember(value, path[1:])
			removed = removed || nestedRemoved
		}
		writeJSONMember(&rebuilt, name, value)
	}
	if _, err := decoder.Token(); err != nil {
		return object, false
	}
	rebuilt.WriteByte('}')
	return rebuilt.Bytes(), removed
}

func writeJSONMember(object *bytes.Buffer, name string, value json.RawMessage) {
	if object.Len() > 1 {
		object.WriteByte(',')
	}
	encodedName, _ := json.Marshal(name)
	object.Write(encodedName)
	object.WriteByte(':')
	object.Write(value)
}
