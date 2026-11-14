package search

import (
	"fmt"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
)

type ApplicationQuery struct {
	TestJoin `search:"type:left;on:id:receipt_id;table:receipt_goods;join:receipts"`
	ApplicationOrder
	ID      string    `search:"type:icontains;column:id;table:receipt" form:"id"`
	Domain  string    `search:"type:icontains;column:domain;table:receipt" form:"domain"`
	Version string    `search:"type:exact;column:version;table:receipt" form:"version"`
	Status  []int     `search:"type:in;column:status;table:receipt" form:"status"`
	Start   time.Time `search:"type:gte;column:created_at;table:receipt" form:"start"`
	End     time.Time `search:"type:lte;column:created_at;table:receipt" form:"end"`
	NotNeed string    `search:"-"`
}

type ApplicationOrder struct {
	IDOrder string `search:"type:order;column:id;table:receipt" form:"id_order"`
}

type TestJoin struct {
	PaymentAccount string `search:"type:icontains;column:payment_account;table:receipts" form:"payment_account"`
}

func TestResolveSearchQuery(t *testing.T) {
	// Only pass t into top-level convey.Convey calls
	convey.Convey("Given some integer with a starting value", t, func() {
		d := ApplicationQuery{
			ID:             "aaa",
			Domain:         "bbb",
			Version:        "ccc",
			Status:         []int{1, 2},
			Start:          time.Now().Add(-8 * time.Hour),
			End:            time.Now(),
			IDOrder:        "desc",
			PaymentAccount: "1212",
		}
		condition := mustResolve("mysql", d)
		fmt.Println(condition)
	})
}

// JSONContainsQuery covers nullable and non-nullable JSON filters.
type JSONContainsQuery struct {
	// json_contains：精确包含
	AgentCodeStrict string `search:"type:json_contains;column:agent_codes;table:tool_script_templates" form:"agentCodeStrict"`
	// json_contains_or_null：三态白名单
	AgentCodeWhitelist string `search:"type:json_contains_or_null;column:agent_codes;table:tool_script_templates" form:"agentCodeWhitelist"`
	// 与其他 type 共存校验
	Name string `search:"type:exact;column:name;table:tool_script_templates" form:"name"`
}

func TestJsonContains_MySQLZeroValueSkip(t *testing.T) {
	convey.Convey("Zero fields produce no WHERE clauses", t, func() {
		q := JSONContainsQuery{}
		cond := mustResolve(MySQL, q)

		convey.So(len(cond.Where), convey.ShouldEqual, 0)
	})
}

func TestJsonContains_MySQLSQLCorrectness(t *testing.T) {
	convey.Convey("MySQL JSON filter uses JSON_CONTAINS", t, func() {
		q := JSONContainsQuery{AgentCodeStrict: "ai-coder"}
		cond := mustResolve(MySQL, q)

		expected := "JSON_CONTAINS(`tool_script_templates`.`agent_codes`, JSON_QUOTE(?))"
		_, exist := cond.Where[expected]
		convey.So(exist, convey.ShouldBeTrue)
		convey.So(cond.Where[expected], convey.ShouldResemble, []any{"ai-coder"})
	})

	convey.Convey("MySQL nullable JSON filter includes all three states", t, func() {
		q := JSONContainsQuery{AgentCodeWhitelist: "ai-reviewer"}
		cond := mustResolve(MySQL, q)

		expected := "(`tool_script_templates`.`agent_codes` IS NULL OR " +
			"`tool_script_templates`.`agent_codes` = JSON_ARRAY() OR " +
			"JSON_CONTAINS(`tool_script_templates`.`agent_codes`, JSON_QUOTE(?)))"
		_, exist := cond.Where[expected]
		convey.So(exist, convey.ShouldBeTrue)
		convey.So(cond.Where[expected], convey.ShouldResemble, []any{"ai-reviewer"})
	})
}

func TestJsonContains_PostgresSQLCorrectness(t *testing.T) {
	convey.Convey("Postgres JSON filter uses containment", t, func() {
		q := JSONContainsQuery{AgentCodeStrict: "ai-coder"}
		cond := mustResolve(Postgres, q)

		expected := "tool_script_templates.agent_codes @> ?::jsonb"
		args, exist := cond.Where[expected]
		convey.So(exist, convey.ShouldBeTrue)
		// 标量字符串 → JSON 字面量 "ai-coder"
		convey.So(args, convey.ShouldResemble, []any{`"ai-coder"`})
	})

	convey.Convey("Postgres nullable JSON filter includes all three states", t, func() {
		q := JSONContainsQuery{AgentCodeWhitelist: "ai-reviewer"}
		cond := mustResolve(Postgres, q)

		expected := "(tool_script_templates.agent_codes IS NULL OR " +
			"tool_script_templates.agent_codes = '[]'::jsonb OR " +
			"tool_script_templates.agent_codes @> ?::jsonb)"
		args, exist := cond.Where[expected]
		convey.So(exist, convey.ShouldBeTrue)
		convey.So(args, convey.ShouldResemble, []any{`"ai-reviewer"`})
	})
}

func TestJsonContains_CoexistWithOtherTypes(t *testing.T) {
	convey.Convey("JSON and exact filters produce independent clauses", t, func() {
		q := JSONContainsQuery{
			AgentCodeStrict:    "a1",
			AgentCodeWhitelist: "a2",
			Name:               "tpl-x",
		}
		cond := mustResolve(MySQL, q)

		convey.So(len(cond.Where), convey.ShouldEqual, 3)

		_, hasStrict := cond.Where["JSON_CONTAINS(`tool_script_templates`.`agent_codes`, JSON_QUOTE(?))"]
		convey.So(hasStrict, convey.ShouldBeTrue)

		_, hasWhitelist := cond.Where["(`tool_script_templates`.`agent_codes` IS NULL OR "+
			"`tool_script_templates`.`agent_codes` = JSON_ARRAY() OR "+
			"JSON_CONTAINS(`tool_script_templates`.`agent_codes`, JSON_QUOTE(?)))"]
		convey.So(hasWhitelist, convey.ShouldBeTrue)

		_, hasExact := cond.Where["`tool_script_templates`.`name` = ?"]
		convey.So(hasExact, convey.ShouldBeTrue)
	})
}

func TestJsonContains_BuildClauseHelpers(t *testing.T) {
	convey.Convey("BuildJSONContainsClause MySQL", t, func() {
		sql, args, err := BuildJSONContainsClause(MySQL, "agent_codes", "ai-coder")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "JSON_CONTAINS(agent_codes, JSON_QUOTE(?))")
		convey.So(args, convey.ShouldResemble, []any{"ai-coder"})
	})

	convey.Convey("BuildJSONContainsClause Postgres", t, func() {
		sql, args, err := BuildJSONContainsClause(Postgres, "agent_codes", "ai-coder")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "agent_codes @> ?::jsonb")
		convey.So(args, convey.ShouldResemble, []any{`"ai-coder"`})
	})

	convey.Convey("MySQL nullable JSON clause", t, func() {
		sql, args, err := BuildJSONContainsOrNullClause(MySQL, "agent_codes", "ai-coder")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "(agent_codes IS NULL OR agent_codes = JSON_ARRAY() OR JSON_CONTAINS(agent_codes, JSON_QUOTE(?)))")
		convey.So(args, convey.ShouldResemble, []any{"ai-coder"})
	})

	convey.Convey("Postgres nullable JSON clause", t, func() {
		sql, args, err := BuildJSONContainsOrNullClause(Postgres, "agent_codes", "ai-coder")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "(agent_codes IS NULL OR agent_codes = '[]'::jsonb OR agent_codes @> ?::jsonb)")
		convey.So(args, convey.ShouldResemble, []any{`"ai-coder"`})
	})

	convey.Convey("Nullable JSON clause supports a table prefix", t, func() {
		sql, args, err := BuildJSONContainsOrNullClause(MySQL, "t.agent_codes", "ai-coder")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "(t.agent_codes IS NULL OR t.agent_codes = JSON_ARRAY() OR JSON_CONTAINS(t.agent_codes, JSON_QUOTE(?)))")
		convey.So(args, convey.ShouldResemble, []any{"ai-coder"})
	})

	convey.Convey("Empty driver defaults to MySQL syntax", t, func() {
		sql, _, err := BuildJSONContainsClause("", "agent_codes", "x")
		convey.So(err, convey.ShouldBeNil)
		convey.So(sql, convey.ShouldEqual, "JSON_CONTAINS(agent_codes, JSON_QUOTE(?))")
	})
}

// The view keeps existing SQL assertions independent of the result's layout.
type conditionView struct {
	Where map[string][]any
	Order []string
	Join  []Join
}

func mustResolve(driver string, q any) conditionView {
	c, err := Resolve(driver, q)
	if err != nil {
		panic(err)
	}
	v := conditionView{Where: make(map[string][]any), Order: c.Order, Join: c.Join}
	for _, clause := range c.Where {
		v.Where[clause.SQL] = clause.Args
	}
	return v
}
