-- +migrate Up

-- 钱包增加状态和用户信息字段
ALTER TABLE `wallet` ADD COLUMN `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1正常 2冻结';
ALTER TABLE `wallet` ADD COLUMN `phone` VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE `wallet` ADD COLUMN `zone` VARCHAR(10) NOT NULL DEFAULT '';

-- 提现记录
CREATE TABLE `wallet_withdrawal` (
  id            INTEGER       NOT NULL PRIMARY KEY AUTO_INCREMENT,
  withdrawal_no VARCHAR(40)   NOT NULL DEFAULT '',
  uid           VARCHAR(40)   NOT NULL DEFAULT '',
  amount        DECIMAL(12,2) NOT NULL DEFAULT 0.00,
  fee           DECIMAL(12,2) NOT NULL DEFAULT 0.00,
  status        TINYINT       NOT NULL DEFAULT 0 COMMENT '0待审核 1已批准 2已拒绝 3已完成',
  address       VARCHAR(200)  NOT NULL DEFAULT '',
  remark        VARCHAR(200)  NOT NULL DEFAULT '',
  admin_remark  VARCHAR(200)  NOT NULL DEFAULT '',
  created_at    TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX wallet_withdrawal_uid ON `wallet_withdrawal` (uid);
CREATE INDEX wallet_withdrawal_status ON `wallet_withdrawal` (status);

-- 管理员操作日志
CREATE TABLE `wallet_operation_log` (
  id          INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  operator    VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '操作员',
  action      VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '操作类型',
  target_uid  VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '目标用户',
  amount      DECIMAL(12,2) NOT NULL DEFAULT 0.00,
  reason      VARCHAR(500) NOT NULL DEFAULT '',
  result      VARCHAR(20)  NOT NULL DEFAULT 'success',
  detail      TEXT,
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX wallet_oplog_operator ON `wallet_operation_log` (operator);
CREATE INDEX wallet_oplog_action ON `wallet_operation_log` (action);

-- 客服联系方式
CREATE TABLE `wallet_customer_service` (
  id         INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  name       VARCHAR(40)  NOT NULL DEFAULT '',
  uid        VARCHAR(40)  NOT NULL DEFAULT '',
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
