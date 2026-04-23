// Package relay is the WalletConnect v2 relay transport: a WebSocket
// client speaking the "irn" JSON-RPC protocol against the public
// relay at wss://relay.walletconnect.org.
//
// Lifecycle is tied to a single Client struct (one per WC session).
// Dial returns a live client; callers then subscribe to topics and
// publish encrypted messages. Incoming envelopes are delivered via
// Client.Incoming(), a single-consumer channel.
//
// The relay protocol (as implemented by @walletconnect/relay-api):
//
//	client → relay:  {"id":1,"method":"irn_subscribe","params":{"topic":"<hex>"}}
//	client ← relay:  {"id":1,"result":"<subscriptionId>"}
//	client → relay:  {"id":2,"method":"irn_publish","params":{"topic":..,"message":..,"ttl":..,"tag":..,"prompt":..}}
//	client ← relay:  {"id":2,"result":true}
//	client ← relay:  {"id":99,"method":"irn_subscription","params":{"id":"<subId>","data":{"topic":..,"message":..,"publishedAt":..}}}
//	client → relay:  {"id":99,"result":true}        (ack the subscription delivery)
//
// All "messages" on the wire are already base64-encoded WC envelopes
// — this package is message-content agnostic, it just routes bytes by
// topic.
package relay
