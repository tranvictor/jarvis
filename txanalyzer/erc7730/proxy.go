package erc7730

// Proxy resolution lives in the caller's hands so this package stays
// free of network dependencies (and easy to test). The public Engine
// interface accepts an ImplFor func that the production wiring
// supplies; tests pass nil for offline operation.
//
// The function signature matches ContractMatchInput.ImplFor:
// given a proxy address, return the implementation address and
// whether the lookup succeeded. The default jarvis wiring (in
// clearsign.go) hooks this to util.EthReader.ImplementationOf which
// already understands EIP-1967, ZeppelinOS and Polygon's beacon
// variants.
