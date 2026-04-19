package hotline

// AssignType 指派类型
type AssignType int

const (
	// AssignTypePerson 个人
	AssignTypePerson AssignType = 1
	// AssignTypeGroup 群组
	AssignTypeGroup AssignType = 2
)

func (a AssignType) Int() int {
	return int(a)
}

// GroupType 群类型
type GroupType int

const (
	// GroupTypeCommon 普通群
	GroupTypeCommon GroupType = 1 // 普通群
	// GroupTypeKill 技能群
	GroupTypeKill GroupType = 2 // 技能群
)

// Int Int
func (g GroupType) Int() int {
	return int(g)
}

// SubscriberType 订阅者类型
type SubscriberType int

const (
	// SubscriberTypeVisitor 访客
	SubscriberTypeVisitor SubscriberType = iota
	// SubscriberTypeAgent 客服
	SubscriberTypeAgent
)

// Int Int
func (s SubscriberType) Int() int {
	return int(s)
}

// RoleConst 角色常量
type RoleConst string

const (
	// RoleAdmin 超级管理员
	RoleAdmin RoleConst = "admin"
	// RoleManager 管理员
	RoleManager RoleConst = "manager"
	// RoleUsermanager 用户管理员
	RoleUsermanager RoleConst = "usermanager"
	// RoleAgent 普通客服
	RoleAgent RoleConst = "agent"
)

func (r RoleConst) String() string {
	return string(r)
}

// RoleName RoleName
func (r RoleConst) RoleName() string {
	if r == RoleAdmin {
		return "超级管理员"
	}
	if r == RoleManager {
		return "管理员"
	}
	if r == RoleUsermanager {
		return "用户管理员"
	}
	if r == RoleAgent {
		return "普通客服"
	}
	return ""
}

// DefaultAppID  默认appid
const DefaultAppID = "default"

// UserType 用户类型
type UserType int

const (
	// UserTypeVisitor 访客
	UserTypeVisitor UserType = 0
	// UserTypeAgent 客服
	UserTypeAgent UserType = 1
)

// Int Int
func (u UserType) Int() int {
	return int(u)
}

type SessionCategory string

const (
	// SessionCategoryRealtime 实时访客
	SessionCategoryRealtime SessionCategory = "Realtime"
	// SessionCategoryNew 新建
	SessionCategoryNew SessionCategory = "new"
	// SessionCategoryAssignMe 指派给我的
	SessionCategoryAssignMe SessionCategory = "assignMe"
	// SessionCategorySolved 已解决
	SessionCategorySolved SessionCategory = "solved"
	// SessionCategoryAllAssigned 所有已指派
	SessionCategoryAllAssigned SessionCategory = "allAssigned"
)

func (s SessionCategory) String() string {
	return string(s)
}

// 频道业务类型
type ChannelBusType int

const (
	// ChannelBusTypeVisitor 访客频道
	ChannelBusTypeVisitor ChannelBusType = iota
	// ChannelBusTypeSkill 技能组
	ChannelBusTypeSkill
	// ChannelBusTypeCommon 普通群组
	ChannelBusTypeCommon
)

func (c ChannelBusType) Int() int {
	return int(c)
}

type IntelliStrategy string

const (
	IntelliStrategyTurn    IntelliStrategy = "turn"
	IntelliStrategyBalance IntelliStrategy = "balance"
)

func (i IntelliStrategy) String() string {
	return string(i)
}

type UserCategory string

const (
	UserCategoryVisitor         UserCategory = "visitor"         // 访客
	UserCategoryCustomerService UserCategory = "customerService" // 客服
)
