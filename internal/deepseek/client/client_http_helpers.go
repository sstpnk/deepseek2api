package client

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
)

const maxCompressedBytes = 50 << 20    // 50 MiB compressed (network wire)
const maxDecompressedBytes = 200 << 20 // 200 MiB decompressed (defense against zip bombs)

func readResponseBody(resp *http.Response) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	reader := io.LimitReader(resp.Body, maxCompressedBytes)
	switch encoding {
	case "gzip":
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer func() { _ = gz.Close() }()
		reader = io.LimitReader(gz, maxDecompressedBytes)
	case "br":
		reader = io.LimitReader(brotli.NewReader(reader), maxDecompressedBytes)
	}
	return io.ReadAll(reader)
}

func preview(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

func (c *Client) jsonHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		out[k] = v
	}
	out["Content-Type"] = "application/json"
	return out
}
