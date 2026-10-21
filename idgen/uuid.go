package idgen

import "uuid"

// UUID returns a new time-sortable UUID (version 7) as a 36-character string.
//
// Version 7 UUIDs encode a 48-bit millisecond timestamp in the most significant
// bits followed by at least 62 bits of cryptographically secure random data, so
// they sort lexicographically by creation time — useful for object storage keys,
// log correlation IDs, and database keys where insert order matters.
//
// Successive calls always return increasing values, except when the system clock
// moves backwards. UUID is safe for concurrent use and needs no registration.
//
// Example output: "01a15dcb-f932-7466-8eff-3471d31055da".
func UUID() string {
	return uuid.NewV7().String()
}

// UUIDv4 returns a new fully random UUID (version 4) as a 36-character string.
//
// Unlike UUID, version 4 values carry no timestamp and are not sortable. Use it
// when the identifier must leak nothing about creation time, such as a public
// resource token. UUIDv4 is safe for concurrent use and needs no registration.
//
// Example output: "f47ac10b-58cc-4372-a567-0e02b2c3d479".
func UUIDv4() string {
	return uuid.NewV4().String()
}
