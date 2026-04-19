package user

import (
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

type privilegeDB struct {
	session *dbr.Session
}

func newPrivilegeDB(session *dbr.Session) *privilegeDB {
	return &privilegeDB{session: session}
}

type privilegeModel struct {
	UID                  string
	GroupManageOn        int
	AllMemberInviteOn    int
	MutualDeletePersonOn int
	MutualDeleteGroupOn  int
	db.BaseModel
}

type privilegeUserRow struct {
	UID                  string `db:"uid" json:"uid"`
	Name                 string `db:"name" json:"name"`
	Username             string `db:"username" json:"username"`
	Zone                 string `db:"zone" json:"zone"`
	Phone                string `db:"phone" json:"phone"`
	Avatar               string `json:"avatar"`
	GroupManageOn        int    `db:"group_manage_on" json:"group_manage_on"`
	AllMemberInviteOn    int    `db:"all_member_invite_on" json:"all_member_invite_on"`
	MutualDeletePersonOn int    `db:"mutual_delete_person_on" json:"mutual_delete_person_on"`
	MutualDeleteGroupOn  int    `db:"mutual_delete_group_on" json:"mutual_delete_group_on"`
	CreatedAt            string `db:"created_at" json:"created_at"`
}

func (p *privilegeDB) list(limit int, offset int, keyword string) ([]*privilegeUserRow, error) {
	var rows []*privilegeUserRow
	q := p.session.Select("user.uid,user.name,user.username,user.zone,user.phone,ifnull(user_privilege.group_manage_on,1) as group_manage_on,ifnull(user_privilege.all_member_invite_on,0) as all_member_invite_on,ifnull(user_privilege.mutual_delete_person_on,1) as mutual_delete_person_on,ifnull(user_privilege.mutual_delete_group_on,1) as mutual_delete_group_on,DATE_FORMAT(user_privilege.created_at,'%Y-%m-%d %H:%i:%s') as created_at").
		From("user_privilege").
		Join("user", "user_privilege.uid=user.uid")
	if strings.TrimSpace(keyword) != "" {
		kw := "%" + strings.TrimSpace(keyword) + "%"
		q = q.Where("user.uid like ? or user.username like ? or user.phone like ? or user.name like ?", kw, kw, kw, kw)
	}
	_, err := q.OrderDir("user_privilege.created_at", false).Limit(uint64(limit)).Offset(uint64(offset)).Load(&rows)
	if rows == nil {
		rows = make([]*privilegeUserRow, 0)
	}
	for _, r := range rows {
		r.Avatar = ""
	}
	return rows, err
}

func (p *privilegeDB) count(keyword string) (int64, error) {
	var total int64
	q := p.session.Select("count(*)").From("user_privilege").Join("user", "user_privilege.uid=user.uid")
	if strings.TrimSpace(keyword) != "" {
		kw := "%" + strings.TrimSpace(keyword) + "%"
		q = q.Where("user.uid like ? or user.username like ? or user.phone like ? or user.name like ?", kw, kw, kw, kw)
	}
	_, err := q.Load(&total)
	return total, err
}

func (p *privilegeDB) queryByUID(uid string) (*privilegeModel, error) {
	var m *privilegeModel
	_, err := p.session.Select("*").From("user_privilege").Where("uid=?", uid).Load(&m)
	return m, err
}

func (p *privilegeDB) insert(m *privilegeModel) error {
	_, err := p.session.InsertInto("user_privilege").Columns("uid", "group_manage_on", "all_member_invite_on", "mutual_delete_person_on", "mutual_delete_group_on").
		Values(m.UID, m.GroupManageOn, m.AllMemberInviteOn, m.MutualDeletePersonOn, m.MutualDeleteGroupOn).Exec()
	return err
}

func (p *privilegeDB) deleteByUID(uid string) error {
	_, err := p.session.DeleteFrom("user_privilege").Where("uid=?", uid).Exec()
	return err
}

func (p *privilegeDB) updateSwitch(uid string, field string, value int) error {
	_, err := p.session.Update("user_privilege").Set(field, value).Where("uid=?", uid).Exec()
	return err
}

func (p *privilegeDB) searchUser(keyword string) (*privilegeUserRow, error) {
	var row *privilegeUserRow
	kw := strings.TrimSpace(keyword)
	if kw == "" {
		return nil, nil
	}
	_, err := p.session.Select("uid,name,username,zone,phone").From("user").
		Where("uid=?", kw).
		Load(&row)
	if err != nil {
		return nil, err
	}
	if row != nil && row.UID != "" {
		return row, nil
	}
	var rows []*privilegeUserRow
	like := "%" + kw + "%"
	_, err = p.session.Select("uid,name,username,zone,phone").From("user").
		Where("username like ? or phone like ? or name like ?", like, like, like).
		OrderDir("created_at", false).Limit(1).Load(&rows)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		rows[0].Avatar = ""
		return rows[0], nil
	}
	return nil, nil
}
