package monitor

import (
	"time"

	"github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util/reader"
)

type TxMonitor struct {
	reader *reader.EthReader
}

func NewGenericTxMonitor(r *reader.EthReader) *TxMonitor {
	return &TxMonitor{r}
}

// periodicCheck polls the tx until it reaches a final state and sends that
// state on info. When status is non-nil, every intermediate state change
// ("pending" once the tx is seen in the mempool) is sent there too, and the
// final state is sent on it as well before it is closed.
func (tm TxMonitor) periodicCheck(tx string, info chan common.TxInfo, status chan string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	startTime := time.Now()
	isOnNode := false
	var notFoundSince time.Time

	finish := func(result common.TxInfo) {
		if status != nil {
			status <- result.Status
			close(status)
		}
		if info != nil {
			info <- result
		}
	}

	for {
		t := <-ticker.C
		txinfo, _ := tm.reader.TxInfoFromHash(tx)
		st, tx, receipt := txinfo.Status, txinfo.Tx, txinfo.Receipt
		switch st {
		case "error":
			continue
		case "notfound":
			if isOnNode {
				// Tx was in the mempool but is now gone —
				if notFoundSince.IsZero() {
					notFoundSince = t
				}
				if t.Sub(notFoundSince) > 1*time.Minute {
					finish(common.TxInfo{Status: "lost", Tx: tx, Receipt: receipt})
					return
				}
			} else if t.Sub(startTime) > 3*time.Minute {
				finish(common.TxInfo{Status: "lost", Tx: tx, Receipt: receipt})
				return
			}
			continue
		case "pending":
			if !isOnNode && status != nil {
				status <- "pending"
			}
			isOnNode = true
			notFoundSince = time.Time{} // reset if tx reappears in mempool
			continue
		case "reverted":
			finish(common.TxInfo{Status: "reverted", Tx: tx, Receipt: receipt})
			return
		case "done":
			finish(common.TxInfo{Status: "done", Tx: tx, Receipt: receipt})
			return
		}
	}
}

// MakeStatusChannel returns a channel that receives "pending" when the tx is
// first seen in the mempool, then exactly one final status ("done",
// "reverted" or "lost"), after which the channel is closed.
func (tm TxMonitor) MakeStatusChannel(tx string) <-chan string {
	status := make(chan string, 2)
	go tm.periodicCheck(tx, nil, status, 5*time.Second)
	return status
}

func (tm TxMonitor) MakeWaitChannelWithInterval(tx string, interval time.Duration) <-chan common.TxInfo {
	result := make(chan common.TxInfo)
	go tm.periodicCheck(tx, result, nil, interval)
	return result
}
