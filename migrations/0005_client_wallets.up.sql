ALTER TABLE wallets DROP CONSTRAINT check_provider;
ALTER TABLE wallets ADD CONSTRAINT check_provider CHECK (provider in ('uphold', 'client'));
