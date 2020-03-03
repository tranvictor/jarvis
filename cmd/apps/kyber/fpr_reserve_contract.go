package kyber

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
)

type FPRReserveContract struct {
	Address                string
	ConversionRateContract *common.Address
	reader                 *reader.EthReader
}

func NewFPRReserveContract(address string, r *reader.EthReader) (*FPRReserveContract, error) {

	conversionRateContract, err := r.AddressFromContract(address, "conversionRatesContract")
	if err != nil {
		return nil, err
	}

	return &FPRReserveContract{
		address, conversionRateContract, r,
	}, nil
}

func (self *FPRReserveContract) QueryQtyStepFunc(token common.Address) (numSellSteps int, sellXs []float64, sellYs []float64, numBuySteps int, buyXs []float64, buyYs []float64, err error) {
	type qtyFunc struct {
		NumBuyRateQtySteps  *big.Int
		BuyRateQtyStepsX    []*big.Int
		BuyRateQtyStepsY    []*big.Int
		NumSellRateQtySteps *big.Int
		SellRateQtyStepsX   []*big.Int
		SellRateQtyStepsY   []*big.Int
	}
	res := &qtyFunc{}
	fmt.Printf("conversion rate contract: %s\n", self.ConversionRateContract.Hex())
	err = self.reader.ReadContract(
		res,
		"0x7FA7599413E53dED64b587cc5a607c384f600C66",
		"readQtyStepFunctions",
		self.ConversionRateContract,
		token,
	)
	if err != nil {
		return 0, []float64{}, []float64{}, 0, []float64{}, []float64{}, err
	}
	numSellSteps = int(res.NumSellRateQtySteps.Int64())
	numBuySteps = int(res.NumBuyRateQtySteps.Int64())
	decimal, err := self.reader.ERC20Decimal(token.Hex())
	if err != nil {
		return 0, []float64{}, []float64{}, 0, []float64{}, []float64{}, err
	}
	sellXs = []float64{}
	for _, x := range res.SellRateQtyStepsX {
		sellXs = append(sellXs, ethutils.BigToFloat(x, decimal))
	}
	buyXs = []float64{}
	for _, x := range res.BuyRateQtyStepsX {
		buyXs = append(buyXs, ethutils.BigToFloat(x, decimal))
	}
	sellYs = []float64{}
	for _, y := range res.SellRateQtyStepsY {
		sellYs = append(sellYs, ethutils.BigToFloat(y, 2))
	}

	buyYs = []float64{}
	for _, y := range res.BuyRateQtyStepsY {
		buyYs = append(buyYs, ethutils.BigToFloat(y, 2))
	}
	return numSellSteps, sellXs, sellYs, numBuySteps, buyXs, buyYs, nil
}

func (self *FPRReserveContract) QueryImbalanceStepFunc(token common.Address) (numSellSteps int, sellXs []float64, sellYs []float64, numBuySteps int, buyXs []float64, buyYs []float64, err error) {
	type imbFunc struct {
		NumBuyRateImbalanceSteps  *big.Int
		BuyRateImbalanceStepsX    []*big.Int
		BuyRateImbalanceStepsY    []*big.Int
		NumSellRateImbalanceSteps *big.Int
		SellRateImbalanceStepsX   []*big.Int
		SellRateImbalanceStepsY   []*big.Int
	}
	res := &imbFunc{}
	fmt.Printf("conversion rate contract: %s\n", self.ConversionRateContract.Hex())
	err = self.reader.ReadContract(
		res,
		"0x7FA7599413E53dED64b587cc5a607c384f600C66",
		"readImbalanceStepFunctions",
		self.ConversionRateContract,
		token,
	)
	if err != nil {
		return 0, []float64{}, []float64{}, 0, []float64{}, []float64{}, err
	}
	numSellSteps = int(res.NumSellRateImbalanceSteps.Int64())
	numBuySteps = int(res.NumBuyRateImbalanceSteps.Int64())
	decimal, err := self.reader.ERC20Decimal(token.Hex())
	if err != nil {
		return 0, []float64{}, []float64{}, 0, []float64{}, []float64{}, err
	}
	sellXs = []float64{}
	for _, x := range res.SellRateImbalanceStepsX {
		sellXs = append(sellXs, ethutils.BigToFloat(x, decimal))
	}
	buyXs = []float64{}
	for _, x := range res.BuyRateImbalanceStepsX {
		buyXs = append(buyXs, ethutils.BigToFloat(x, decimal))
	}
	sellYs = []float64{}
	for _, y := range res.SellRateImbalanceStepsY {
		sellYs = append(sellYs, ethutils.BigToFloat(y, 2))
	}

	buyYs = []float64{}
	for _, y := range res.BuyRateImbalanceStepsY {
		buyYs = append(buyYs, ethutils.BigToFloat(y, 2))
	}
	return numSellSteps, sellXs, sellYs, numBuySteps, buyXs, buyYs, nil
}

func (self *FPRReserveContract) DisplayStepFunctionData(token string) error {
	err := self.DisplayImbalanceStepFunc(token)
	if err != nil {
		return err
	}
	return self.DisplayQtyStepFunc(token)
}

func (self *FPRReserveContract) DisplayImbalanceStepFunc(token string) error {
	numSellSteps, sellXs, sellYs, numBuySteps, buyXs, buyYs, err := self.QueryImbalanceStepFunc(ethutils.HexToAddress(token))
	if err != nil {
		return err
	}
	// 	fmt.Printf("token qty (token)      : ")
	fmt.Printf("token imbalance (token): ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellXs[i])
	}
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyXs[i])
	}
	fmt.Printf("\n")
	fmt.Printf("slippage (%%)           : ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellYs[i])
	}
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyYs[i])
	}
	fmt.Printf("\n")
	return nil
}

func (self *FPRReserveContract) DisplayQtyStepFunc(token string) error {
	numSellSteps, sellXs, sellYs, numBuySteps, buyXs, buyYs, err := self.QueryQtyStepFunc(ethutils.HexToAddress(token))
	if err != nil {
		return err
	}
	fmt.Printf("token qty (token)      : ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellXs[i])
	}
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyXs[i])
	}
	fmt.Printf("\n")
	fmt.Printf("slippage (%%)           : ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellYs[i])
	}
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyYs[i])
	}
	fmt.Printf("\n")
	return nil
}
