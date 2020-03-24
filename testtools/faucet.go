package testtools

import (
	"context"
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// NewFaucet creates faucet object, requires client and private key.
func NewFaucet(client *ethclient.Client, pkey *ecdsa.PrivateKey) Faucet {
	return Faucet{
		pkey:    pkey,
		address: crypto.PubkeyToAddress(pkey.PublicKey),
		signer:  types.NewEIP155Signer(new(big.Int).SetUint64(NetworkID)),
		client:  client,
	}
}

// Faucet provides API to request funds.
type Faucet struct {
	pkey    *ecdsa.PrivateKey
	address common.Address
	signer  types.Signer
	client  *ethclient.Client
}

// Request funds for an address. Context will be passed to all internal network calls.
// Funding transaction is signed but not sent to the ethereum network.
func (f Faucet) Request(ctx context.Context, to common.Address, funds *big.Int) (*types.Transaction, error) {
	nonce, err := f.client.PendingNonceAt(ctx, f.address)
	if err != nil {
		return nil, err
	}
	price, err := f.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	tx := types.NewTransaction(nonce, to, funds, ETHTransferGas, price, nil)
	tx, err = types.SignTx(tx, f.signer, f.pkey)
	if err != nil {
		return nil, err
	}
	return tx, f.client.SendTransaction(ctx, tx)
}
