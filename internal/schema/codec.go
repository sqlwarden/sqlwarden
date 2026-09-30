package schema

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
)

func gzipJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := json.NewEncoder(gz).Encode(v); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipJSON(data []byte, v any) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
