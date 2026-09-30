package gateway

import (
	"strconv"

	"github.com/cespare/xxhash/v2"
)

// extractPrefixHash hashes the first n bytes of the request body. Requests
// sharing a system prompt / conversation start hash to the same value.
func extractPrefixHash(body []byte, n int) uint64 {
	if len(body) < n {
		n = len(body)
	}
	return xxhash.Sum64(body[:n])
}

func hashHex(h uint64) string {
	return strconv.FormatUint(h, 16)
}
