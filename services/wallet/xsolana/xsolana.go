package xsolana

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type Client struct {
	rpc *rpc.Client
}

func New(endpoint string) *Client {
	return &Client{
		rpc: rpc.New(endpoint),
	}
}

func (c *Client) GetTokenAccountsByOwner(ctx context.Context, owner solana.PublicKey, mint solana.PublicKey) (*rpc.GetTokenAccountsResult, error) {
	arg := &rpc.GetTokenAccountsConfig{Mint: &mint}

	return c.rpc.GetTokenAccountsByOwner(ctx, owner, arg, nil)
}
