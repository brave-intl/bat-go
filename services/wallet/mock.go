package wallet

import "context"

type mockSolAddrsChecker struct {
	fnIsAllowed func(ctx context.Context, addrs string) error
}

func (c *mockSolAddrsChecker) IsAllowed(ctx context.Context, addrs string) error {
	if c.fnIsAllowed == nil {
		return nil
	}

	return c.fnIsAllowed(ctx, addrs)
}
