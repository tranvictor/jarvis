package kyberdao

import (
	"fmt"
	"math/big"
)

type Campaign struct {
	CampType       uint8
	StartBlock     *big.Int
	EndBlock       *big.Int
	TotalKNCSupply *big.Int
	FormulaParams  *big.Int
	Link           []byte
	Options        []*big.Int
	OptionPoints   []*big.Int
	TotalPoints    *big.Int
	WinningOption  *big.Int
	ID             *big.Int
}

func NewEmptyCampaign() *Campaign {
	return &Campaign{
		CampType:       0,
		StartBlock:     big.NewInt(0),
		EndBlock:       big.NewInt(0),
		TotalKNCSupply: big.NewInt(0),
		FormulaParams:  big.NewInt(0),
		Link:           []byte{},
		Options:        []*big.Int{},
		OptionPoints:   []*big.Int{},
		TotalPoints:    big.NewInt(0),
		WinningOption:  big.NewInt(0),
	}
}

func (self *Campaign) Type() string {
	switch self.CampType {
	case 0:
		return "general"
	case 1:
		return "fee"
	case 2:
		return "brr"
	}
	return "unsupported campaign type"
}

func (self *Campaign) HasWinningOption() bool {
	return self.WinningOption.Cmp(big.NewInt(0)) != 0
}

func (self *Campaign) LinkStr() string {
	return string(self.Link)
}

func (self *Campaign) VerboseOption(option *big.Int, id uint64, votedID *big.Int) string {
	var result string
	switch self.CampType {
	case 0:
		result = fmt.Sprintf("%d", option.Uint64())
	case 1:
		result = fmt.Sprintf("%.2f%%", float64(option.Uint64())/100)
	case 2:
		rebateBig := big.NewInt(0).Rsh(option, 128)
		rebate := float64(rebateBig.Uint64()) / 100

		temp := big.NewInt(0).Lsh(rebateBig, 128)
		rewardBig := big.NewInt(0).Sub(option, temp)
		reward := float64(rewardBig.Uint64()) / 100

		burn := 100.0 - rebate - reward
		result = fmt.Sprintf("reward: %.2f%%, rebate: %.2f%%, burn: %.2f%%", reward, rebate, burn)
	default:
		return "unsupported campaign type"
	}

	result = fmt.Sprintf("%s (%.2f%% voted)", result, percentage(self.OptionPoints[id-1], self.TotalPoints))

	if id == votedID.Uint64() {
		result = fmt.Sprintf("%s (you voted)", result)
	}

	if id == self.WinningOption.Uint64() {
		result = fmt.Sprintf("%s ==> winning option", result)
	}
	return result
}
