package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTimeStr converts strings like "1s", "6m0s" into milliseconds.
// Returns 0 if parsing fails or the format is unsupported.
func ParseTimeStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Simple just-seconds format like "10s"
	if strings.HasSuffix(s, "s") && !strings.Contains(s, "m") {
		val := strings.TrimSuffix(s, "s")
		sec, err := strconv.Atoi(val)
		if err == nil {
			return int64(sec) * 1000
		}
	}

	// "XmYs" format
	var minutes, seconds int
	n, err := fmt.Sscanf(s, "%dm%ds", &minutes, &seconds)
	if n == 2 && err == nil {
		totalMs := int64(minutes)*60_000 + int64(seconds)*1_000
		return totalMs
	}

	return 0
}

// UnixToMs converts a UNIX timestamp (seconds) to ms.
func UnixToMs(timestamp int64) int64 {
	return timestamp * 1000
}

// IsInFuture checks if ms timestamp is in the future.
func IsInFuture(ms int64) bool {
	return ms > time.Now().UnixMilli()
}
