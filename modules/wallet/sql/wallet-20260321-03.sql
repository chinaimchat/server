-- +migrate Up

-- 操作日志表增加字段以匹配远程站点
ALTER TABLE `wallet_operation_log` ADD COLUMN `operation_desc` VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE `wallet_operation_log` ADD COLUMN `ip_address` VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE `wallet_operation_log` ADD COLUMN `user_agent` TEXT;
ALTER TABLE `wallet_operation_log` ADD COLUMN `error_msg` TEXT;
ALTER TABLE `wallet_operation_log` ADD COLUMN `operation_data` TEXT;
ALTER TABLE `wallet_operation_log` ADD COLUMN `target_info` VARCHAR(200) NOT NULL DEFAULT '';

-- 交易流水表增加余额快照字段
ALTER TABLE `wallet_transaction` ADD COLUMN `balance_after` DECIMAL(12,2) NOT NULL DEFAULT 0.00;
