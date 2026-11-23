package holdings

import "errors"

var (
	errNoReader       = errors.New("no rpc node")
	errShortMulticall = errors.New("multicall returned a short result")
)
