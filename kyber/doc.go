// Package kyber is a client for the Kyber Data API
// (https://api.kyberswap.com/data/v1).
//
// Authentication is the x-api-key header. The WebSocket also sends a
// User-Agent, which the edge requires. JARVIS_KYBER_URL overrides the
// production base URL.
//
// With --debug, each request URL and raw response body, and each socket
// frame, is appended to ~/.jarvis/debug.out. The directory is created
// when it is missing. Nothing is written when the flag is off.
package kyber
