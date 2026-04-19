-- +migrate Up

CREATE TABLE IF NOT EXISTS `aibot_config` (
  id          INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  enabled     TINYINT      NOT NULL DEFAULT 0,
  provider    VARCHAR(40)  NOT NULL DEFAULT 'deepseek',
  api_key     VARCHAR(500) NOT NULL DEFAULT '',
  model       VARCHAR(100) NOT NULL DEFAULT 'deepseek-chat',
  max_tokens  INTEGER      NOT NULL DEFAULT 2000,
  temperature DECIMAL(3,2) NOT NULL DEFAULT 0.70,
  system_uid  VARCHAR(40)  NOT NULL DEFAULT 'u_10000',
  created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO `aibot_config` (id, provider, model, system_uid) VALUES (1, 'deepseek', 'deepseek-chat', 'u_10000') ON DUPLICATE KEY UPDATE id=id;

CREATE TABLE IF NOT EXISTS `web3_laboratory` (
  id         INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  short_url  VARCHAR(100) NOT NULL DEFAULT '',
  url        VARCHAR(1000) NOT NULL DEFAULT '',
  status     TINYINT      NOT NULL DEFAULT 1 COMMENT '1启用 0禁用',
  remark     VARCHAR(500) NOT NULL DEFAULT '',
  created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
