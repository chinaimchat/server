-- +migrate Up

--  app配置
create table IF NOT EXISTS  `hotline_config`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    app_name VARCHAR(40)  NOT NULL DEFAULT '' COMMENT 'APP 名字',
    uid     VARCHAR(40)  NOT NULL DEFAULT '' COMMENT 'app对应的用户uid，一些系统消息将以此uid的名义发送',
    logo   VARCHAR(255) NOT NULL DEFAULT '' COMMENT '公司logo',
    color  VARCHAR(10) NOT NULL DEFAULT '' COMMENT 'widget颜色 例如 #ec45a2',
    chat_bg  VARCHAR(255) NOT NULL DEFAULT '' COMMENT '聊天背景',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
SET @hl_db := DATABASE();
SET @hl_cfg_idx := (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=@hl_db AND table_name='hotline_config' AND index_name='app_id_idx');
SET @hl_cfg_sql := IF(@hl_cfg_idx=0, 'CREATE UNIQUE INDEX app_id_idx on `hotline_config` (app_id)', 'SELECT 1');
PREPARE hl_stmt_cfg_idx FROM @hl_cfg_sql;
EXECUTE hl_stmt_cfg_idx;
DEALLOCATE PREPARE hl_stmt_cfg_idx;

-- 后台管理曾创建的精简版 hotline_config 可能缺少 chat_bg，补齐后再插入种子数据
SET @hl_has_chat_bg := (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=@hl_db AND table_name='hotline_config' AND column_name='chat_bg');
SET @hl_add_chat_bg := IF(@hl_has_chat_bg=0, 'ALTER TABLE `hotline_config` ADD COLUMN `chat_bg` VARCHAR(255) NOT NULL DEFAULT '''' COMMENT ''聊天背景'' AFTER `color`', 'SELECT 1');
PREPARE hl_stmt_chat_bg FROM @hl_add_chat_bg;
EXECUTE hl_stmt_chat_bg;
DEALLOCATE PREPARE hl_stmt_chat_bg;

--  测试数据
INSERT IGNORE INTO hotline_config(app_id,app_name,logo,color,chat_bg) VALUES('wukongchat','海洋之家小二','https://ss0.bdstatic.com/70cFuHSh_Q1YnxGkpoWK1HF6hhy/it/u=1696053968,2389900790&fm=26&gp=0.jpg','red','');

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_config_updated_at
--   BEFORE UPDATE
--   ON `hotline_config` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd

-- 访客
create table IF NOT EXISTS  `hotline_visitor`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客唯一ID',
    name varchar(40) NOT NULL DEFAULT '' COMMENT '用户名称',
    avatar varchar(255) NOT NULL DEFAULT '' COMMENT '用户头像地址',
    ip_address varchar(20) NOT NULL DEFAULT '' COMMENT 'IP地址',
    phone varchar(20) NOT NULL DEFAULT '' COMMENT '手机号',
    email varchar(40) NOT NULL DEFAULT '' COMMENT 'email',
    state varchar(40) NOT NULL DEFAULT '' COMMENT '省',
    city varchar(40) NOT NULL DEFAULT '' COMMENT '市',
    last_session bigint NOT NULL DEFAULT  0 COMMENT '最后一次会话时间 单位秒',
    last_reply  bigint NOT NULL DEFAULT  0 COMMENT '访客最后一次回复时间 单位秒',
    timezone varchar(40) NOT NULL DEFAULT '' COMMENT '访客时区',
    source  varchar(100) NOT NULL DEFAULT '' COMMENT '来源（访客是从那些渠道过来的，比如 百度，360搜索 soso等等）',
    search_keyword varchar(255) NOT NULL DEFAULT '' COMMENT '通过搜索什么关键字过来的',
    session_count integer NOT NULL DEFAULT  0 COMMENT '会话次数',
    `local` smallint not null default 0 COMMENT '访客是否是本地用户，（同一个系统的用户）',
    status smallint NOT NULL DEFAULT 0 COMMENT '访客状态 0.非活动状态 1.活动状态',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
   
);
CREATE UNIQUE INDEX vid_idx on `hotline_visitor` (app_id,vid);

-- 访客自定义属性
create table IF NOT EXISTS  `hotline_visitor_props`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客唯一ID',
    field VARCHAR(40) NOT NULL DEFAULT '' COMMENT '属性域',
    value VARCHAR(255) NOT NULL DEFAULT '' COMMENT '属性值',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX vid_app_idx on `hotline_visitor_props` (vid,app_id);
CREATE UNIQUE INDEX field_unque_idx on `hotline_visitor_props` (field,vid,app_id);
CREATE INDEX vid_idx on `hotline_visitor_props` (vid);


-- 在线状态表
create table IF NOT EXISTS  `hotline_online`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    user_type smallint  NOT NULL  DEFAULT 0 COMMENT '用户类型 0.访客 1.客服',
    user_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT '如果user_type==0则为访客ID user_type==1则为客服uid',
    online smallint not null DEFAULT 0 COMMENT '是否在线 0.否 1.是',
    last_offline   integer not null DEFAULT 0 COMMENT '最后一次离线时间',
    last_online   integer not null DEFAULT 0 COMMENT '最后一次在线时间',
    device_flag smallint not null DEFAULT 0 COMMENT '设备标记 0.APP 1.PC'
);
CREATE UNIQUE INDEX type_id_idx on `hotline_online` (user_id,user_type,device_flag);


-- 访问历史
create table IF NOT EXISTS  `hotline_history`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客唯一ID',
    site_url VARCHAR(255)  NOT NULL DEFAULT '' COMMENT '访问的站点',
    site_title VARCHAR(100)  NOT NULL DEFAULT '' COMMENT '站点标题',
    referrer  VARCHAR(1000)   NOT NULL DEFAULT '' COMMENT '站点referrer',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
   
);
CREATE INDEX vid_idx on `hotline_history` (vid);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_history_updated_at
--   BEFORE UPDATE
--   ON `hotline_history` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 访客频道
create table IF NOT EXISTS  `hotline_channel`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客唯一ID(访客频道有效)',
    topic_id integer NOT NULL DEFAULT 0 COMMENT 'topicID(访客频道有效)',
    title  VARCHAR(100) NOT NULL DEFAULT '' COMMENT '频道标题(访客频道有效)',
    channel_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '频道ID',
    channel_type smallint  NOT NULL DEFAULT 0 COMMENT '频道类型',
    bind  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '绑定的频道ID',
    bus_type smallint NOT NULL DEFAULT 0 COMMENT '频道业务类型 0.访客频道 1.技能组 2.普通群组',
    agent_uid  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '分配的客服(访客频道有效)',
    session_active_limit     integer    not NULL DEFAULT 0 COMMENT '每名成员的活动对话限制（技能组有效）',
    category    VARCHAR(40) NOT NULL DEFAULT '' COMMENT '会话类别 new:新建 assignMe:指派给我的 solved: 已解决的 (访客频道有效)',
    disable smallint NOT NULL DEFAULT 0 COMMENT '是否禁用 0.否 1.是',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE UNIQUE INDEX channel_idx on `hotline_channel` (channel_id);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_visitor_channel_updated_at
--   BEFORE UPDATE
--   ON `hotline_visitor_channel` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd

-- 频道订阅者
create table IF NOT EXISTS  `hotline_subscribers`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    channel_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '频道ID',
    channel_type smallint  NOT NULL DEFAULT 0 COMMENT '频道类型',
    subscriber_type  smallint NOT NULL DEFAULT 0 COMMENT '0.访客 1.客服',
    subscriber  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '订阅者ID 0.位vid 1.为uid',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX channel_idx on `hotline_subscribers` (channel_id,channel_type);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_subscribers_updated_at
--   BEFORE UPDATE
--   ON `hotline_subscribers` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd



-- 访客设备
create table IF NOT EXISTS  `hotline_device`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客唯一ID',
    device VARCHAR(40) NOT NULL DEFAULT '' COMMENT '设备：例如: desktop,phone',
    os    VARCHAR(40) NOT NULL DEFAULT '' COMMENT '设备系统 例如 Web,iOS,Android',
    model VARCHAR(40) NOT NULL DEFAULT '' COMMENT '设备型号 例如 Chrome，iPhone X',
    `version` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '系统版本：例如：13.0',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);

CREATE INDEX vid_idx on `hotline_device` (vid);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_device_updated_at
--   BEFORE UPDATE
--   ON `hotline_device` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 话题
create table IF NOT EXISTS  `hotline_topic`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    title  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '话题标题',
    welcome VARCHAR(255) NOT NULL DEFAULT '' COMMENT '欢迎语',
    is_default smallint not NULL DEFAULT 0 COMMENT '是否是默认话题',
    is_deleted smallint   not NULL DEFAULT 0 COMMENT '是否已删除',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);

INSERT IGNORE INTO hotline_topic(app_id,title,welcome) VALUES('wukongchat','测试话题','欢迎咨询测试话题');
INSERT IGNORE INTO hotline_topic(app_id,title,welcome) VALUES('wukongchat','测试话题2','欢迎咨询测试话题2');

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_topic_updated_at
--   BEFORE UPDATE
--   ON `hotline_topic` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd



-- 智能分配配置
create table IF NOT EXISTS  `hotline_intelli_assign`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    strategy VARCHAR(40) NOT NULL DEFAULT '' COMMENT '智能策略 turn:轮流，balance:负载',
    user_max_idle  integer      not NULL DEFAULT 0 COMMENT '成员最大空闲(单位分钟),如果成员达到最大空闲，则将其设为非活动状态',
    visitor_max_idle  integer   not NULL DEFAULT 0 COMMENT '访客最大空闲(单位分钟),如果访客达到最大空闲，则将其设为非活动状态',
    session_remember_enable smallint NOT NULL DEFAULT 0 COMMENT '会话记住是否开启',
    session_remember_min  integer   not NULL DEFAULT 0 COMMENT '对话记忆时间(单位分钟) 如果对话在此时间内重新打开，则将其重新指派给同一成员',
    session_active_limit     integer    not NULL DEFAULT 0 COMMENT '每名成员的活动对话限制 不能为0 为0将不会分配',
    status smallint   not NULL DEFAULT 0 COMMENT '0.禁用 1.启用',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_intelli_assign` (app_id);

INSERT IGNORE INTO hotline_intelli_assign(app_id,strategy,user_max_idle,visitor_max_idle,session_remember_enable,session_remember_min,session_active_limit,status) VALUES('wukongchat','turn',10,10,1,1440,50,1);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_intelli_assign_updated_at
--   BEFORE UPDATE
--   ON `hotline_intelli_assign` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd



-- 群组
create table IF NOT EXISTS  `hotline_group`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    group_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '群组唯一编号',
    group_type smallint  NOT NULL DEFAULT 1 COMMENT '群类型 1.普通群 2.技能组',
    creater_uid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '创建者uid',
    name   VARCHAR(40) NOT NULL DEFAULT '' COMMENT '群组名字',
    remark   VARCHAR(255) NOT NULL DEFAULT '' COMMENT '群组备注',
    session_active_limit    integer    not NULL DEFAULT 0 COMMENT '每名成员的活动对话限制',
    intelli_assign   smallint NOT NULL DEFAULT 0 COMMENT '是否开启智能分配',
    status     smallint  NOT NULL DEFAULT 0 COMMENT '0.禁用 1.启用',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_group` (app_id);

INSERT IGNORE INTO hotline_group(app_id,group_no,group_type,name,session_active_limit,status) VALUES('default','beginner',2,'入门',1,1);
INSERT IGNORE INTO hotline_group(app_id,group_no,group_type,name,session_active_limit,status) VALUES('default','intermediate',2,'熟练',5,1);
INSERT IGNORE INTO hotline_group(app_id,group_no,group_type,name,session_active_limit,status) VALUES('default','expert',2,'专家',8,1);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_group_updated_at
--   BEFORE UPDATE
--   ON `hotline_group` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 群组成员
create table IF NOT EXISTS  `hotline_members`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    group_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '群组唯一编号',
    uid   VARCHAR(40) NOT NULL DEFAULT '' COMMENT '成员uid',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_members` (app_id);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_members_updated_at
--   BEFORE UPDATE
--   ON `hotline_members` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 规则属性
create table IF NOT EXISTS  `hotline_rule_props`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id   VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    name     VARCHAR(40) NOT NULL DEFAULT '' COMMENT '属性名称',
    field    VARCHAR(40) NOT NULL DEFAULT '' COMMENT '属性field',
    kind VARCHAR(40) NOT NULL DEFAULT '' COMMENT '属性种类 message：消息属性 visitor:访客属性 sys:系统属性',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 

);
CREATE INDEX app_id_idx on `hotline_rule_props` (app_id);


create table IF NOT EXISTS  `hotline_field`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    field VARCHAR(40) NOT NULL DEFAULT '' COMMENT '属性域',
    name VARCHAR(40) NOT NULL DEFAULT '' COMMENT '域名称',
    `group`  VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '域所属组',
    `type` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '域类型 string:字符串 select:下拉选择 number:数字 datetime:日前时间（例如2021-01-12 12:23:23） date: 日期（例如：2020-11-12）',
    datasource VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '数据源 options:多选 从options字段获取 topic:从topic表中获取',
    options   VARCHAR(1000)  NOT NULL DEFAULT '' COMMENT '数据选项 例如[{"value":"test",label:"测试"}]',
    symbols   VARCHAR(500)  NOT NULL DEFAULT '' COMMENT '符号例如: [{"label":"等于","value":"="},{"label":"不等于","value":"<>"}]',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);

INSERT IGNORE INTO hotline_field(`group`,field,name,`type`,datasource,options,symbols) VALUES('message','message.payload.content','消息内容','string','','','[{"label":"等于","value":"="},{"label":"不等于","value":"<>"},{"label":"包含","value":"in"},{"label":"不包含","value":"not"}]');
INSERT IGNORE INTO hotline_field(`group`,field,name,`type`,datasource,options,symbols) VALUES('message','channel.topicid','话题','select','topic','','[{"label":"等于","value":"="},{"label":"不等于","value":"<>"}]');
INSERT IGNORE INTO hotline_field(`group`,field,name,`type`,datasource,options,symbols) VALUES('visitor','visitor.state','省份','string','','','[{"label":"等于","value":"="},{"label":"不等于","value":"<>"},{"label":"包含","value":"in"},{"label":"不包含","value":"not"}]');
INSERT IGNORE INTO hotline_field(`group`,field,name,`type`,datasource,options,symbols) VALUES('visitor','visitor.city','城市','string','','','[{"label":"等于","value":"="},{"label":"不等于","value":"<>"},{"label":"包含","value":"in"},{"label":"不包含","value":"not"}]');



-- 指派规则 优先于hotline_intelli_assign
create table IF NOT EXISTS  `hotline_rule`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    rule_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '规则编号',
    name   VARCHAR(40) NOT NULL DEFAULT '' COMMENT '指派规则名称',
    expression VARCHAR(255) NOT NULL DEFAULT '' COMMENT '表达式 比如 (A1&A2)||(A3|A4)&&(A5&A6)',
    status smallint    NOT NULL DEFAULT 0 COMMENT '0.不可用 1.可用',
    weight   integer   NOT NULL DEFAULT 0 COMMENT '权重，越大越优先',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_rule` (app_id);
CREATE UNIQUE INDEX rule_no_uidx on `hotline_rule` (rule_no);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_rule_updated_at
--   BEFORE UPDATE
--   ON `hotline_rule` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 规则条件
create table IF NOT EXISTS  `hotline_condition`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    rule_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '规则编号',
    tag    VARCHAR(20) NOT NULL DEFAULT '' COMMENT '标示',
    field  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '域 比如 message.text',
    value  VARCHAR(100) NOT NULL DEFAULT '' COMMENT '满足条件的值',
    `condition` VARCHAR(20) NOT NULL DEFAULT '' COMMENT '条件 =:等于 <>:不等于,not:不包含,in:包含',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_condition` (app_id);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_condition_updated_at
--   BEFORE UPDATE
--   ON `hotline_condition` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 规则结果
create table IF NOT EXISTS  `hotline_rule_result`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    rule_no VARCHAR(40) NOT NULL DEFAULT '' COMMENT '规则编号',
    assign_type smallint NOT NULL DEFAULT 0 COMMENT '分配类型 1.分配给人 2.分配给群组',
    assign_no  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '分配编号，如果是1.则为个人uid，如果是2.则为群组编号',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_rule_result` (app_id);
CREATE UNIQUE INDEX rule_no_uidx on `hotline_rule_result` (rule_no);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_rule_result_updated_at
--   BEFORE UPDATE
--   ON `hotline_rule_result` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 分配历史
create table IF NOT EXISTS  `hotline_assign_history`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    vid VARCHAR(40) NOT NULL DEFAULT '' COMMENT '访客编号',
    uid  text NOT NULL  COMMENT '分配给的客服',
    group_no  VARCHAR(40) NOT NULL DEFAULT '' COMMENT '群组编号，如果是分配给的群组 则此字段有值',
    rule_no   VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '如果是通过规则触发的 则有此字段',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_assign_history` (app_id);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_assign_history_updated_at
--   BEFORE UPDATE
--   ON `hotline_assign_history` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd


-- 角色
create table IF NOT EXISTS  `hotline_role`
(
    id integer PRIMARY KEY AUTO_INCREMENT,
    app_id VARCHAR(40) NOT NULL DEFAULT '' COMMENT 'APP ID',
    role VARCHAR(40) NOT NULL DEFAULT '' COMMENT '角色',
    remark VARCHAR(40) NOT NULL DEFAULT 0 COMMENT '角色说明',
    created_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
    updated_at timeStamp    not null DEFAULT CURRENT_TIMESTAMP 
);
CREATE INDEX app_id_idx on `hotline_role` (app_id);
CREATE UNIQUE INDEX role_app_id_idx on `hotline_role` (role,app_id);

-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_role_updated_at
--   BEFORE UPDATE
--   ON `hotline_role` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd

INSERT IGNORE INTO hotline_role(app_id,role,remark) values('default','admin','超级管理员');
INSERT IGNORE INTO hotline_role(app_id,role,remark) values('default','manager','管理员');
INSERT IGNORE INTO hotline_role(app_id,role,remark) values('default','usermanager','账户管理员');
INSERT IGNORE INTO hotline_role(app_id,role,remark) values('default','agent','客服');


-- 客服
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
CREATE INDEX app_id_idx on `hotline_agent` (app_id);
CREATE UNIQUE INDEX app_uid_idx on `hotline_agent` (app_id,uid);


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
CREATE UNIQUE INDEX app_id_channel_type_id_idx on `hotline_session` (app_id,channel_id,channel_type);


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


-- -- +migrate StatementBegin
-- CREATE TRIGGER hotline_agent_updated_at
--   BEFORE UPDATE
--   ON `hotline_agent` for each row 
--   BEGIN 
--     set NEW.updated_at = NOW();
--   END;
-- -- +migrate StatementEnd