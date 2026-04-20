-- +migrate Up

-- 群头像版本号：每次群头像被更换 bump 一次（毫秒时间戳）。
-- 客户端拼到群头像 URL 的 ?v= 后面，绕过 Glide / SDWebImage / OkHttp / 浏览器多层缓存。
ALTER TABLE `group` ADD COLUMN `avatar_update_at` BIGINT NOT NULL DEFAULT 0 COMMENT '群头像更新时间(ms)，每次上传群头像 bump，客户端用作 ?v= 缓存破坏';

-- +migrate Down
ALTER TABLE `group` DROP COLUMN `avatar_update_at`;
