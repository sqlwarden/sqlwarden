package access

import (
	"context"
	"fmt"
	"time"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/uptrace/bun"
)

type casbinRule struct {
	bun.BaseModel `bun:"table:casbin_rules"`
	ID            int64  `bun:",pk,autoincrement"`
	Ptype         string `bun:",notnull"`
	V0            string `bun:",notnull,default:''"`
	V1            string `bun:",notnull,default:''"`
	V2            string `bun:",notnull,default:''"`
	V3            string `bun:",notnull,default:''"`
	V4            string `bun:",notnull,default:''"`
	V5            string `bun:",notnull,default:''"`
}

type bunAdapter struct {
	db *bun.DB
}

func newBunAdapter(db *bun.DB) *bunAdapter {
	return &bunAdapter{db: db}
}

func dbCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// LoadPolicy loads all policy rules from the database.
func (a *bunAdapter) LoadPolicy(m model.Model) error {
	ctx, cancel := dbCtx()
	defer cancel()

	var rules []casbinRule
	err := a.db.NewSelect().Model(&rules).Scan(ctx)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		line := fmt.Sprintf("%s, %s, %s, %s, %s, %s, %s",
			rule.Ptype, rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5)
		persist.LoadPolicyLine(line, m)
	}

	return nil
}

// SavePolicy saves all policy rules to the database, replacing existing rules.
func (a *bunAdapter) SavePolicy(m model.Model) error {
	ctx, cancel := dbCtx()
	defer cancel()

	_, err := a.db.NewDelete().Model((*casbinRule)(nil)).Where("1=1").Exec(ctx)
	if err != nil {
		return err
	}

	var rules []casbinRule

	for ptype, ast := range m["p"] {
		for _, rule := range ast.Policy {
			rules = append(rules, ruleToRow(ptype, rule))
		}
	}

	for ptype, ast := range m["g"] {
		for _, rule := range ast.Policy {
			rules = append(rules, ruleToRow(ptype, rule))
		}
	}

	if len(rules) == 0 {
		return nil
	}

	_, err = a.db.NewInsert().Model(&rules).Exec(ctx)
	return err
}

// AddPolicy adds a policy rule to the database.
func (a *bunAdapter) AddPolicy(sec, ptype string, rule []string) error {
	ctx, cancel := dbCtx()
	defer cancel()

	row := ruleToRow(ptype, rule)
	_, err := a.db.NewInsert().Model(&row).Exec(ctx)
	return err
}

// AddPolicies adds multiple policy rules to the database.
func (a *bunAdapter) AddPolicies(sec, ptype string, rules [][]string) error {
	ctx, cancel := dbCtx()
	defer cancel()

	rows := make([]casbinRule, len(rules))
	for i, rule := range rules {
		rows[i] = ruleToRow(ptype, rule)
	}

	_, err := a.db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

// RemovePolicy removes a policy rule from the database.
func (a *bunAdapter) RemovePolicy(sec, ptype string, rule []string) error {
	ctx, cancel := dbCtx()
	defer cancel()

	padded := padRule(rule)
	q := a.db.NewDelete().Model((*casbinRule)(nil)).Where("ptype = ?", ptype)
	for i, v := range padded {
		q = q.Where(fmt.Sprintf("v%d = ?", i), v)
	}

	_, err := q.Exec(ctx)
	return err
}

// RemovePolicies removes multiple policy rules from the database.
func (a *bunAdapter) RemovePolicies(sec, ptype string, rules [][]string) error {
	for _, rule := range rules {
		if err := a.RemovePolicy(sec, ptype, rule); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFilteredPolicy removes policy rules that match the filter.
func (a *bunAdapter) RemoveFilteredPolicy(sec, ptype string, fieldIndex int, fieldValues ...string) error {
	ctx, cancel := dbCtx()
	defer cancel()

	q := a.db.NewDelete().Model((*casbinRule)(nil)).Where("ptype = ?", ptype)
	for i, v := range fieldValues {
		if v != "" {
			q = q.Where(fmt.Sprintf("v%d = ?", fieldIndex+i), v)
		}
	}

	_, err := q.Exec(ctx)
	return err
}

// ruleToRow converts a ptype and rule slice into a casbinRule, padding to 6 values.
func ruleToRow(ptype string, rule []string) casbinRule {
	padded := padRule(rule)
	return casbinRule{
		Ptype: ptype,
		V0:    padded[0],
		V1:    padded[1],
		V2:    padded[2],
		V3:    padded[3],
		V4:    padded[4],
		V5:    padded[5],
	}
}

// padRule pads a rule slice to exactly 6 elements with empty strings.
func padRule(rule []string) []string {
	padded := make([]string, 6)
	copy(padded, rule)
	return padded
}
