package hotline

import (
	"testing"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/testutil"
	"github.com/stretchr/testify/assert"
)

func TestGetObjValue(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	rs := NewRouteService(ctx)

	message := &config.MessageResp{
		ChannelID: "1234",
		Header: config.MsgHeader{
			RedDot: 1,
		},
		Payload: []byte(`{"text":"测试"}`),
	}
	result := rs.getObjValue(*message, "payload.text", true)
	assert.Equal(t, "测试", result)

}

func TestParseExpression(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	rs := NewRouteService(ctx)

	result := rs.parseExpression("(A&B)||(C|D)")
	assert.Equal(t, "测试", result)
	panic("")

}

func TestAssertExpression(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	rs := NewRouteService(ctx)

	expression := &Expression{
		ExpType: ExpTypeAnd,
		Children: []*Expression{
			{
				Tag:    "A",
				Result: true,
			},
			{
				ExpType: ExpTypeAnd,
				Children: []*Expression{
					{
						Tag:    "B1",
						Result: true,
					},
					{
						Tag:    "B2",
						Result: true,
					},
				},
			},
			{
				ExpType: ExpTypeOr,
				Children: []*Expression{
					{
						Tag:    "C1",
						Result: true,
					},
					{
						Tag:    "C2",
						Result: false,
					},
				},
			},
		},
	}

	result := rs.assertExpression(expression)
	assert.Equal(t, true, result)
}

func TestGetTriggerRule(t *testing.T) {
	s, ctx := testutil.NewTestServer()
	f := New(ctx)
	f.Route(s.GetRoute())

	rs := NewRouteService(ctx)

	routeCtx := &RouteContext{
		Message: &config.MessageResp{
			ChannelID: "1234",
			Header: config.MsgHeader{
				RedDot: 1,
			},
			Payload: []byte(`{"text":"测试"}`),
		},
	}

	rules := make([]*ruleDetailModel, 0)
	rules = append(rules, &ruleDetailModel{
		ruleModel: ruleModel{
			Expression: "(A&B)&&(C&D)",
		},
		Conditions: []*conditionModel{
			{
				Tag:       "A",
				Field:     "message.payload.text",
				Value:     "测试",
				Condition: string(ActionEqual),
			},
			{
				Tag:       "B",
				Field:     "message.channelid",
				Value:     "1234",
				Condition: string(ActionEqual),
			},
			{
				Tag:       "C",
				Field:     "message.header.reddot",
				Value:     "1",
				Condition: string(ActionEqual),
			},
			{
				Tag:       "D",
				Field:     "message.header.nopersist",
				Value:     "0",
				Condition: string(ActionEqual),
			},
		},
	})

	result := rs.getTriggerRule(rules, routeCtx)
	assert.NotNil(t, result)
}
