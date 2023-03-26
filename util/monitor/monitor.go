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

func NewRopstenTxMonitor() *TxMonitor {
	return &TxMonitor{
		reader: reader.NewRopstenReader(),
	}
}

func NewTxMonitor() *TxMonitor {
	return &TxMonitor{
		reader: reader.NewEthReader(),
	}
}

func (self TxMonitor) periodicCheck(tx string, info chan common.TxInfo) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	startTime := time.Now()
	isOnNode := false
	for {
		t := <-ticker.C
		txinfo, _ := self.reader.TxInfoFromHash(tx)
		st, tx, receipt := txinfo.Status, txinfo.Tx, txinfo.Receipt
		switch st {
		case "error":
			continue
		case "notfound":
			if t.Sub(startTime) > 3*time.Minute && !isOnNode {
				info <- common.TxInfo{"lost", tx, []common.InternalTx{}, receipt, nil}
				return
			} else {
				continue
			}
		case "pending":
			isOnNode = true
			continue
		case "reverted":
			block, _ := self.reader.HeaderByNumber(receipt.BlockNumber.Int64())
			info <- common.TxInfo{"reverted", tx, []common.InternalTx{}, receipt, block}
			return
		case "done":
			block, _ := self.reader.HeaderByNumber(receipt.BlockNumber.Int64())
			info <- common.TxInfo{"done", tx, []common.InternalTx{}, receipt, block}
			return
		}
	}
}

func (self TxMonitor) MakeWaitChannel(tx string) <-chan common.TxInfo {
	result := make(chan common.TxInfo)
	go self.periodicCheck(tx, result)
	return result
}

func (self TxMonitor) BlockingWait(tx string) common.TxInfo {
	wChannel := self.MakeWaitChannel(tx)
	return <-wChannel
}

func (self TxMonitor) MakeWaitChannelForMultipleTxs(txs ...string) []<-chan common.TxInfo {
	result := [](<-chan common.TxInfo){}
	for _, tx := range txs {
		ch := make(chan common.TxInfo)
		go self.periodicCheck(tx, ch)
		result = append(result, ch)
	}
	return result
}

func waitForChannel(wg *sync.WaitGroup, channel <-chan common.TxInfo, result *sync.Map) {
	defer wg.Done()
	info := <-channel
	result.Store(info.Tx.Hash().Hex(), info)
}

func (self TxMonitor) BlockingWaitForMultipleTxs(txs ...string) map[string]common.TxInfo {
	resultMap := sync.Map{}
	wg := sync.WaitGroup{}
	channels := self.MakeWaitChannelForMultipleTxs(txs...)
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
