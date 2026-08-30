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

func (tm TxMonitor) periodicCheck(tx string, info chan common.TxInfo, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	startTime := time.Now()
	isOnNode := false
	var notFoundSince time.Time

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
					info <- common.TxInfo{
						Status:  "lost",
						Tx:      tx,
						Receipt: receipt,
					}
					return
				}
			} else if t.Sub(startTime) > 3*time.Minute {
				info <- common.TxInfo{
					Status:  "lost",
					Tx:      tx,
					Receipt: receipt,
				}
				return
			}
			continue
		case "pending":
			isOnNode = true
			notFoundSince = time.Time{} // reset if tx reappears in mempool
			continue
		case "reverted":
			info <- common.TxInfo{
				Status:  "reverted",
				Tx:      tx,
				Receipt: receipt,
			}
			return
		case "done":
			info <- common.TxInfo{
				Status:  "done",
				Tx:      tx,
				Receipt: receipt,
			}
			return
		}
	}
}

func (tm TxMonitor) MakeWaitChannel(tx string) <-chan common.TxInfo {
	result := make(chan common.TxInfo)
	go tm.periodicCheck(tx, result, 5*time.Second)
	return result
}

func (tm TxMonitor) MakeWaitChannelWithInterval(tx string, interval time.Duration) <-chan common.TxInfo {
	result := make(chan common.TxInfo)
	go tm.periodicCheck(tx, result, interval)
	return result
}

func (tm TxMonitor) BlockingWait(tx string) common.TxInfo {
	wChannel := tm.MakeWaitChannel(tx)
	return <-wChannel
}
