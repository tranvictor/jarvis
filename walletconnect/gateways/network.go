package gateways

import (
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	jarvisutil "github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/broadcaster"
	utilreader "github.com/tranvictor/jarvis/util/reader"
)

// jarvisNetReader returns a live RPC reader for net. Separated from
// the gateway so it can be swapped in tests.
func jarvisNetReader(net jarvisnetworks.Network) (utilreader.Reader, error) {
	return jarvisutil.EthReader(net)
}

// jarvisNetBroadcaster returns a broadcaster for net.
func jarvisNetBroadcaster(net jarvisnetworks.Network) (*broadcaster.Broadcaster, error) {
	return jarvisutil.EthBroadcaster(net)
}
