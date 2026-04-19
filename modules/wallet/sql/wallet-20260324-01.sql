-- +migrate Up
-- 提现先冻结：可用 balance、冻结 frozen_balance；审核通过再从冻结划出，拒绝/超时解冻退回

ALTER TABLE `wallet` ADD COLUMN `frozen_balance` DECIMAL(20,2) NOT NULL DEFAULT 0 COMMENT '提现冻结金额' AFTER `balance`;

-- 兼容旧数据：此前申请时已从 balance 扣款，待审核(status=0)的金额记入 frozen_balance（balance 已是扣后可用额）
UPDATE `wallet` w
INNER JOIN (
  SELECT `uid`, COALESCE(SUM(`amount` + `fee`), 0) AS pending_sum
  FROM `wallet_withdrawal`
  WHERE `status` = 0
  GROUP BY `uid`
) t ON w.`uid` = t.`uid`
SET w.`frozen_balance` = t.pending_sum;
