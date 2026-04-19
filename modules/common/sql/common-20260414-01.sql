-- +migrate Up
ALTER TABLE app_config
  ADD COLUMN privilege_only_add_friend_on TINYINT NOT NULL DEFAULT 0 COMMENT '仅特权用户可搜索添加好友' AFTER search_by_phone,
  ADD COLUMN friend_apply_auto_accept_on TINYINT NOT NULL DEFAULT 0 COMMENT '添加好友免验证' AFTER privilege_only_add_friend_on;

-- +migrate Down
ALTER TABLE app_config
  DROP COLUMN friend_apply_auto_accept_on,
  DROP COLUMN privilege_only_add_friend_on;
