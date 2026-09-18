package vet

import (
	"github.com/ethereum/go-ethereum/core/types"
)

// AuthorizationsFromTx copies a type-4 authorization_list into vet's
// hex-only Authorization records. Authority is recovered when the
// signature is valid.
func AuthorizationsFromTx(tx *types.Transaction) []Authorization {
	if tx == nil {
		return nil
	}
	return authorizationsFrom(tx.SetCodeAuthorizations())
}

func authorizationsFrom(auths []types.SetCodeAuthorization) []Authorization {
	if len(auths) == 0 {
		return nil
	}
	out := make([]Authorization, 0, len(auths))
	for _, a := range auths {
		item := Authorization{
			Address: a.Address.Hex(),
			ChainID: a.ChainID.Uint64(),
			Nonce:   a.Nonce,
		}
		if auth, err := a.Authority(); err == nil {
			item.Authority = auth.Hex()
		}
		out = append(out, item)
	}
	return out
}
