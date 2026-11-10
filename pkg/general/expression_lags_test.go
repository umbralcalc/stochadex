package general

import (
	"reflect"
	"testing"
)

func TestExpressionLiteralLags(t *testing.T) {
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
		},
	}
	want := []ExpressionLag{
		{Name: "c", Row: 2, Call: "lag(c, 2)"},
		{Name: "x", Row: 1, Call: "lag( x ,1 )"},
		{Name: "c", Row: 4, Call: "lag(c, (4))"},
		{Name: "c", Row: 1, Call: "lag(c, 1)"},
		{Name: "y", Row: -1, Call: "lag(y, -1)"},
		{Name: "y", Row: 2, Call: "lag(y, +2)"},
		{Name: "y", Row: 2, Call: "lag(y, 2.7)"},
	}
	if got := iteration.LiteralLags(); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if got := (&ExpressionIteration{Outputs: []string{"lag(c, -x)", "lag(c, \"2\")", "lag(c, !2)"}}).LiteralLags(); got != nil {
		t.Errorf("rows that are not written numbers gave %+v", got)
	}
}
