package general

import (
	"reflect"
	"testing"
)

func TestExpressionLags(t *testing.T) {
	iteration := &ExpressionIteration{
		Fields:    []ExpressionField{{Name: "x"}, {Name: "y"}},
		Upstreams: map[string]string{"c": "counter"},
		Bindings: []ExpressionBinding{
			{Name: "early", Expr: "lag(c, 2)[0] + lag( x ,1 )"},
			{Name: "broken", Expr: "lag(c, 3"},
		},
		Outputs: []string{
			"where(step > 3, lag(c, (4))[0], -1) + min(lag(c, 1)[0], 2)",
			"lag(y, -1) + lag(y, +2) + lag(y, 2.7) + lag(y, step) + lag(y, 1 + 1) + lag(c) + lag(slice(c, 0, 1), 1)",
			`lag(c, -x) + lag(c, "2") + lag(c, !2)`,
		},
	}
	want := []ExpressionLag{
		{Name: "c", Row: 2, Written: true, Call: "lag(c, 2)"},
		{Name: "x", Row: 1, Written: true, Call: "lag( x ,1 )"},
		{Name: "c", Row: 4, Written: true, Call: "lag(c, (4))"},
		{Name: "c", Row: 1, Written: true, Call: "lag(c, 1)"},
		{Name: "y", Row: -1, Written: true, Call: "lag(y, -1)"},
		{Name: "y", Row: 2, Written: true, Call: "lag(y, +2)"},
		{Name: "y", Row: 2, Written: true, Call: "lag(y, 2.7)"},
		{Name: "y", Call: "lag(y, step)"},
		{Name: "y", Call: "lag(y, 1 + 1)"},
		{Name: "", Row: 1, Written: true, Call: "lag(slice(c, 0, 1), 1)"},
		{Name: "c", Call: "lag(c, -x)"},
		{Name: "c", Call: `lag(c, "2")`},
		{Name: "c", Call: "lag(c, !2)"},
	}
	if got := iteration.Lags(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if got := (&ExpressionIteration{Outputs: []string{"x + 1"}}).Lags(); got != nil {
		t.Errorf("an expression with no lag gave %+v", got)
	}
}
