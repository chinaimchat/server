-- +migrate Up
-- 提现手续费：比例(%) + 每笔固定金额；申请时冻结 amount + fee

CREATE TABLE IF NOT EXISTS `withdrawal_config` (
  `id`         INT          NOT NULL PRIMARY KEY COMMENT '固定为 1',
  `fee_rate`   VARCHAR(20)  NOT NULL DEFAULT '0' COMMENT '按提现金额的比例，单位：百分比，如 1 表示 1%%',
  `fee_fixed`  VARCHAR(20)  NOT NULL DEFAULT '0' COMMENT '每笔固定加收，与余额同单位',
  `updated_at` TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

INSERT INTO `withdrawal_config` (`id`, `fee_rate`, `fee_fixed`) VALUES (1, '0', '0')
ON DUPLICATE KEY UPDATE `id` = `id`;
