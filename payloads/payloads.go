// Package payloads embeds pre-built zip bomb payloads compiled into the binary.
// Payloads are sourced from zipbomb.me © 2019-2025 Austin Hartzheim (CC BY-NC-SA 4.0).
package payloads

import _ "embed"

// Brotli100GiB is a brotli-compressed payload that expands to 100 GiB of zeros.
// Compressed size: ~79 KiB — expansion factor ~1,340,000x.
//
//go:embed 100gib.br
var Brotli100GiB []byte
