package bleve

import (
	"fmt"
)

type AddressDesc struct {
	Address      string
	Desc         string
	SearchString string
}

func GetAddress(input string) (AddressDesc, error) {
	results, _ := GetAddresses(input)
	if len(results) == 0 {
		return AddressDesc{}, fmt.Errorf("couldn't find address for: %s", input)
	}
	return results[0], nil
}

func GetAddresses(input string) ([]AddressDesc, []int) {
	db, err := NewBleveDB()
	if err != nil {
		fmt.Printf("Getting address db failed: %s\n", err)
		return []AddressDesc{}, []int{}
	}
	return db.Search(input)
}

// func GetTokenAddress(input string) (AddressDesc, error) {
// 	return AddressDesc{}, fmt.Errorf("unimplemented")
// }
