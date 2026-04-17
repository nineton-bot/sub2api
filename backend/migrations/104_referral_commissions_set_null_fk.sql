-- 104_referral_commissions_set_null_fk.sql
-- Align referral_commissions FK cascade policy with users.invited_by_user_id
-- (which is SET NULL in migration 103). Hard-deleting a user should not wipe
-- historical commission rows; they remain as audit trail with referrer_id /
-- referee_id nulled out.
--
-- referral_pending_bonuses is intentionally left with ON DELETE CASCADE: a
-- pending welcome bonus only has business meaning while the referee account
-- still exists; if the referee is removed, discarding the unrealised bonus is
-- the correct behaviour.
--
-- NOTE: migrations_runner 会自动包裹事务，本文件不要写 BEGIN/COMMIT。

ALTER TABLE referral_commissions
    ALTER COLUMN referrer_id DROP NOT NULL;

ALTER TABLE referral_commissions
    ALTER COLUMN referee_id DROP NOT NULL;

ALTER TABLE referral_commissions
    DROP CONSTRAINT IF EXISTS referral_commissions_referrer_id_fkey;

ALTER TABLE referral_commissions
    DROP CONSTRAINT IF EXISTS referral_commissions_referee_id_fkey;

ALTER TABLE referral_commissions
    ADD CONSTRAINT referral_commissions_referrer_id_fkey
        FOREIGN KEY (referrer_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE referral_commissions
    ADD CONSTRAINT referral_commissions_referee_id_fkey
        FOREIGN KEY (referee_id) REFERENCES users(id) ON DELETE SET NULL;
