package moments

import (
	"errors"
	"strconv"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// Setting 朋友圈设置
type Setting struct {
	ctx *config.Context
	log.Log
	userService user.IService
	settingDB   *settingDB
}

// NewSetting NewSetting
func NewSetting(ctx *config.Context) *Setting {
	return &Setting{
		Log:         log.NewTLog("moments"),
		ctx:         ctx,
		userService: user.NewService(ctx),
		settingDB:   newSettingDB(ctx),
	}
}

// Route 路由配置
func (s *Setting) Route(r *wkhttp.WKHttp) {
	setting := r.Group("/v1/moment", s.ctx.AuthMiddleware(r))
	{
		setting.PUT("/setting/hidemy/:to_uid/:on", s.hideMy)   // 隐藏我的朋友圈
		setting.PUT("/setting/hidehis/:to_uid/:on", s.hideHis) // 不看别人的朋友圈
		setting.GET("/setting/:to_uid", s.detail)              // 朋友圈设置
	}
}

// 隐藏我的朋友圈
func (s *Setting) hideMy(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	toUID := c.Param("to_uid")
	on := c.Param("on")
	err := s.checkUser(toUID)
	if err != nil {
		c.ResponseError(err)
		return
	}
	model, err := s.settingDB.queryWithUIDAndToUID(loginUID, toUID)
	if err != nil {
		s.Error("查询用户朋友圈设置错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户朋友圈设置错误"))
		return
	}
	isHideMy, _ := strconv.Atoi(on)
	insert := true
	if model == nil {
		model = &settingModel{
			IsHideMy:  isHideMy,
			IsHideHis: 0,
			UID:       loginUID,
			ToUID:     toUID,
		}
	} else {
		model.IsHideMy = isHideMy
		insert = false
	}
	if insert {
		err = s.settingDB.insert(model)
		if err != nil {
			s.Error("添加朋友圈权限错误", zap.Error(err))
			c.ResponseError(errors.New("添加朋友圈权限错误"))
			return
		}
	} else {
		err = s.settingDB.update(model)
		if err != nil {
			s.Error("修改朋友圈权限错误", zap.Error(err))
			c.ResponseError(errors.New("修改朋友圈权限错误"))
			return
		}
	}

	c.ResponseOK()
}

// 不看他的朋友圈
func (s *Setting) hideHis(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	toUID := c.Param("to_uid")
	on := c.Param("on")
	err := s.checkUser(toUID)
	if err != nil {
		c.ResponseError(err)
		return
	}
	model, err := s.settingDB.queryWithUIDAndToUID(loginUID, toUID)
	if err != nil {
		s.Error("查询用户朋友圈设置错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户朋友圈设置错误"))
		return
	}
	isHideHis, _ := strconv.Atoi(on)
	insert := true
	if model == nil {
		model = &settingModel{
			IsHideMy:  0,
			IsHideHis: isHideHis,
			UID:       loginUID,
			ToUID:     toUID,
		}
	} else {
		insert = false
		model.IsHideHis = isHideHis
	}
	if insert {
		err = s.settingDB.insert(model)
		if err != nil {
			s.Error("添加朋友圈权限错误", zap.Error(err))
			c.ResponseError(errors.New("添加朋友圈权限错误"))
			return
		}
	} else {
		err = s.settingDB.update(model)
		if err != nil {
			s.Error("修改朋友圈权限错误", zap.Error(err))
			c.ResponseError(errors.New("修改朋友圈权限错误"))
			return
		}
	}
	c.ResponseOK()
}

// 查看对某人对朋友圈设置
func (s *Setting) detail(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	toUID := c.Param("to_uid")
	err := s.checkUser(toUID)
	if err != nil {
		c.ResponseError(err)
		return
	}
	model, err := s.settingDB.queryWithUIDAndToUID(loginUID, toUID)
	if err != nil {
		s.Error("查询用户朋友圈设置错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户朋友圈设置错误"))
		return
	}
	if model == nil {
		model = &settingModel{
			IsHideMy:  0,
			IsHideHis: 0,
		}
	}
	c.Response(&settingResp{
		IsHideMy:  model.IsHideMy,
		IsHideHis: model.IsHideHis,
	})
}

func (s *Setting) checkUser(toUID string) error {
	if toUID == "" {
		return errors.New("操作用户ID不能为空")
	}
	user, err := s.userService.GetUser(toUID)
	if err != nil {
		s.Error("查询用户信息错误", zap.Error(err))
		return errors.New("查询用户信息错误")
	}
	if user == nil {
		return errors.New("操作用户不存在")
	}
	return nil
}

type settingResp struct {
	IsHideMy  int `json:"is_hide_my"`  // 隐藏我的朋友圈
	IsHideHis int `json:"is_hide_his"` // 不看他的朋友圈
}
