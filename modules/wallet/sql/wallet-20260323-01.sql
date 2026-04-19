-- +migrate Up

-- 充值申请：U 盾场景记录用户填写的 U 数量与所用汇率（渠道 install_key）
ALTER TABLE `wallet_recharge_application` ADD COLUMN `amount_u` DECIMAL(20,8) NOT NULL DEFAULT 0 COMMENT 'U盾时用户提交的U数量';
ALTER TABLE `wallet_recharge_application` ADD COLUMN `exchange_rate` DECIMAL(20,8) NOT NULL DEFAULT 0 COMMENT '所用汇率(来自渠道install_key)';
