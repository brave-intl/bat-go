-- fkey constraints on claims not needed
ALTER TABLE claims DROP CONSTRAINT claims_promotion_id_fkey;
ALTER TABLE claims DROP CONSTRAINT claims_wallet_id_fkey;
