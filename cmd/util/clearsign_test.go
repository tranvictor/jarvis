package util

import (
	"bytes"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

func TestRenderContractClearSignRejectsShortCalldata(t *testing.T) {
	var buf bytes.Buffer
	u := ui.NewTerminalUIWithWriter(&buf, false)
	if RenderContractClearSign(u, networks.EthereumMainnet, "0x0000000000000000000000000000000000000001", nil, []byte{1, 2, 3}, nil, nil) {
		t.Fatal("short calldata must not match")
	}
	if buf.Len() != 0 {
		t.Fatalf("printed %q", buf.String())
	}
}

func TestInfoClearSignAttachesCallback(t *testing.T) {
	d := &util.TxDisplay{}
	r := &jarviscommon.TxResult{FunctionCall: &jarviscommon.FunctionCall{Method: "approve"}}
	InfoClearSign(networks.EthereumMainnet)(d, r, nil)
	if d.ClearSign == nil {
		t.Fatal("expected ClearSign callback")
	}
}

func TestInfoClearSignNilCallDoesNothing(t *testing.T) {
	d := &util.TxDisplay{}
	InfoClearSign(networks.EthereumMainnet)(d, &jarviscommon.TxResult{}, nil)
	if d.ClearSign != nil {
		t.Fatal("expected no callback when there is no analysed call")
	}
}
