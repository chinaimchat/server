-- +migrate Up
CREATE TABLE IF NOT EXISTS user_privilege (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  uid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '用户UID',
  group_manage_on TINYINT NOT NULL DEFAULT 1 COMMENT '群管理开关',
  all_member_invite_on TINYINT NOT NULL DEFAULT 0 COMMENT '全员拉群开关',
  mutual_delete_person_on TINYINT NOT NULL DEFAULT 1 COMMENT '单聊双删开关',
  mutual_delete_group_on TINYINT NOT NULL DEFAULT 1 COMMENT '群聊双删开关',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY uk_uid (uid),
  KEY idx_created_at (created_at)
) CHARACTER SET utf8mb4;

-- +migrate Down
DROP TABLE IF EXISTS user_privilege;
