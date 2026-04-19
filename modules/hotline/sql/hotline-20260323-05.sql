-- +migrate Up
-- 补齐：迁移 hotline-20210202-01 若曾中断在 hotline_agent 附近，则缺索引与末尾三张表；新环境若已由 20210202 建全，此处 IF NOT EXISTS / 按 statistics 跳过。
-- 若 hotline_agent 为后台精简表（无 app_id），则备份后按官方模块结构重建。

SET @hlm_db := DATABASE();

SET @hlm_agent_tbl := (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=@hlm_db AND table_name='hotline_agent');
SET @hlm_agent_appid := (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=@hlm_db AND table_name='hotline_agent' AND column_name='app_id');
SET @hlm_rn := IF(@hlm_agent_tbl>0 AND @hlm_agent_appid=0, 'RENAME TABLE `hotline_agent` TO `hotline_agent_legacy_admin`', 'SELECT 1');
PREPARE hlm_rn_st FROM @hlm_rn;
EXECUTE hlm_rn_st;
DEALLOCATE PREPARE hlm_rn_st;

create table IF NOT EXISTS  `hotline_agent`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    uid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '客服uid',
    name VARCHAR(40) NOT NULL DEFAULT '' COMMENT '客服名称',
    last_active integer NOT NULL DEFAULT 0 COMMENT '最后一次活动时间 10位时间戳（单位秒）',
    is_work smallint  NOT NULL DEFAULT 0 COMMENT '是否工作中...',
    role    VARCHAR(40) NOT NULL DEFAULT '' COMMENT '角色',
    position  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '职位',
    status smallint not NULL DEFAULT 0  COMMENT '0.不可用 1.正常',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP
);

-- hotline_agent 索引
SET @hlm_e := (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=@hlm_db AND table_name='hotline_agent' AND index_name='app_id_idx');
SET @hlm_s := IF(@hlm_e=0, 'CREATE INDEX app_id_idx on `hotline_agent` (app_id)', 'SELECT 1');
PREPARE hlm_st FROM @hlm_s;
EXECUTE hlm_st;
DEALLOCATE PREPARE hlm_st;

SET @hlm_e := (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=@hlm_db AND table_name='hotline_agent' AND index_name='app_uid_idx');
SET @hlm_s := IF(@hlm_e=0, 'CREATE UNIQUE INDEX app_uid_idx on `hotline_agent` (app_id,uid)', 'SELECT 1');
PREPARE hlm_st FROM @hlm_s;
EXECUTE hlm_st;
DEALLOCATE PREPARE hlm_st;

-- 会话
create table IF NOT EXISTS  `hotline_session`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid   VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客vid 如果是跟访客聊天则此有值',
    channel_type smallint  NOT NULL  DEFAULT 0 COMMENT '频道类型',
    channel_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '频道id',
    send_count integer  NOT NULL  DEFAULT 0 COMMENT '发送次数',
    last_send  integer  NOT NULL  DEFAULT 0 COMMENT '最后一次发送消息时间戳(10位)',
    recv_count integer  NOT NULL  DEFAULT 0 COMMENT '收到次数',
    last_recv  integer  NOT NULL  DEFAULT 0 COMMENT '最后一次收消息时间戳(10位)',
    unread_count integer NOT NULL  DEFAULT 0 COMMENT '未读数',
    last_message text      NOT NULL   COMMENT '最后一条消息的内容',
    last_content_type integer NOT NULL  DEFAULT 0 COMMENT '最后一条消息正文类型',
    last_session_timestamp integer NOT NULL  DEFAULT 0 COMMENT '最后一次会话时间',
    version_lock  integer NOT NULL  DEFAULT 0 COMMENT '版本锁',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP
);

SET @hlm_e := (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=@hlm_db AND table_name='hotline_session' AND index_name='app_id_channel_type_id_idx');
SET @hlm_s := IF(@hlm_e=0, 'CREATE UNIQUE INDEX app_id_channel_type_id_idx on `hotline_session` (app_id,channel_id,channel_type)', 'SELECT 1');
PREPARE hlm_st FROM @hlm_s;
EXECUTE hlm_st;
DEALLOCATE PREPARE hlm_st;

create table IF NOT EXISTS  `hotline_info_category`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    category_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '类别编号',
    category_name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '类别名称',
    creater VARCHAR(40) NOT NULL DEFAULT '' COMMENT '创建者uid',
    share smallint NOT NULL  DEFAULT 0 COMMENT '是否分享给所有 0.否 1.是',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP
);

create table IF NOT EXISTS  `hotline_quick_reply`
(
   id integer PRIMARY KEY AUTO_INCREMENT,
   app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
   title VARCHAR(40) NOT NULL DEFAULT '' COMMENT '快捷回复标题',
   content text NOT NULL                 COMMENT '快捷回复正文',
   category_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '类别编号',
   shortcode VARCHAR(40) NOT NULL DEFAULT '' COMMENT '短码',
   creater VARCHAR(40) NOT NULL DEFAULT '' COMMENT '创建者uid',
   created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
   updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP
);
