-- +migrate Up
ALTER TABLE `app_config`
ADD COLUMN `show_last_offline_on` smallint NOT NULL DEFAULT 1 COMMENT '是否允许客户端看到对方上次在线时间';

-- +migrate Down
ALTER TABLE `app_config`
DROP COLUMN `show_last_offline_on`;
