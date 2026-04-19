package moments

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHideMy(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := NewSetting(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  "11",
		Name: "11",
	})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  testutil.UID,
		Name: "11",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("PUT", "/v1/moments/setting/hidemy/11/1", nil)

	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHideHis(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := NewSetting(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  "11",
		Name: "11",
	})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  testutil.UID,
		Name: "11",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("PUT", "/v1/moments/setting/hidehis/11/1", nil)

	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSettingDetail(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := NewSetting(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  "11",
		Name: "11",
	})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{
		UID:  testutil.UID,
		Name: "11",
	})
	assert.NoError(t, err)
	err = m.settingDB.insert(&settingModel{
		UID:       testutil.UID,
		ToUID:     "11",
		IsHideMy:  1,
		IsHideHis: 1,
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", "/v1/moments/setting/11", nil)

	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"is_hide_my":1`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"is_hide_his":1`))
}
