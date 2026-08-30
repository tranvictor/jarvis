package cmd

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildSafeBatchSummaryCounts(t *testing.T) {
	out := buildSafeBatchSummary([]safeBatchResult{
		{ref: "a", status: "approved"},
		{ref: "b", status: "executed", execTxHash: "0xexec"},
		{ref: "c", status: "skipped", reason: "already signed"},
		{ref: "d", status: "failed", reason: "boom"},
		{ref: "e", status: "approved"},
	})
	if out.Total != 5 || out.Approved != 2 || out.Executed != 1 || out.Skipped != 1 || out.Failed != 1 {
		t.Fatalf("counts: %+v", out)
	}
	if _, err := time.Parse(time.RFC3339, out.Generated); err != nil {
		t.Fatalf("generated_at: %v", err)
	}
	if out.Results[1].ExecTxHash != "0xexec" {
		t.Fatalf("exec hash %q", out.Results[1].ExecTxHash)
	}
}

func TestBuildClassicBatchSummaryCountsAndHistory(t *testing.T) {
	out := buildClassicBatchSummary([]batchResult{
		{network: "mainnet", msigTxID: "1", status: "approved", initTxHash: "0xinit"},
		{network: "bsc", status: "broadcasted"},
		{network: "mainnet", status: "skipped", reason: "already executed"},
		{
			network: "mainnet",
			status:  "failed",
			history: &msigTxHistory{
				confirmations: []msigTxConfirmation{
					{txHash: "0xa", sender: "0xb"},
				},
				executionTxHash: "0xc",
			},
		},
	})
	if out.Total != 4 || out.Approved != 1 || out.Broadcasted != 1 || out.Skipped != 1 || out.Failed != 1 {
		t.Fatalf("counts: %+v", out)
	}
	jr := out.Results[3]
	if jr.ExecutionTx != "0xc" || len(jr.Confirmations) != 1 || jr.Confirmations[0].TxHash != "0xa" {
		t.Fatalf("history: %+v", jr)
	}
}

func TestBuildMixedBatchSummaryCombinesBoth(t *testing.T) {
	out := buildMixedBatchSummary(
		[]safeBatchResult{
			{ref: "safe-1", status: "approved"},
			{ref: "safe-2", status: "executed"},
		},
		[]batchResult{
			{network: "mainnet", status: "broadcasted"},
			{network: "bsc", status: "skipped", reason: "pending"},
		},
	)
	if out.Total != 4 || out.Approved != 1 || out.Executed != 1 || out.Broadcasted != 1 || out.Skipped != 1 || out.Failed != 0 {
		t.Fatalf("counts: %+v", out)
	}
	if len(out.Safe) != 2 || len(out.Classic) != 2 {
		t.Fatalf("arrays safe=%d classic=%d", len(out.Safe), len(out.Classic))
	}
	if out.Safe[0].Ref != "safe-1" || out.Classic[0].Network != "mainnet" {
		t.Fatalf("rows: %+v %+v", out.Safe[0], out.Classic[0])
	}
}

func TestBatchApproveJSONPayloadShapes(t *testing.T) {
	safe := []safeBatchResult{{
		ref:         "multisig_0xsafe_0xhash",
		network:     "mainnet",
		safeAddress: "0xsafe",
		safeTxHash:  "0xhash",
		status:      "approved",
		confirmType: "approve",
	}}
	classic := []batchResult{{
		network:    "mainnet",
		msigTxID:   "3",
		initTxHash: "0xinit",
		status:     "approved",
	}}

	assertKeys := func(t *testing.T, payload any, want, deny []string) map[string]json.RawMessage {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		for _, k := range want {
			if _, ok := raw[k]; !ok {
				t.Fatalf("missing key %q in %s", k, data)
			}
		}
		for _, k := range deny {
			if _, ok := raw[k]; ok {
				t.Fatalf("unexpected key %q in %s", k, data)
			}
		}
		return raw
	}

	assertKeys(t, batchApproveJSONPayload(safe, nil),
		[]string{"results", "generated_at", "executed", "approved"},
		[]string{"safe", "classic", "broadcasted"},
	)
	assertKeys(t, batchApproveJSONPayload(nil, classic),
		[]string{"results", "broadcasted", "approved"},
		[]string{"safe", "classic", "generated_at", "executed"},
	)
	assertKeys(t, batchApproveJSONPayload(safe, classic),
		[]string{"safe", "classic", "generated_at", "executed", "broadcasted"},
		[]string{"results"},
	)
	data, err := json.Marshal(batchApproveJSONPayload(safe, classic))
	if err != nil {
		t.Fatal(err)
	}
	var mixed jsonMixedBatchSummary
	if err := json.Unmarshal(data, &mixed); err != nil {
		t.Fatal(err)
	}
	if mixed.Total != 2 || len(mixed.Safe) != 1 || len(mixed.Classic) != 1 {
		t.Fatalf("mixed: %+v", mixed)
	}
	if mixed.Safe[0].SafeTxHash != "0xhash" || mixed.Classic[0].MsigTxID != "3" {
		t.Fatalf("mixed rows: %+v %+v", mixed.Safe[0], mixed.Classic[0])
	}

	if batchApproveJSONPayload(nil, nil) != nil {
		t.Fatal("empty payload should be nil")
	}
}
