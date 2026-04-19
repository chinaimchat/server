-- +migrate Up

CREATE TABLE IF NOT EXISTS `recharge_channel` (
  id          INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  app_id      VARCHAR(40)  NOT NULL DEFAULT 'tsdd_app',
  pay_type    TINYINT      NOT NULL DEFAULT 2 COMMENT '2支付宝 3微信 4U盾',
  icon        VARCHAR(500) NOT NULL DEFAULT '',
  install_key VARCHAR(500) NOT NULL DEFAULT '',
  status      TINYINT      NOT NULL DEFAULT 1 COMMENT '1启用 0禁用',
  title       VARCHAR(100) NOT NULL DEFAULT '',
  remark      VARCHAR(200) NOT NULL DEFAULT '',
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS `udun_config` (
  id           INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  base_url     VARCHAR(500) NOT NULL DEFAULT '',
  merchant_id  VARCHAR(100) NOT NULL DEFAULT '',
  sign_key     VARCHAR(200) NOT NULL DEFAULT '',
  callback_url VARCHAR(500) NOT NULL DEFAULT '',
  timeout      INTEGER      NOT NULL DEFAULT 30,
  status       TINYINT      NOT NULL DEFAULT 1,
  created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO `udun_config` (base_url, merchant_id, sign_key, callback_url) VALUES ('', '', '', '') ON DUPLICATE KEY UPDATE id=id;

CREATE TABLE IF NOT EXISTS `udun_coin_type` (
  id          INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  symbol      VARCHAR(20)  NOT NULL DEFAULT '',
  coin_name   VARCHAR(50)  NOT NULL DEFAULT '',
  name        VARCHAR(100) NOT NULL DEFAULT '',
  main_symbol VARCHAR(20)  NOT NULL DEFAULT '',
  decimals    VARCHAR(10)  NOT NULL DEFAULT '',
  min_trade   VARCHAR(50)  NOT NULL DEFAULT '0',
  status      TINYINT      NOT NULL DEFAULT 1,
  tips        VARCHAR(500) NOT NULL DEFAULT '',
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
