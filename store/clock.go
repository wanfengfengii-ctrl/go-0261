package store

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// idCounter provides a monotonically increasing process-local component for ID
// generation, guaranteeing uniqueness without external state.
var idCounter atomic.Uint64

// randID returns a short unique identifier composed of a nanosecond timestamp
// and a monotonic counter. It is used for internal row primary keys (leases,
// evidence, adapter calls, reviews) whose exact value is not part of the public
// contract.
func randID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) + "-" + itoa(idCounter.Add(1)) + "-" + itoa(uint64(time.Now().UnixNano()))
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
