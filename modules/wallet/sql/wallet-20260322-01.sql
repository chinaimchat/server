-- +migrate Up

CREATE TABLE IF NOT EXISTS `redpacket_config` (
  id                          INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
  expire_hours                INTEGER NOT NULL DEFAULT 1,
  expire_check_interval_min   INTEGER NOT NULL DEFAULT 5,
  max_amount_per_redpacket    INTEGER NOT NULL DEFAULT 100000,
  max_daily_amount_per_user   INTEGER NOT NULL DEFAULT 1000000,
  max_redpackets_per_hour     INTEGER NOT NULL DEFAULT 1000,
  min_amount_per_redpacket    INTEGER NOT NULL DEFAULT 100,
  batch_expire_limit          INTEGER NOT NULL DEFAULT 100,
  enable_risk_control         TINYINT NOT NULL DEFAULT 0,
  enable_auto_expire          TINYINT NOT NULL DEFAULT 1,
  refund_on_expire            TINYINT NOT NULL DEFAULT 1,
  created_at                  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at                  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO `redpacket_config` (id) VALUES (1) ON DUPLICATE KEY UPDATE id=id;

CREATE TABLE IF NOT EXISTS `transfer_config` (
  id             INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  daily_limit    VARCHAR(50)  NOT NULL DEFAULT '1000000',
  expire_hours   VARCHAR(10)  NOT NULL DEFAULT '24',
  fee_rate       VARCHAR(10)  NOT NULL DEFAULT '0.00',
  max_amount     VARCHAR(50)  NOT NULL DEFAULT '500000',
  min_amount     VARCHAR(50)  NOT NULL DEFAULT '100',
  risk_threshold VARCHAR(50)  NOT NULL DEFAULT '100000',
  created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO `transfer_config` (id) VALUES (1) ON DUPLICATE KEY UPDATE id=id;
