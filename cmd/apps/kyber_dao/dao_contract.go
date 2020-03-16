package kyberdao

import (
	"fmt"
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
	if err != nil {
		return 0, err
	}
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

func (self *KyberDAO) GetRewardInfo(s string, e uint64) (reward *big.Int, totalReward *big.Int, share float64, isClaimed bool, err error) {
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
	err = self.reader.ReadContract(
		&isClaimed,
		self.dao,
		"hasClaimedReward",
		ethutils.HexToAddress(s),
		big.NewInt(int64(e)),
	)
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

func (self *KyberDAO) GetVotedOptionID(s string, camID *big.Int) (*big.Int, error) {
	var result *big.Int
	err := self.reader.ReadContract(
		&result,
		self.dao,
		"stakerVotedOption",
		ethutils.HexToAddress(s),
		camID,
	)
	return result, err
}

type StakeRelatedInfo struct {
	Staker         string
	Epoch          uint64
	CurrentEpoch   uint64
	Stake          *big.Int
	Balance        *big.Int
	Allowance      *big.Int
	FutureStake    *big.Int
	PendingStake   *big.Int
	Representative string
	DelegatedStake *big.Int
}

func (self *KyberDAO) AllStakeRelatedInfo(s string, e uint64) (info *StakeRelatedInfo, err error) {
	info = &StakeRelatedInfo{
		Staker:         s,
		Epoch:          e,
		CurrentEpoch:   0,
		Stake:          nil,
		Balance:        nil,
		Allowance:      nil,
		FutureStake:    nil,
		PendingStake:   nil,
		Representative: "",
		DelegatedStake: nil,
	}

	if info.CurrentEpoch, err = self.CurrentEpoch(); err != nil {
		err = fmt.Errorf("Couldn't get current epoch: %w", err)
		return
	}
	if info.Epoch == 0 {
		info.Epoch = info.CurrentEpoch
	}
	if info.Stake, err = self.GetStake(s, info.Epoch); err != nil {
		err = fmt.Errorf("Couldn't get stake of %s at epoch %d: %w", s, info.Epoch, err)
		return
	}
	if info.Balance, err = self.reader.ERC20Balance(KNCContract, s); err != nil {
		err = fmt.Errorf("Couldn't get knc balance of %s: %w", s, err)
		return
	}
	if info.Allowance, err = self.reader.ERC20Allowance(KNCContract, s, StakingContract); err != nil {
		err = fmt.Errorf("Couldn't get knc allowance for the staking contract of %s : %w", s, err)
		return
	}
	if info.FutureStake, err = self.GetStake(s, info.Epoch+1); err != nil {
		err = fmt.Errorf("Couldn't get future stake of %s at epoch %d: %w", s, info.Epoch, err)
		return
	}
	info.PendingStake = big.NewInt(0).Sub(info.FutureStake, info.Stake)
	var poolMaster common.Address
	if poolMaster, err = self.GetPoolMaster(s, info.Epoch); err != nil {
		err = fmt.Errorf("Couldn't get representative of %s at epoch %d: %w", s, info.Epoch, err)
		return
	}
	if poolMaster.Hash().Big().Cmp(big.NewInt(0)) == 0 {
		info.Representative = info.Staker
	} else {
		info.Representative = poolMaster.Hex()
	}
	if info.DelegatedStake, err = self.GetDelegatedStake(s, info.Epoch); err != nil {
		err = fmt.Errorf("Couldn't get delegated stake of %s at epoch %d: %w", s, info.Epoch, err)
		return
	}
	return
}
