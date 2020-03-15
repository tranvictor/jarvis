package kyberdao

import (
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
	return "unsupported type"
}

func (self *Campaign) LinkStr() string {
	return string(self.Link)
}

func (self *Campaign) VerboseOption(option *big.Int) string {
	return "not implemented yet"
}
