-- +migrate Up

-- 用户充值申请（待管理员审核通过后入账）
CREATE TABLE IF NOT EXISTS `wallet_recharge_application` (
  id              BIGINT        NOT NULL PRIMARY KEY AUTO_INCREMENT,
  application_no  VARCHAR(40)   NOT NULL DEFAULT '',
  uid             VARCHAR(40)   NOT NULL DEFAULT '',
  amount          DECIMAL(12,2) NOT NULL DEFAULT 0.00,
  channel_id      BIGINT        NOT NULL DEFAULT 0 COMMENT 'recharge_channel.id，0 表示未选渠道',
  pay_type        TINYINT       NOT NULL DEFAULT 0 COMMENT '2支付宝3微信4U盾，与渠道一致',
  remark          VARCHAR(500)  NOT NULL DEFAULT '' COMMENT '用户备注',
  proof_url       VARCHAR(500)  NOT NULL DEFAULT '' COMMENT '凭证图URL，可选',
  status          TINYINT       NOT NULL DEFAULT 0 COMMENT '0待审核 1已通过 2已拒绝',
  admin_remark    VARCHAR(500)  NOT NULL DEFAULT '',
  reviewer_uid    VARCHAR(40)   NOT NULL DEFAULT '',
  created_at      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX `wallet_recharge_app_uid` ON `wallet_recharge_application` (`uid`);
CREATE INDEX `wallet_recharge_app_status` ON `wallet_recharge_application` (`status`);
CREATE UNIQUE INDEX `wallet_recharge_app_no` ON `wallet_recharge_application` (`application_no`);
