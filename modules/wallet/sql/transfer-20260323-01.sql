-- +migrate Up

-- 转账发起时的会话：群聊则进对应群，单聊进双方会话（与红包提示一致）
ALTER TABLE `transfer` ADD COLUMN `channel_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '单聊可为空由服务端推导；群聊为群编号';
ALTER TABLE `transfer` ADD COLUMN `channel_type` TINYINT NOT NULL DEFAULT 1 COMMENT '1单聊 2群聊，同 common.ChannelType';
