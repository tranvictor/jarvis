package kyber

import "testing"

func TestDecodeNativeOnchainFrame(t *testing.T) {
	msg := []byte(`{"e":"onchainPrice","E":1795879666553,"s":"10:0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","bp":"1645.846133594868421033","bt":1795879666553,"sp":"1645.159220703940274674","st":1795879666553}`)
	ev, _, isReply, ok := decodeFrame(msg)
	if !ok || isReply {
		t.Fatalf("frame not decoded as an event: ok=%v reply=%v", ok, isReply)
	}
	if ev.Kind != "onchainPrice" || ev.ChainID != 10 || ev.Address != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		t.Fatalf("stream: %+v", ev)
	}
	if ev.Time != 1795879666553 || !ev.HasBuy || !ev.HasSell {
		t.Fatalf("sides: %+v", ev)
	}
	if ev.Buy != "1645.846133594868421033" || ev.BuyAt != ev.Time || ev.Sell != "1645.159220703940274674" || ev.SellAt != ev.Time {
		t.Fatalf("prices: %+v", ev)
	}
}
