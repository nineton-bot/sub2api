-- 133b prelude: 一次性把生产库 schema_migrations 里旧编号的私有 referral 迁移
-- 文件名升号到 134-137，避开 upstream 在 103/104/105/106 已占用的编号。
--
-- 命名设计：133b 在 sort.Strings() 字典序下满足
--     133_*.sql < 133b_renumber_*.sql < 134_*.sql
-- migrations_runner.go 按字典序扫，所以这条 UPDATE 一定先于 134-137 文件被 apply。
-- 沿袭 upstream 已有的 letter-suffix 模式（108a / 120a）。
--
-- 文件内容不变，sha256 checksum 不变，runner 后续扫到 134-137 时会因
-- 「schema_migrations 已有该文件名记录 + checksum 一致」而跳过 apply，不会重复
-- 建表。
--
-- 新环境（schema_migrations 不含旧记录）下这 4 条 UPDATE 影响 0 行，无害。
--
-- 升号原因：upstream/main 已占用 103-106 编号（103_add_allow_user_refund /
-- 104_migrate_notify_emails_to_struct / 105_migrate_websearch_emulation_to_tristate /
-- 106_add_account_stats_pricing_intervals），与本仓库 referral V2 系列冲突。

UPDATE schema_migrations
   SET filename = '134_add_referral_system.sql'
 WHERE filename = '103_add_referral_system.sql';

UPDATE schema_migrations
   SET filename = '135_referral_commissions_set_null_fk.sql'
 WHERE filename = '104_referral_commissions_set_null_fk.sql';

UPDATE schema_migrations
   SET filename = '136_referral_usable_and_withdrawals.sql'
 WHERE filename = '105_referral_usable_and_withdrawals.sql';

UPDATE schema_migrations
   SET filename = '137_add_group_config_template.sql'
 WHERE filename = '106_add_group_config_template.sql';
