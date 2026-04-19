-- +migrate Up

-- 朋友圈表1
create table `moments1`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号
    publisher           VARCHAR(40)     not null default '',     -- 发布者uid
    publisher_name      VARCHAR(40)     not null default '',     -- 用户UIDs   
    video_path          VARCHAR(255)     not null default '',     -- 视频地址
    video_cover_path    VARCHAR(255)     not null default '',     -- 视频封面
    content             text,                                    -- 发布内容
    imgs                VARCHAR(1000)    not null default '',     -- 图片地址
    privacy_type        VARCHAR(40)     not null default '',     -- 隐私类型
    privacy_uids        VARCHAR(100)    not null default '',     -- 不可见用户uid
    address             VARCHAR(255)    not null default '',     -- 位置
    longitude           VARCHAR(100)    not null default '',     -- 经度
    latitude            VARCHAR(100)    not null default '',     -- 纬度
    remind_uids         text            not null ,     -- 提醒用户uid
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

-- -- +migrate StatementBegin
-- CREATE TRIGGER moments1_updated_at
--   BEFORE UPDATE
--   ON `moments1` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 朋友圈表2
create table `moments2`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号  
    publisher           VARCHAR(40)     not null default '',     -- 发布者uid
    publisher_name      VARCHAR(40)     not null default '',     -- 用户UIDs   
    video_path          VARCHAR(255)     not null default '',     -- 视频地址
    video_cover_path    VARCHAR(255)     not null default '',     -- 视频封面
    content             text,                                    -- 发布内容
    imgs                VARCHAR(1000)    not null default '',     -- 图片地址
    privacy_type        VARCHAR(40)     not null default '',     -- 隐私类型
    privacy_uids        VARCHAR(100)    not null default '',     -- 不可见用户uid
    address             VARCHAR(255)    not null default '',     -- 位置
    longitude           VARCHAR(100)    not null default '',     -- 经度
    latitude            VARCHAR(100)    not null default '',     -- 纬度
    remind_uids         text            not null ,     -- 提醒用户uid
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

-- -- +migrate StatementBegin
-- CREATE TRIGGER moments2_updated_at
--   BEFORE UPDATE
--   ON `moments2` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 朋友圈表3
create table `moments3`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号
    publisher           VARCHAR(40)     not null default '',     -- 发布者uid
    publisher_name      VARCHAR(40)     not null default '',     -- 用户UIDs   
    video_path          VARCHAR(255)     not null default '',     -- 视频地址
    video_cover_path    VARCHAR(255)     not null default '',     -- 视频封面
    content             text,                                    -- 发布内容
    imgs                VARCHAR(1000)    not null default '',     -- 图片地址
    privacy_type        VARCHAR(40)     not null default '',     -- 隐私类型
    privacy_uids        VARCHAR(100)    not null default '',     -- 不可见用户uid
    address             VARCHAR(255)    not null default '',     -- 位置
    longitude           VARCHAR(100)    not null default '',     -- 经度
    latitude            VARCHAR(100)    not null default '',     -- 纬度
    remind_uids         text            not null ,     -- 提醒用户uid
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

-- -- +migrate StatementBegin
-- CREATE TRIGGER moments3_updated_at
--   BEFORE UPDATE
--   ON `moments3` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd

-- 朋友圈关系表1
create table `moment_user1`(
    id                  integer         not null primary key AUTO_INCREMENT,
    uid                 VARCHAR(40)     not null default '',                -- 用户ID
    moment_no           VARCHAR(40)     not null default '',                -- 朋友圈编号
    publisher           VARCHAR(40)     not null default '',                -- 发布者
    sort_num            int             not null DEFAULT 0,                 -- 排序编号
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE unique INDEX moment_user1_uid_moment_no on `moment_user1` (uid, moment_no);

-- -- +migrate StatementBegin
-- CREATE TRIGGER moment_user1_updated_at
--   BEFORE UPDATE
--   ON `moment_user1` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 朋友圈关系表2
create table `moment_user2`(
    id                  integer         not null primary key AUTO_INCREMENT,
    uid                 VARCHAR(40)     not null default '',                -- 用户ID
    moment_no           VARCHAR(40)     not null default '',                -- 朋友圈编号
    publisher           VARCHAR(40)     not null default '',                -- 发布者
    sort_num            int             not null DEFAULT 0,                 -- 排序编号
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE unique INDEX moment_user2_uid_moment_no on `moment_user2` (uid, moment_no);
-- -- +migrate StatementBegin
-- CREATE TRIGGER moment_user2_updated_at
--   BEFORE UPDATE
--   ON `moment_user2` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd

-- 朋友圈关系表3
create table `moment_user3`(
    id                  integer         not null primary key AUTO_INCREMENT,
    uid                 VARCHAR(40)     not null default '',                -- 用户ID
    moment_no           VARCHAR(40)     not null default '',                -- 朋友圈编号
    publisher           VARCHAR(40)     not null default '',                -- 发布者
    sort_num            int             not null DEFAULT 0,                 -- 排序编号
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE unique INDEX moment_user3_uid_moment_no on `moment_user3` (uid, moment_no);
-- -- +migrate StatementBegin
-- CREATE TRIGGER moment_user3_updated_at
--   BEFORE UPDATE
--   ON `moment_user3` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 评论表1
create table `comments1`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号
    content             VARCHAR(100)    not null default '',     -- 评论内容
    uid                 VARCHAR(40)     not null default '',     -- 评论者uid
    name                VARCHAR(100)    not null default '',     -- 评论者名字
    handle_type         smallint        not null default 0,      -- 操作类型0:点赞1:评论 
    reply_comment_id    VARCHAR(40)     not null default '',     -- 回复评论的id
    reply_uid           VARCHAR(40)     not null default '',     -- 被回复者uid
    reply_name          VARCHAR(100)    not null default '',     -- 被回复者名字
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE INDEX comments1_moment_no on `comments1` (moment_no);

-- -- +migrate StatementBegin
-- CREATE TRIGGER comments1_updated_at
--   BEFORE UPDATE
--   ON `comments1` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 评论表2
create table `comments2`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号
    content             VARCHAR(100)    not null default '',     -- 评论内容
    uid                 VARCHAR(40)     not null default '',     -- 评论者uid
    name                VARCHAR(100)    not null default '',     -- 评论者名字
    handle_type         smallint        not null default 0,      -- 操作类型0:点赞1:评论 
    reply_comment_id    VARCHAR(40)     not null default '',     -- 回复评论的id
    reply_uid           VARCHAR(40)     not null default '',     -- 被回复者uid
    reply_name          VARCHAR(100)    not null default '',     -- 被回复者名字
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE INDEX comments2_moment_no on `comments2` (moment_no);

-- -- +migrate StatementBegin
-- CREATE TRIGGER comments2_updated_at
--   BEFORE UPDATE
--   ON `comments2` for each row 
--   BEGIN
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd



-- 评论表3
create table `comments3`(
    id                  integer         not null primary key AUTO_INCREMENT,
    moment_no           VARCHAR(40)     not null default '',     -- 朋友圈编号
    content             VARCHAR(100)    not null default '',     -- 评论内容
    uid                 VARCHAR(40)     not null default '',     -- 评论者uid
    name                VARCHAR(100)    not null default '',     -- 评论者名字
    handle_type         smallint        not null default 0,      -- 操作类型0:点赞1:评论 
    reply_comment_id    VARCHAR(40)     not null default '',     -- 回复评论的id
    reply_uid           VARCHAR(40)     not null default '',     -- 被回复者uid
    reply_name          VARCHAR(100)    not null default '',     -- 被回复者名字
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

CREATE INDEX comments3_moment_no on `comments3` (moment_no);



-- 朋友圈设置表
create table `moment_setting`(
    id                  integer         not null primary key AUTO_INCREMENT,
    uid                 VARCHAR(40)     not null default '',     -- uid
    to_uid              VARCHAR(40)     not null default '',     -- 被操作者
    is_hide_my          smallint        not null default 0,     -- 隐藏我的朋友圈
    is_hide_his         smallint        not null default 0,     -- 隐藏他的朋友圈
    created_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP, -- 创建时间
    updated_at          timeStamp       not null DEFAULT CURRENT_TIMESTAMP  -- 更新时间
);

