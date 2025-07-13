package monitor

import (
	"sync"
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
	for {
		t := <-ticker.C
		txinfo, _ := tm.reader.TxInfoFromHash(tx)
		st, tx, receipt := txinfo.Status, txinfo.Tx, txinfo.Receipt
		switch st {
		case "error":
			continue
		case "notfound":
			if t.Sub(startTime) > 3*time.Minute && !isOnNode {
				info <- common.TxInfo{
					Status:  "lost",
					Tx:      tx,
					Receipt: receipt,
				}
				return
			} else {
				continue
			}
		case "pending":
			isOnNode = true
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

func (tm TxMonitor) MakeWaitChannelForMultipleTxs(txs ...string) []<-chan common.TxInfo {
	result := [](<-chan common.TxInfo){}
	for _, tx := range txs {
		ch := make(chan common.TxInfo)
		go tm.periodicCheck(tx, ch, 5*time.Second)
		result = append(result, ch)
	}
	return result
}

func (tm TxMonitor) MakeWaitChannelForMultipleTxsWithInterval(interval time.Duration, txs ...string) []<-chan common.TxInfo {
	result := [](<-chan common.TxInfo){}
	for _, tx := range txs {
		ch := make(chan common.TxInfo)
		go tm.periodicCheck(tx, ch, interval)
		result = append(result, ch)
	}
	return result
}

func waitForChannel(wg *sync.WaitGroup, channel <-chan common.TxInfo, result *sync.Map) {
	defer wg.Done()
	info := <-channel
	result.Store(info.Tx.Hash().Hex(), info)
}

func (tm TxMonitor) BlockingWaitForMultipleTxs(txs ...string) map[string]common.TxInfo {
	resultMap := sync.Map{}
	wg := sync.WaitGroup{}
	channels := tm.MakeWaitChannelForMultipleTxs(txs...)
	for _, channel := range channels {
		wg.Add(1)
		go waitForChannel(&wg, channel, &resultMap)
	}
	wg.Wait()
	result := map[string]common.TxInfo{}
	resultMap.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(common.TxInfo)
		return true
	})
	return result
}
