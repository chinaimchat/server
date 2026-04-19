-- +migrate Up

-- 表情商店示例数据：插入一条示例表情包，便于商店列表不为空（可按需在管理后台或数据库继续添加）
INSERT INTO `sticker_store` (`category`, `title`, `desc`, `cover`, `cover_lim`, `is_gone`, `created_at`, `updated_at`)
SELECT 'emoji', '示例表情', '默认示例表情包', '', '', 0, NOW(), NOW()
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `sticker_store` WHERE `category` = 'emoji' LIMIT 1);
