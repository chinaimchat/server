package sticker

import (
	"errors"
	"sort"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

// 新增注册用户时默认添加一个表情
func (s *Sticker) handleRegisterUserEvent(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	err := util.ReadJsonByByte(data, &req)
	if err != nil {
		s.Error("表情处理用户注册加入群聊参数有误")
		commit(err)
		return
	}
	uid := req["uid"].(string)
	if uid == "" {
		s.Error("表情处理用户注册加入群聊UID不能为空")
		commit(errors.New("表情处理用户注册加入群聊UID不能为空"))
		return
	}

	// 为新用户补齐所有上架的贴图分类（sticker_store.is_gone=0）。
	// 这样用户注册后即可直接在“表情商店/我的”中使用动物贴图，无需逐个点击“添加此分类”。
	cmodel, err := s.db.queryUserCategoryWithMaxSortNum(uid)
	if err != nil {
		s.Error("查询最大用户表情分类错误", zap.Error(err))
		commit(errors.New("查询最大用户表情分类错误"))
		return
	}

	sortNum := 1
	if cmodel != nil {
		sortNum = cmodel.SortNum + 1
	}

	var cats []struct {
		Category string
	}
	_, err = s.db.session.
		Select("category").
		From("sticker_store").
		Where("is_gone=0").
		Load(&cats)
	if err != nil {
		s.Error("查询表情商店分类错误", zap.Error(err))
		commit(errors.New("查询表情商店分类错误"))
		return
	}

	// 给 sort_num 赋值时保持稳定顺序，避免不同机器/不同时间导致插入顺序不一致。
	//（sort_num 的展示顺序最终由 sticker_user_category.sort_num 决定）
	catNames := make([]string, 0, len(cats))
	for _, c := range cats {
		if c.Category != "" {
			catNames = append(catNames, c.Category)
		}
	}
	sort.Strings(catNames)

	for _, category := range catNames {
		// 已添加过的分类不重复插入（避免 sort_num 混乱）
		model, err := s.db.queryUserCategoryWithCategory(uid, category)
		if err != nil {
			s.Error("查询用户分类表情错误", zap.Error(err))
			commit(errors.New("查询用户分类表情错误"))
			return
		}
		if model != nil {
			continue
		}
		err = s.db.insertUserCategory(&categoryModel{
			UID:      uid,
			SortNum:  sortNum,
			Category: category,
		})
		if err != nil {
			s.Error("注册用户添加表情分类错误", zap.Error(err))
			commit(err)
			return
		}
		sortNum++
	}
	commit(nil)
}
