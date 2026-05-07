package model

// ValueType represents the type of a KeyValue value.
type ValueType int

const (
	ValueString      ValueType = iota
	ValueInt
	ValueFloat
	ValueBool
	ValueStringSlice
)

// KeyValue represents an OpenTelemetry attribute key-value pair.
type KeyValue struct {
	Key  string
	Type ValueType
	SVal string
	IVal int64
	FVal float64
	BVal bool
	SArr []string
}

func StringKV(k, v string) KeyValue {
	return KeyValue{Key: k, Type: ValueString, SVal: v}
}

func IntKV(k string, v int64) KeyValue {
	return KeyValue{Key: k, Type: ValueInt, IVal: v}
}

func BoolKV(k string, v bool) KeyValue {
	return KeyValue{Key: k, Type: ValueBool, BVal: v}
}

func FloatKV(k string, v float64) KeyValue {
	return KeyValue{Key: k, Type: ValueFloat, FVal: v}
}
