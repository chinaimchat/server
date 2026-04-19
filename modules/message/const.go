package message

const (
	// 消息已删除
	CMDMessageDeleted = "messageDeleted"
	// CMDMessageErase 消息擦除
	CMDMessageErase = "messageEerase"
)
const CacheReadedCountPrefix = "readedCount:" // 消息已读数量

type ReminderType int

const (
	ReminderTypeMentionMe      = 1 // 有人@我
	ReminderTypeApplyJoinGroup = 2 // 申请加群
)

