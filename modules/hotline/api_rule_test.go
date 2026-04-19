package hotline

import "testing"

func TestGetRuleExpression(t *testing.T) {
	req := &ruleReq{}
	req.TermType = 1
	req.Terms = []*term{
		{
			Field: "tet",
		},
		{
			Field: "zzz",
		},
	}
	req.Rules = []*ruleReq{
		{
			TermType: 0,
			Terms: []*term{
				{
					Field: "12",
				},
				{
					Field: "ksd",
				},
			},
			Rules: []*ruleReq{
				{
					TermType: 1,
					Terms: []*term{
						{
							Field: "dad",
						},
						{
							Field: "zdsd",
						},
					},
				},
			},
		},
	}

	exp := getRuleExpression(req)

	panic(exp)
}
