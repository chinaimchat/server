-- +migrate Up
SET @col_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'app_config'
    AND COLUMN_NAME = 'invite_code_system_on'
);
SET @up_sql := IF(
  @col_exists = 0,
  'ALTER TABLE `app_config` ADD COLUMN `invite_code_system_on` smallint NOT NULL DEFAULT 1 COMMENT ''邀请码系统总开关''',
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
    AND COLUMN_NAME = 'invite_code_system_on'
);
SET @down_sql := IF(
  @col_exists = 1,
  'ALTER TABLE `app_config` DROP COLUMN `invite_code_system_on`',
  'SELECT 1'
);
PREPARE down_stmt FROM @down_sql;
EXECUTE down_stmt;
DEALLOCATE PREPARE down_stmt;
