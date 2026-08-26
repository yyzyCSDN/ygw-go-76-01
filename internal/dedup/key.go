package dedup

import (
	"strconv"
	"time"

	"github.com/cespare/xxhash/v2"
)

func SwipeKey(ticketID string) string {
	return keyOf("swipe", ticketID)
}

func EventFingerprint(gateID string, kind string, at time.Time) string {
	payload := gateID + "|" + kind + "|" + at.UTC().Format(time.RFC3339Nano)
	digest := xxhash.Sum64String(payload)
	return strconv.FormatUint(digest, 16)
}

func keyOf(scope string, id string) string {
	payload := scope + ":" + id
	digest := xxhash.Sum64String(payload)
	return strconv.FormatUint(digest, 16)
}
