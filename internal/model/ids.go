package model

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// TraceID is a 128-bit OpenTelemetry-compatible trace identifier.
type TraceID [16]byte

// SpanID is a 64-bit span identifier.
type SpanID [8]byte

var ZeroTraceID TraceID
var ZeroSpanID SpanID

func (t TraceID) String() string { return hex.EncodeToString(t[:]) }
func (s SpanID) String() string  { return hex.EncodeToString(s[:]) }
func (t TraceID) IsZero() bool   { return t == TraceID{} }
func (s SpanID) IsZero() bool    { return s == SpanID{} }

func (t TraceID) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *TraceID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	id, err := ParseTraceID(s)
	if err != nil {
		return err
	}
	*t = id
	return nil
}

func (s SpanID) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SpanID) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	id, err := ParseSpanID(str)
	if err != nil {
		return err
	}
	*s = id
	return nil
}

// NewTraceID generates a new random TraceID using crypto/rand.
func NewTraceID() (TraceID, error) {
	var id TraceID
	if _, err := rand.Read(id[:]); err != nil {
		return TraceID{}, fmt.Errorf("generating trace ID: %w", err)
	}
	return id, nil
}

// NewSpanID generates a new random SpanID using crypto/rand.
func NewSpanID() (SpanID, error) {
	var id SpanID
	if _, err := rand.Read(id[:]); err != nil {
		return SpanID{}, fmt.Errorf("generating span ID: %w", err)
	}
	return id, nil
}

// ParseTraceID parses a 32-character hex string into a TraceID.
func ParseTraceID(s string) (TraceID, error) {
	if len(s) != 32 {
		return TraceID{}, fmt.Errorf("trace ID must be 32 hex chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return TraceID{}, fmt.Errorf("invalid trace ID hex: %w", err)
	}
	var id TraceID
	copy(id[:], b)
	return id, nil
}

// ParseSpanID parses a 16-character hex string into a SpanID.
func ParseSpanID(s string) (SpanID, error) {
	if len(s) != 16 {
		return SpanID{}, fmt.Errorf("span ID must be 16 hex chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return SpanID{}, fmt.Errorf("invalid span ID hex: %w", err)
	}
	var id SpanID
	copy(id[:], b)
	return id, nil
}
