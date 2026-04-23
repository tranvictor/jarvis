package walletconnect

import "time"

// WalletConnect v2 relay tags + TTLs per message type. The relay
// uses the tag to route push-notifications and to enforce retention,
// and each (request, response) pair has a disjoint tag.
//
// Source: @walletconnect/sign-client rpc.ts
// (https://github.com/WalletConnect/walletconnect-monorepo/blob/v2.x/packages/sign-client/src/constants/rpc.ts).
//
// Constants are grouped by method so additions in one place are
// immediately adjacent to the matching TTL.

const (
	tagSessionProposeRequest  uint64 = 1100
	tagSessionProposeResponse uint64 = 1101
	// Reject / auto-reject (pairing) — not the same as approve (1101).
	// See specs/clients/sign/rpc-methods.md
	tagSessionProposeReject     uint64 = 1120
	tagSessionProposeRejectAuto uint64 = 1121
	tagSessionSettleRequest   uint64 = 1102
	tagSessionSettleResponse  uint64 = 1103
	tagSessionUpdateRequest   uint64 = 1104
	tagSessionUpdateResponse  uint64 = 1105
	tagSessionExtendRequest   uint64 = 1106
	tagSessionExtendResponse  uint64 = 1107
	tagSessionRequestRequest  uint64 = 1108
	tagSessionRequestResponse uint64 = 1109
	tagSessionEventRequest   uint64 = 1110
	tagSessionEventResponse    uint64 = 1111
	tagSessionDeleteRequest   uint64 = 1112
	tagSessionDeleteResponse  uint64 = 1113
	tagSessionPingRequest     uint64 = 1114
	tagSessionPingResponse    uint64 = 1115
)

var (
	ttlFiveMin = 5 * time.Minute
	ttlOneDay  = 24 * time.Hour
	ttlThirty  = 30 * time.Second
)

// wcMethod identifies the WC-protocol-level method (as opposed to the
// inner eth_* method inside wc_sessionRequest).
type wcMethod string

const (
	wcMethodSessionPropose wcMethod = "wc_sessionPropose"
	wcMethodSessionSettle  wcMethod = "wc_sessionSettle"
	wcMethodSessionUpdate  wcMethod = "wc_sessionUpdate"
	wcMethodSessionExtend  wcMethod = "wc_sessionExtend"
	wcMethodSessionRequest wcMethod = "wc_sessionRequest"
	wcMethodSessionDelete  wcMethod = "wc_sessionDelete"
	wcMethodSessionEvent   wcMethod = "wc_sessionEvent"
	wcMethodSessionPing    wcMethod = "wc_sessionPing"
)
