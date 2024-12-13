// internal/time_parser.go
// ------------------------
// This internal package provides helper functions for parsing and working with time strings and timestamps.
// These might be used by adapters or internally in the SDK to convert provider-specific time formats
// into a standard format or to check if a time is in the future.
//
// Functions:
// - ParseTimeStr: Convert strings like "1s", "6m0s" into milliseconds.
// - UnixToMs: Convert a UNIX timestamp in seconds to milliseconds.
// - IsInFuture: Check if a given timestamp (ms) is in the future.
package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTimeStr converts strings like "1s", "6m0s" into ms.
func ParseTimeStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if strings.HasSuffix(s, "s") && !strings.Contains(s, "m") {
		val := strings.TrimSuffix(s, "s")
		sec, err := strconv.Atoi(val)
		if err == nil {
			return int64(sec) * 1000
		}
	}

	var minutes, seconds int
	n, err := fmt.Sscanf(s, "%dm%ds", &minutes, &seconds)
	if n == 2 && err == nil {
		totalMs := int64(minutes)*60_000 + int64(seconds)*1_000
		return totalMs
	}

	return 0
}

// UnixToMs converts a UNIX timestamp in seconds to milliseconds.
func UnixToMs(timestamp int64) int64 {
	return timestamp * 1000
}

// IsInFuture checks if a timestamp (in ms) is in the future relative to the current time.
func IsInFuture(ms int64) bool {
	return ms > time.Now().UnixMilli()
}
