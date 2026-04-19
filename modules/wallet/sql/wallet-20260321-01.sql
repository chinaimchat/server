-- +migrate Up

-- 用户钱包
create table `wallet`
(
  id           integer      not null primary key AUTO_INCREMENT,
  uid          VARCHAR(40)  not null default '',
  balance      DECIMAL(12,2) not null default 0.00,
  pay_password VARCHAR(128) not null default '',
  created_at   timeStamp    not null DEFAULT CURRENT_TIMESTAMP,
  updated_at   timeStamp    not null DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX wallet_uid on `wallet` (uid);

-- 交易流水
create table `wallet_transaction`
(
  id           integer       not null primary key AUTO_INCREMENT,
  uid          VARCHAR(40)   not null default '',
  type         VARCHAR(40)   not null default '',
  amount       DECIMAL(12,2) not null default 0.00,
  related_id   VARCHAR(40)   not null default '',
  remark       VARCHAR(200)  not null default '',
  created_at   timeStamp     not null DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX wallet_transaction_uid on `wallet_transaction` (uid);
CREATE INDEX wallet_transaction_type on `wallet_transaction` (type);
