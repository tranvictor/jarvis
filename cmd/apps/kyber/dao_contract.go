package kyber

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
)

type KyberDAO struct {
	reader  *reader.EthReader
	staking string
	dao     string
}

func NewKyberDAO(r *reader.EthReader, staking, dao string) *KyberDAO {
	return &KyberDAO{r, staking, dao}
}

func (self *KyberDAO) CurrentEpoch() (uint64, error) {
	var res *big.Int
	err := self.reader.ReadContract(&res, self.staking, "getCurrentEpochNumber")
	return res.Uint64(), err
}

func (self *KyberDAO) GetStake(s string, e uint64) (*big.Int, error) {
	var res *big.Int
	err := self.reader.ReadContract(&res, self.staking, "getStakes",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
	return res, err
}

func (self *KyberDAO) GetPoolMaster(s string, e uint64) (common.Address, error) {
	var res common.Address
	err := self.reader.ReadContract(&res, self.staking, "getDelegatedAddress",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
	return res, err
}

func (self *KyberDAO) GetDelegatedStake(s string, e uint64) (*big.Int, error) {
	var res *big.Int
	err := self.reader.ReadContract(&res, self.staking, "getDelegatedStakes",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
	return res, err
}
