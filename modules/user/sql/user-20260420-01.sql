-- +migrate Up

-- 头像版本号：每次上传/更换头像 bump 一次（毫秒时间戳）。
-- 客户端把这个值拼到头像 URL 的 ?v= 后面，绕过 Glide / SDWebImage / OkHttp / 浏览器
-- 多层缓存，避免对方更新头像后本端因为 URL 不变一直命中老缓存（包括离线漏掉
-- wk_userAvatarUpdate CMD 的场景）。
ALTER TABLE `user` ADD COLUMN `avatar_update_at` BIGINT NOT NULL DEFAULT 0 COMMENT '头像更新时间(ms)，每次上传头像 bump，客户端用作 ?v= 缓存破坏';

-- +migrate Down
ALTER TABLE `user` DROP COLUMN `avatar_update_at`;
