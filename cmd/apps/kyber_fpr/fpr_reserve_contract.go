package kyberfpr

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
)

const RATE_WRAPPER_CONTRACT = "0x3F0d4A4363d08Cd625285965832C4BA53b5A718A"

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
	fmt.Printf("Conversion rate contract: %s\n", conversionRateContract.Hex())

	return &FPRReserveContract{
		address, conversionRateContract, r,
	}, nil
}

func (self *FPRReserveContract) QueryListedTokens() ([]common.Address, error) {
	res := []common.Address{}
	abi, err := util.GetABI(self.ConversionRateContract.Hex(), config.Network)
	if err != nil {
		return nil, err
	}
	err = self.reader.ReadHistoryContractWithABI(
		config.AtBlock,
		&res,
		self.ConversionRateContract.Hex(),
		abi,
		"getListedTokens",
	)
	return res, err
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
	err = self.reader.ReadHistoryContract(
		config.AtBlock,
		res,
		RATE_WRAPPER_CONTRACT,
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

func (self *FPRReserveContract) GetTokenControlInfo(token string) (*big.Int, *big.Int, *big.Int, error) {
	result := [3]*big.Int{nil, nil, nil}
	abi, err := util.GetABI(self.ConversionRateContract.Hex(), config.Network)
	if err != nil {
		return result[0], result[1], result[2], err
	}
	err = self.reader.ReadHistoryContractWithABI(
		config.AtBlock,
		&result,
		self.ConversionRateContract.Hex(),
		abi,
		"getTokenControlInfo",
		ethutils.HexToAddress(token),
	)
	return result[0], result[1], result[2], err
}

func (self *FPRReserveContract) GetBasicInfo(token string) (bool, bool, error) {
	result := [2]bool{false, false}
	abi, err := util.GetABI(self.ConversionRateContract.Hex(), config.Network)
	if err != nil {
		return result[0], result[1], err
	}
	err = self.reader.ReadHistoryContractWithABI(
		config.AtBlock,
		&result,
		self.ConversionRateContract.Hex(),
		abi,
		"getTokenBasicData",
		ethutils.HexToAddress(token),
	)
	return result[0], result[1], err
}

func (self *FPRReserveContract) QueryImbalanceStepFunc(token common.Address) (numSellStepsX int, sellXs []float64, numSellStepsY int, sellYs []float64, numBuyStepsX int, buyXs []float64, numBuyStepsY int, buyYs []float64, err error) {
	type imbFunc struct {
		NumBuyRateImbalanceStepsX  *big.Int
		BuyRateImbalanceStepsX     []*big.Int
		NumBuyRateImbalanceStepsY  *big.Int
		BuyRateImbalanceStepsY     []*big.Int
		NumSellRateImbalanceStepsX *big.Int
		SellRateImbalanceStepsX    []*big.Int
		NumSellRateImbalanceStepsY *big.Int
		SellRateImbalanceStepsY    []*big.Int
	}
	res := &imbFunc{}
	err = self.reader.ReadHistoryContract(
		config.AtBlock,
		res,
		RATE_WRAPPER_CONTRACT,
		"readImbalanceStepFunctions",
		self.ConversionRateContract,
		token,
	)
	if err != nil {
		return 0, []float64{}, 0, []float64{}, 0, []float64{}, 0, []float64{}, err
	}
	numSellStepsX = int(res.NumSellRateImbalanceStepsX.Int64())
	numSellStepsY = int(res.NumSellRateImbalanceStepsY.Int64())
	numBuyStepsX = int(res.NumBuyRateImbalanceStepsX.Int64())
	numBuyStepsY = int(res.NumBuyRateImbalanceStepsY.Int64())
	decimal, err := self.reader.ERC20Decimal(token.Hex())
	if err != nil {
		return 0, []float64{}, 0, []float64{}, 0, []float64{}, 0, []float64{}, err
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
	return numSellStepsX, sellXs, numSellStepsY, sellYs, numBuyStepsX, buyXs, numBuyStepsY, buyYs, nil
}

func (self *FPRReserveContract) DisplayStepFunctionData(token string, price float64) error {
	fmt.Printf("\nImbalance step functions:\n")
	return self.DisplayImbalanceStepFunc(token, price)
	// if err != nil {
	// 	return err
	// }
	// fmt.Printf("\nQty step functions:\n")
	// return self.DisplayQtyStepFunc(token)
}

func (self *FPRReserveContract) DisplayImbalanceStepFunc(token string, price float64) error {
	numSellStepsX, sellXs, numSellStepsY, sellYs, numBuyStepsX, buyXs, numBuyStepsY, buyYs, err := self.QueryImbalanceStepFunc(ethutils.HexToAddress(token))
	if err != nil {
		return err
	}
	// displaying imbalance in token
	fmt.Printf("imb($): ")
	fmt.Printf("%5s|", "-INF")
	for i := 0; i < numSellStepsX; i++ {
		fmt.Printf("%10.1f|", sellXs[i])
	}
	fmt.Printf("%10.1f|", 0.0)
	for i := 0; i < numBuyStepsX; i++ {
		fmt.Printf("%10.1f|", buyXs[i])
	}
	fmt.Printf("%10s", "+INF")
	// displaying imbalance in USD
	fmt.Printf("\n")
	fmt.Printf("imb  :  ")
	fmt.Printf("%5s|", "-INF")
	for i := 0; i < numSellStepsX; i++ {
		fmt.Printf("%10.1f|", sellXs[i]*price)
	}
	fmt.Printf("%10.1f|", 0.0)
	for i := 0; i < numBuyStepsX; i++ {
		fmt.Printf("%10.1f|", buyXs[i]*price)
	}
	fmt.Printf("%10s", "+INF")
	// display slippage
	fmt.Printf("\n")
	fmt.Printf("y (%%): ")
	for i := 0; i < numSellStepsY; i++ {
		fmt.Printf("%11.2f", sellYs[i])
	}
	for i := 0; i < numBuyStepsY; i++ {
		fmt.Printf("%11.2f", buyYs[i])
	}
	fmt.Printf("\n")
	return nil
}

func (self *FPRReserveContract) DisplayQtyStepFunc(token string) error {
	numSellSteps, sellXs, sellYs, numBuySteps, buyXs, buyYs, err := self.QueryQtyStepFunc(ethutils.HexToAddress(token))
	if err != nil {
		return err
	}
	fmt.Printf("SELL: token qty (token)      : ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellXs[i])
	}
	fmt.Printf("\n")
	fmt.Printf("SELL: slippage (%%)           : ")
	for i := 0; i < numSellSteps; i++ {
		fmt.Printf("%10.2f|", sellYs[i])
	}
	fmt.Printf("\n")
	fmt.Printf("BUY : token qty (token)      : ")
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyXs[i])
	}
	fmt.Printf("\n")
	fmt.Printf("BUY : slippage (%%)           : ")
	for i := 0; i < numBuySteps; i++ {
		fmt.Printf("%10.2f|", buyYs[i])
	}
	fmt.Printf("\n")
	return nil
}
