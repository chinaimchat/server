-- +migrate Up

-- 红包
create table `redpacket`
(
  id               integer       not null primary key AUTO_INCREMENT,
  packet_no        VARCHAR(40)   not null default '',
  uid              VARCHAR(40)   not null default '',
  channel_id       VARCHAR(40)   not null default '',
  channel_type     smallint      not null default 0,
  type             smallint      not null default 1,
  total_amount     DECIMAL(12,2) not null default 0.00,
  total_count      integer       not null default 0,
  remaining_amount DECIMAL(12,2) not null default 0.00,
  remaining_count  integer       not null default 0,
  to_uid           VARCHAR(40)   not null default '',
  remark           VARCHAR(200)  not null default '',
  status           smallint      not null default 0,
  expired_at       timeStamp     null,
  created_at       timeStamp     not null DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX redpacket_packet_no on `redpacket` (packet_no);
CREATE INDEX redpacket_uid on `redpacket` (uid);
CREATE INDEX redpacket_status on `redpacket` (status);

-- 红包领取记录
create table `redpacket_record`
(
  id         integer       not null primary key AUTO_INCREMENT,
  packet_no  VARCHAR(40)   not null default '',
  uid        VARCHAR(40)   not null default '',
  amount     DECIMAL(12,2) not null default 0.00,
  is_best    smallint      not null default 0,
  created_at timeStamp     not null DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX redpacket_record_packet_no on `redpacket_record` (packet_no);
CREATE INDEX redpacket_record_uid on `redpacket_record` (uid);
