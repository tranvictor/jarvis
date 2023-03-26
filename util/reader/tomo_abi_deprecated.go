package reader

// gas station response
// type tomoabiresponse struct {
// 	Contract struct {
// 		ABICode string `json:"abiCode"`
// 	} `json:"contract"`
// }

// func (self *EthReader) GetTomoABIString(address string) (string, error) {
// 	resp, err := http.Get(fmt.Sprintf("https://scan.tomochain.com/api/accounts/%s", address))
// 	if err != nil {
// 		return "", err
// 	}
// 	defer resp.Body.Close()
// 	body, err := ioutil.ReadAll(resp.Body)
// 	abiresp := tomoabiresponse{}
// 	err = json.Unmarshal(body, &abiresp)
// 	if err != nil {
// 		return "", err
// 	}
// 	return abiresp.Contract.ABICode, nil
// }

// func (self *EthReader) GetTomoABI(address string) (*abi.ABI, error) {
// 	body, err := self.GetTomoABIString(address)
// 	if err != nil {
// 		return nil, err
// 	}
// 	result, err := abi.JSON(strings.NewReader(body))
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &result, nil
// }
