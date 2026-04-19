-- +migrate Up

-- 红包风控规则表
CREATE TABLE IF NOT EXISTS `redpacket_risk_rule` (
  id           INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  name         VARCHAR(100) NOT NULL DEFAULT '',
  type         VARCHAR(40)  NOT NULL DEFAULT 'amount_limit',
  threshold    DECIMAL(12,2) NOT NULL DEFAULT 0,
  time_window  INTEGER      NOT NULL DEFAULT 3600,
  description  VARCHAR(500) NOT NULL DEFAULT '',
  enabled      TINYINT      NOT NULL DEFAULT 1,
  sort_order   INTEGER      NOT NULL DEFAULT 0,
  created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 红包风控事件表
CREATE TABLE IF NOT EXISTS `redpacket_risk_event` (
  id           INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  uid          VARCHAR(40)  NOT NULL DEFAULT '',
  event_type   VARCHAR(40)  NOT NULL DEFAULT '',
  risk_level   VARCHAR(20)  NOT NULL DEFAULT 'low',
  remark       VARCHAR(500) NOT NULL DEFAULT '',
  status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
  handler      VARCHAR(40)  NOT NULL DEFAULT '',
  handle_remark VARCHAR(500) NOT NULL DEFAULT '',
  created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE `redpacket` ADD COLUMN `scene_type` SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE `redpacket` ADD COLUMN `creater_name` VARCHAR(40) NOT NULL DEFAULT '';
ALTER TABLE `redpacket_record` ADD COLUMN `receiver_name` VARCHAR(40) NOT NULL DEFAULT '';
