package datasources

import "encoding/json"

func decodeRecord(data []byte, value any) error { return json.Unmarshal(data, value) }
func encodeRecord(value any) json.RawMessage    { b, _ := json.Marshal(value); return b }
