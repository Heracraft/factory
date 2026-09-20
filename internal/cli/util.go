package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
)

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// writeJSONOut is what every read command's --json flag uses: one JSON
// value, nothing else on stdout (07-cli.md checklist: "repose status
// --json | jq . in CI").
func writeJSONOut(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
