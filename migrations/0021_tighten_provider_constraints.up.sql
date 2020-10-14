ALTER TABLE wallets DROP CONSTRAINT check_provider;
update wallets set provider='brave' where provider='client';
ALTER TABLE wallets ADD CONSTRAINT check_provider CHECK (provider IN ('uphold', 'brave'));
