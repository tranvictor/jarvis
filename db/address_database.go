package db

import (
  "strings"
  "fmt"

  "github.com/tranvictor/ethutils/txanalyzer"
  "github.com/sahilm/fuzzy"
)

func GetAddress(input string) (AddressDesc, error) {
  source := NewFuzzySource()
  matches := fuzzy.FindFrom(strings.Replace(input, " ", "_", -1), source)
  if len(matches) == 0 {
    return AddressDesc{}, fmt.Errorf("No address is found with '%s'", input)
  }
  match := matches[0]
  return source[match.Index], nil
}

func GetTokenAddress(input string) (AddressDesc, error) {
  source := NewTokenFuzzySource()
  matches := fuzzy.FindFrom(strings.Replace(input, " ", "_", -1), source)
  if len(matches) == 0 {
    return AddressDesc{}, fmt.Errorf("No address is found with '%s'", input)
  }
  match := matches[0]
  return source[match.Index], nil
}

func AllTokenAddresses() map[string]string {
  result := map[string]string{}
  for addr, desc := range TOKENS {
    result[strings.ToLower(addr)] = desc
  }
  return result
}

func AllAddresses() map[string]string {
  addrs := txanalyzer.NewDefaultAddressDatabase().Data
  tokenAddrs := AllTokenAddresses()
  result := map[string]string{}
  for addr, desc := range addrs {
    result[strings.ToLower(addr.Hex())] = desc
  }
  for addr, desc := range tokenAddrs {
    result[addr] = desc
  }
  return result
}
