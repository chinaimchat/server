-- +migrate Up
-- 充值渠道：二维码 URL、本地上传二维码图、收款地址
-- 列已存在时跳过，避免重复执行迁移报错（Duplicate column）

SET @db = DATABASE();

SET @exists_qr_url = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = @db AND table_name = 'recharge_channel' AND column_name = 'qr_url'
);
SET @sql_qr_url = IF(@exists_qr_url = 0,
  'ALTER TABLE `recharge_channel` ADD COLUMN `qr_url` VARCHAR(1000) NOT NULL DEFAULT '''' COMMENT ''二维码图片URL'' AFTER `icon`',
  'SELECT 1');
PREPARE stmt_common_qr_url FROM @sql_qr_url;
EXECUTE stmt_common_qr_url;
DEALLOCATE PREPARE stmt_common_qr_url;

SET @exists_qr_image_url = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = @db AND table_name = 'recharge_channel' AND column_name = 'qr_image_url'
);
SET @sql_qr_image_url = IF(@exists_qr_image_url = 0,
  'ALTER TABLE `recharge_channel` ADD COLUMN `qr_image_url` VARCHAR(1000) NOT NULL DEFAULT '''' COMMENT ''上传的二维码图片URL'' AFTER `qr_url`',
  'SELECT 1');
PREPARE stmt_common_qr_image FROM @sql_qr_image_url;
EXECUTE stmt_common_qr_image;
DEALLOCATE PREPARE stmt_common_qr_image;

SET @exists_pay_address = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = @db AND table_name = 'recharge_channel' AND column_name = 'pay_address'
);
SET @sql_pay_address = IF(@exists_pay_address = 0,
  'ALTER TABLE `recharge_channel` ADD COLUMN `pay_address` VARCHAR(500) NOT NULL DEFAULT '''' COMMENT ''收款地址'' AFTER `qr_image_url`',
  'SELECT 1');
PREPARE stmt_common_pay FROM @sql_pay_address;
EXECUTE stmt_common_pay;
DEALLOCATE PREPARE stmt_common_pay;
