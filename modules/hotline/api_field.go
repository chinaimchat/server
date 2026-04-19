package hotline

import (
	limlog "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

func (h *Hotline) fieldList(c *wkhttp.Context) {
	fields, err := h.fieldDB.queryAll()
	if err != nil {
		c.ResponseErrorf("查询field失败！", err)
		return
	}
	fielddMap := map[string][]*fieldResp{}
	if len(fields) > 0 {
		for _, field := range fields {
			resps := fielddMap[field.Group]
			if resps == nil {
				resps = make([]*fieldResp, 0)
			}
			resps = append(resps, newFieldResp(field))
			fielddMap[field.Group] = resps
		}
	}

	fieldGroups := make([]*fieldGroup, 0)
	for key, fields := range fielddMap {
		fieldGroups = append(fieldGroups, &fieldGroup{
			Group:     key,
			GroupName: h.getGroupName(key),
			Fields:    fields,
		})
	}
	// resps := make([]*fieldGroup, 0, len(fields))
	// if len(fields) > 0 {
	// 	for _, field := range fields {
	// 		resps = append(resps, newFieldResp(field))
	// 	}
	// }
	c.Response(fieldGroups)
}

func (h *Hotline) getGroupName(group string) string {
	switch group {
	case "message":
		return "消息"
	case "visitor":
		return "访客"
	case "session":
		return "会话"
	case "channel":
		return "频道"
	}
	return group
}

type fieldGroup struct {
	Group     string       `json:"group"`
	GroupName string       `json:"group_name"`
	Fields    []*fieldResp `json:"fields"`
}

type fieldResp struct {
	Field      string                   `json:"field"`
	Name       string                   `json:"name"`
	Type       string                   `json:"type"`
	Datasource string                   `json:"datasource"`
	Options    []map[string]interface{} `json:"options"`
	Symbols    []map[string]interface{} `json:"symbols"`
}

func newFieldResp(m *fieldModel) *fieldResp {
	var symbolsMap []map[string]interface{}
	if len(m.Symbols) > 0 {
		if err := util.ReadJsonByByte([]byte(m.Symbols), &symbolsMap); err != nil {
			limlog.Warn("解析符号字段失败！", zap.Error(err))
		}
	}
	var optionsMap []map[string]interface{}
	if len(m.Field) > 0 {
		if err := util.ReadJsonByByte([]byte(m.Options), &optionsMap); err != nil {
			limlog.Warn("解析options字段失败！", zap.Error(err))
		}
	}
	return &fieldResp{
		Field:      m.Field,
		Name:       m.Name,
		Type:       m.Type,
		Datasource: m.Datasource,
		Options:    optionsMap,
		Symbols:    symbolsMap,
	}
}
