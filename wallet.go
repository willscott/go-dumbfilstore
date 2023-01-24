package main

import (
	"context"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	ctypes "github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/chain/wallet/key"
)

func (sh *StorageHandler) WalletBalance(context.Context, address.Address) (types.BigInt, error) {
	return types.NewInt(0), nil
}

func makeWallet(path string) error {
	localKey, _ := key.GenerateKey(ctypes.KTSecp256k1)
	return os.WriteFile(path, localKey.KeyInfo.PrivateKey, 0600)
}

func loadWallet(path string) (api.Wallet, error) {
	pk, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	wk := wallet.NewMemKeyStore()
	wapi, _ := wallet.NewWallet(wk)
	wapi.WalletImport(context.Background(), &ctypes.KeyInfo{
		Type:       ctypes.KTSecp256k1,
		PrivateKey: pk,
	})
	return wapi, nil
}
