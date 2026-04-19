package moments

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/testutil"
	"github.com/stretchr/testify/assert"
)

func TestPublish(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddFriend(testutil.UID, &user.FriendReq{UID: testutil.UID, ToUID: "111"})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: testutil.UID, Name: "test"})
	assert.NoError(t, err)
	imgs := make([]string, 0)
	imgs = append(imgs, "111")
	remindUids := make([]string, 0)
	remindUids = append(remindUids, "111")
	req, _ := http.NewRequest("POST", "/v1/moments", bytes.NewReader([]byte(util.ToJson(map[string]interface{}{
		"video_path":       "xxll",
		"video_cover_path": "12ss",
		"text":             "动态内容",
		"privacy_type":     "public",
		"Address":          "北京",
		"longitude":        "112.12334",
		"latitude":         "23.234322",
		"imgs":             imgs,
		"privacy_uids":     make([]string, 0),
		"remind_uids":      remindUids,
	}))))

	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteMoment(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.db.insert(&model{
		MomentNo:       "12133",
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态",
		Publisher:      testutil.UID,
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/v1/moments/%s", "12133"), nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDetail(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	momentNo := "12133"
	err = m.db.insert(&model{
		MomentNo:       momentNo,
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态",
		Publisher:      testutil.UID,
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		UID:        testutil.UID,
		Name:       "111",
		HandleType: 0,
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		Content:    "这是评论内容",
		UID:        testutil.UID,
		Name:       "111",
		HandleType: 1,
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:       momentNo,
		Content:        "这是评论内容",
		UID:            testutil.UID,
		Name:           "111",
		HandleType:     1,
		ReplyCommentID: "1",
		ReplyUID:       "12333",
		ReplyName:      "222",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/moments/%s", momentNo), nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"moment_no":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher_name":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"privacy_type":"public"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_path":"xxx"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_cover_path":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"text":"这是一条动态"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"address":"上海"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"longitude":"13322"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"latitude":"11.222"`))
}

func TestList(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddFriend(testutil.UID, &user.FriendReq{UID: testutil.UID, ToUID: "111"})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: testutil.UID, Name: "test"})
	assert.NoError(t, err)
	err = m.userService.AddFriend(testutil.UID, &user.FriendReq{UID: testutil.UID, ToUID: "222"})
	assert.NoError(t, err)
	momentNo := "12133"
	err = m.db.insert(&model{
		MomentNo:       momentNo,
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态22",
		Publisher:      "222",
		PublisherName:  "222",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	err = m.db.insert(&model{
		MomentNo:       "121211222",
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态11",
		Publisher:      "111",
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	_, err = m.momentUserDB.insert(&momentUserModel{UID: testutil.UID, MomentNo: momentNo, Publisher: "222"})
	assert.NoError(t, err)
	_, err = m.momentUserDB.insert(&momentUserModel{UID: testutil.UID, MomentNo: "121211222", Publisher: "111"})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		UID:        testutil.UID,
		Name:       "111",
		HandleType: 0,
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		Content:    "这是评论内容",
		UID:        testutil.UID,
		Name:       "111",
		HandleType: 1,
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:       momentNo,
		Content:        "这是评论内容",
		UID:            testutil.UID,
		Name:           "111",
		HandleType:     1,
		ReplyCommentID: "1",
		ReplyUID:       "12333",
		ReplyName:      "222",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", "/v1/moments?page_index=1&page_size=20", nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"moment_no":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher_name":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"privacy_type":"public"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_path":"xxx"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_cover_path":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"text":"这是一条动态11"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"address":"上海"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"longitude":"13322"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"latitude":"11.222"`))
}

func TestLike(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: testutil.UID, Name: "test"})
	assert.NoError(t, err)
	momentNo := "12133"
	err = m.db.insert(&model{
		MomentNo:       momentNo,
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态11",
		Publisher:      "111",
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/moments/%s/like", momentNo), nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUnLike(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	momentNo := "12133"
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		UID:        testutil.UID,
		HandleType: 0,
		Name:       "111",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/v1/moments/%s/unlike", momentNo), nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAddComment(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: testutil.UID, Name: "test"})
	assert.NoError(t, err)
	momentNo := "12133"
	err = m.db.insert(&model{
		MomentNo:       momentNo,
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态11",
		Publisher:      "111",
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("POST", fmt.Sprintf("/v1/moments/%s/comments", momentNo), bytes.NewReader([]byte(util.ToJson(map[string]interface{}{
		"content":          "好的",
		"reply_uid":        testutil.UID,
		"reply_name":       "111",
		"reply_comment_id": "1",
	}))))
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
func TestDeleteComment(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	momentNo := "12133"
	id, err := m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		Content:    "这是评论内容",
		UID:        testutil.UID,
		Name:       "111",
		HandleType: 1,
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/v1/moments/%s/comments/%d", momentNo, id), nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListWithUID(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	m := New(ctx)
	m.Route(s.GetRoute())
	//清除数据
	err := testutil.CleanAllTables(ctx)
	assert.NoError(t, err)
	err = m.userService.AddFriend(testutil.UID, &user.FriendReq{UID: testutil.UID, ToUID: "111"})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: testutil.UID, Name: "test"})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: "111", Name: "111"})
	assert.NoError(t, err)
	err = m.userService.AddUser(&user.AddUserReq{UID: "222", Name: "222"})
	assert.NoError(t, err)
	momentNo := "12133"
	//添加动态
	err = m.db.insert(&model{
		MomentNo:       momentNo,
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态11",
		Publisher:      "111",
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	err = m.db.insert(&model{
		MomentNo:       "112233",
		VideoPath:      "xxx",
		VideoCoverPath: "111",
		Content:        "这是一条动态22",
		Publisher:      "111",
		PublisherName:  "111",
		PrivacyType:    "public",
		Address:        "上海",
		Latitude:       "11.222",
		Longitude:      "13322",
	})
	assert.NoError(t, err)
	//添加动态的好友关系
	_, err = m.momentUserDB.insert(&momentUserModel{
		UID:       "111",
		MomentNo:  momentNo,
		Publisher: "111",
	})
	assert.NoError(t, err)
	_, err = m.momentUserDB.insert(&momentUserModel{
		UID:       "111",
		MomentNo:  "112233",
		Publisher: "111",
	})
	assert.NoError(t, err)
	// 添加评论
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   momentNo,
		UID:        testutil.UID,
		HandleType: 0,
		Name:       "111",
	})
	assert.NoError(t, err)

	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   "112233",
		UID:        "222",
		HandleType: 1,
		Name:       "222",
		Content:    "uid为222的评论动态",
	})
	assert.NoError(t, err)
	_, err = m.commentDB.insert(&commentModel{
		MomentNo:   "112233",
		UID:        "111",
		HandleType: 1,
		Name:       "111",
		Content:    "uid为111的评论动态",
	})
	assert.NoError(t, err)

	req, _ := http.NewRequest("GET", "/v1/moments?page_index=1&page_size=20&uid=111", nil)
	w := httptest.NewRecorder()
	req.Header.Set("token", testutil.Token)
	s.GetRoute().ServeHTTP(w, req)
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"moment_no":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher":`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"publisher_name":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"privacy_type":"public"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_path":"xxx"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"video_cover_path":"111"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"text":"这是一条动态22"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"address":"上海"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"longitude":"13322"`))
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"latitude":"11.222"`))
}
