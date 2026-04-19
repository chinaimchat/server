-- +migrate Up

CREATE TABLE IF NOT EXISTS `invite_code` (
  id            INTEGER      NOT NULL PRIMARY KEY AUTO_INCREMENT,
  invite_code   VARCHAR(20)  NOT NULL DEFAULT '',
  uid           VARCHAR(40)  NOT NULL DEFAULT '',
  max_use_count INTEGER      NOT NULL DEFAULT 0,
  used_count    INTEGER      NOT NULL DEFAULT 0,
  expire_at     BIGINT       NOT NULL DEFAULT 0,
  status        TINYINT      NOT NULL DEFAULT 1 COMMENT '1启用 0禁用',
  remark        VARCHAR(200) NOT NULL DEFAULT '',
  created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX invite_code_code ON `invite_code` (invite_code);
