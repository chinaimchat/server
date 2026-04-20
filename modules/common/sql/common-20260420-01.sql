-- +migrate Up
-- 邀请码系统采用单一总开关 invite_code_system_on，废弃 register_invite_on；删除该列。
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'app_config'
    AND COLUMN_NAME = 'register_invite_on'
);
SET @up_sql := IF(
  @col_exists = 1,
  'ALTER TABLE `app_config` DROP COLUMN `register_invite_on`',
  'SELECT 1'
);
PREPARE up_stmt FROM @up_sql;
EXECUTE up_stmt;
DEALLOCATE PREPARE up_stmt;

-- +migrate Down
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'app_config'
    AND COLUMN_NAME = 'register_invite_on'
);
SET @down_sql := IF(
  @col_exists = 0,
  'ALTER TABLE `app_config` ADD COLUMN `register_invite_on` smallint NOT NULL DEFAULT 0 COMMENT ''已废弃：原注册邀请开关，已并入 invite_code_system_on''',
  'SELECT 1'
);
PREPARE down_stmt FROM @down_sql;
EXECUTE down_stmt;
DEALLOCATE PREPARE down_stmt;
