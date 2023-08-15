ALTER TABLE wallet_custodian
DROP CONSTRAINT IF EXISTS check_custodian,
ADD CONSTRAINT check_custodian CHECK (
    custodian IN ('brave', 'uphold', 'bitflyer', 'gemini', 'xyzabc')
);
