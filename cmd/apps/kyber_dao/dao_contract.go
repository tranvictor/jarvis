package kyberdao

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
)

type KyberDAO struct {
	reader     *reader.EthReader
	staking    string
	dao        string
	feeHandler string
}

func NewKyberDAO(r *reader.EthReader, staking, dao, feeHandler string) *KyberDAO {
	return &KyberDAO{r, staking, dao, feeHandler}
}

func (self *KyberDAO) CurrentEpoch() (uint64, error) {
	var res *big.Int
	err := self.reader.ReadContract(&res, self.staking, "getCurrentEpochNumber")
	return res.Uint64(), err
}

func (self *KyberDAO) GetStake(s string, e uint64) (*big.Int, error) {
	var res *big.Int
	err := self.reader.ReadContract(&res, self.staking, "getStake",
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
	err := self.reader.ReadContract(&res, self.staking, "getDelegatedStake",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
	return res, err
}

func (self *KyberDAO) GetRewardInfo(s string, e uint64) (reward *big.Int, totalReward *big.Int, share float64, err error) {
	var shareBig *big.Int
	err = self.reader.ReadContract(
		&shareBig,
		self.dao,
		"getStakerRewardPercentageInPrecision",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
	if err != nil {
		return
	}
	err = self.reader.ReadContract(
		&totalReward,
		self.feeHandler,
		"rewardsPerEpoch",
		big.NewInt(int64(e)),
	)
	if err != nil {
		return
	}
	reward = big.NewInt(0).Mul(totalReward, shareBig)
	reward = big.NewInt(0).Div(reward, ethutils.FloatToBigInt(1.0, 18))
	share = ethutils.BigToFloat(shareBig, 18)
	return
}

func (self *KyberDAO) GetCampaignIDs(e uint64) ([]*big.Int, error) {
	result := []*big.Int{}
	err := self.reader.ReadContract(
		&result,
		self.dao,
		"getListCampIDs",
		big.NewInt(int64(e)),
	)
	return result, err
}

func (self *KyberDAO) GetCampaignDetail(id *big.Int) (*Campaign, error) {
	result := NewEmptyCampaign()
	err := self.reader.ReadContract(
		result,
		self.dao,
		"getCampaignDetails",
		id,
	)
	return result, err
}
