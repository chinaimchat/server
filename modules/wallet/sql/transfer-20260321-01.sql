-- +migrate Up

-- 转账
create table `transfer`
(
  id          integer       not null primary key AUTO_INCREMENT,
  transfer_no VARCHAR(40)   not null default '',
  from_uid    VARCHAR(40)   not null default '',
  to_uid      VARCHAR(40)   not null default '',
  amount      DECIMAL(12,2) not null default 0.00,
  remark      VARCHAR(200)  not null default '',
  status      smallint      not null default 0,
  expired_at  timeStamp     null,
  created_at  timeStamp     not null DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX transfer_transfer_no on `transfer` (transfer_no);
CREATE INDEX transfer_from_uid on `transfer` (from_uid);
CREATE INDEX transfer_to_uid on `transfer` (to_uid);
CREATE INDEX transfer_status on `transfer` (status);
