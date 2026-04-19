-- +migrate Up
ALTER TABLE invite_code
  ADD COLUMN name VARCHAR(100) NOT NULL DEFAULT '' AFTER invite_code,
  ADD COLUMN groups_json TEXT NULL AFTER remark,
  ADD COLUMN friends_json TEXT NULL AFTER groups_json,
  ADD COLUMN system_welcome TEXT NULL AFTER friends_json;

-- +migrate Down
ALTER TABLE invite_code
  DROP COLUMN system_welcome,
  DROP COLUMN friends_json,
  DROP COLUMN groups_json,
  DROP COLUMN name;
