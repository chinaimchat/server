-- +migrate Up
ALTER TABLE app_config
  ADD COLUMN privilege_only_create_invite_group_on TINYINT NOT NULL DEFAULT 0 COMMENT '仅特权用户可建群与邀请成员' AFTER friend_apply_auto_accept_on;

-- +migrate Down
ALTER TABLE app_config
  DROP COLUMN privilege_only_create_invite_group_on;
