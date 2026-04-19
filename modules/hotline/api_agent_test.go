package hotline

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/testutil"
	"github.com/jordan-wright/email"
	"github.com/stretchr/testify/assert"
)

func TestAgentAdd(t *testing.T) {

	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	appID := "test"

	err := f.agentDB.insert(&agentModel{
		AppID:  appID,
		UID:    testutil.UID,
		Role:   "admin",
		Status: 1,
	})
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/v1/hotline/agents", bytes.NewBuffer([]byte(util.ToJson(map[string]interface{}{
		"uid":      "xxx",
		"name":     "est",
		"skill_no": "sdsdsd",
		"role":     "admin",
	}))))
	req.Header.Set("token", testutil.Token)
	req.Header.Set("appid", appID)
	assert.NoError(t, err)
	s.GetRoute().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `{"status":200}`)
}

func TestAgentPage(t *testing.T) {

	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	appID := "test"

	err := f.agentDB.insert(&agentModel{
		AppID: appID,
		UID:   "12233",
		Role:  "admin",
	})
	assert.NoError(t, err)

	err = f.agentDB.insert(&agentModel{
		AppID: appID,
		UID:   "zdsdsd",
		Role:  "manager",
	})
	assert.NoError(t, err)

	err = f.userService.AddUser(&user.AddUserReq{
		UID:      "12233",
		Name:     "test",
		Username: "15900000001",
	})
	assert.NoError(t, err)

	// 技能组
	err = f.groupDB.insert(&groupModel{
		AppID:     appID,
		GroupNo:   "skill",
		GroupType: GroupTypeKill.Int(),
		Name:      "新手",
	})
	assert.NoError(t, err)

	err = f.groupDB.insertMember(&memberModel{
		AppID:   appID,
		GroupNo: "skill",
		UID:     "12233",
	})
	assert.NoError(t, err)

	// 群组
	err = f.groupDB.insert(&groupModel{
		AppID:     appID,
		GroupNo:   "commongroup",
		GroupType: GroupTypeCommon.Int(),
		Name:      "测试群",
	})
	assert.NoError(t, err)
	err = f.groupDB.insertMember(&memberModel{
		AppID:   appID,
		GroupNo: "commongroup",
		UID:     "12233",
	})
	assert.NoError(t, err)
	// 群组
	err = f.groupDB.insert(&groupModel{
		AppID:     appID,
		GroupNo:   "commongroup2",
		GroupType: GroupTypeCommon.Int(),
		Name:      "测试群2",
	})
	assert.NoError(t, err)
	err = f.groupDB.insertMember(&memberModel{
		AppID:   appID,
		GroupNo: "commongroup2",
		UID:     "12233",
	})
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/v1/hotline/agents", nil)
	req.Header.Set("token", testutil.Token)
	req.Header.Set("appid", appID)
	assert.NoError(t, err)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"群组1"`)
	time.Sleep(time.Millisecond * 200)
}

func TestTempl(t *testing.T) {
	tpl := template.Must(template.ParseFiles("../../../assets/webroot/hotline/invite_tpl.html"))

	buff := bytes.NewBuffer(make([]byte, 10240))
	err := tpl.ExecuteTemplate(buff, "hotline/invite_tpl.html", map[string]interface{}{})
	assert.NoError(t, err)

	fmt.Println("--->", buff.String())
	panic("dsdd")
}

func TestSendEmail111(t *testing.T) {
	tpl := template.Must(template.ParseFiles("../../../assets/webroot/hotline/invite_tpl.html"))

	buff := bytes.NewBuffer(make([]byte, 0))
	err := tpl.ExecuteTemplate(buff, "hotline/invite_tpl.html", map[string]interface{}{})
	assert.NoError(t, err)

	e := email.NewEmail()
	e.From = "support@tgo.ai"
	e.To = []string{"xxxx@qq.com"}
	e.Subject = "你好1，激活您的账号"
	e.HTML = []byte(strings.TrimSpace(buff.String()))

	err = e.Send("smtp.exmail.qq.com:25", smtp.PlainAuth("", "support@tgo.ai", "LnAZnaCmFLKHmUGe1", "smtp.exmail.qq.com"))
	assert.NoError(t, err)

	fmt.Println(string(e.HTML))
	panic("")

}
