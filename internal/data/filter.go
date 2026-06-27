package data

import (
	"fmt"
	"strings"

	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	expr "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"gorm.io/gorm"
)

// applyFilter converts an AIP filter expression into GORM WHERE clauses.
// Returns the original db if the filter is empty or nil.
func applyFilter(db *gorm.DB, filter filtering.Filter) *gorm.DB {
	if filter.CheckedExpr == nil || filter.CheckedExpr.GetExpr() == nil {
		return db
	}
	clause, args := exprToSQL(filter.CheckedExpr.GetExpr())
	if clause == "" {
		return db
	}
	return db.Where(clause, args...)
}

// exprToSQL recursively converts an expression AST node to a SQL fragment.
func exprToSQL(e *expr.Expr) (string, []any) {
	switch e.ExprKind.(type) {
	case *expr.Expr_CallExpr:
		return callToSQL(e.GetCallExpr())
	case *expr.Expr_IdentExpr:
		// Standalone ident e.g. "completed" → completed = true
		name := e.GetIdentExpr().GetName()
		return fmt.Sprintf("%s = ?", name), []any{true}
	case *expr.Expr_ConstExpr:
		return "?", []any{constToValue(e.GetConstExpr())}
	default:
		return "", nil
	}
}

// callToSQL translates a function call expression to SQL.
// AIP function names: AND, OR, NOT, =, !=, >, <, >=, <=, :
func callToSQL(call *expr.Expr_Call) (string, []any) {
	fn := call.GetFunction()
	args := call.GetArgs()

	switch fn {
	case "AND":
		left, leftArgs := exprToSQL(args[0])
		right, rightArgs := exprToSQL(args[1])
		return fmt.Sprintf("(%s AND %s)", left, right), append(leftArgs, rightArgs...)
	case "OR":
		left, leftArgs := exprToSQL(args[0])
		right, rightArgs := exprToSQL(args[1])
		return fmt.Sprintf("(%s OR %s)", left, right), append(leftArgs, rightArgs...)
	case "NOT":
		inner, innerArgs := exprToSQL(args[0])
		return fmt.Sprintf("(NOT %s)", inner), innerArgs
	case "=":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s = ?", field), []any{constToValue(args[1].GetConstExpr())}
	case "!=":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s != ?", field), []any{constToValue(args[1].GetConstExpr())}
	case ">":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s > ?", field), []any{constToValue(args[1].GetConstExpr())}
	case "<":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s < ?", field), []any{constToValue(args[1].GetConstExpr())}
	case ">=":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s >= ?", field), []any{constToValue(args[1].GetConstExpr())}
	case "<=":
		field := args[0].GetIdentExpr().GetName()
		return fmt.Sprintf("%s <= ?", field), []any{constToValue(args[1].GetConstExpr())}
	case ":":
		field := args[0].GetIdentExpr().GetName()
		val := constToValue(args[1].GetConstExpr())
		return fmt.Sprintf("%s LIKE ?", field), []any{fmt.Sprintf("%%%v%%", val)}
	default:
		// Unknown function → skip (no-op filter).
		return "", nil
	}
}

// constToValue extracts a Go value from an expression constant.
func constToValue(c *expr.Constant) any {
	switch c.ConstantKind.(type) {
	case *expr.Constant_StringValue:
		return c.GetStringValue()
	case *expr.Constant_BoolValue:
		return c.GetBoolValue()
	case *expr.Constant_Int64Value:
		return c.GetInt64Value()
	case *expr.Constant_DoubleValue:
		return c.GetDoubleValue()
	default:
		return nil
	}
}

// applyOrderBy converts AIP ordering to GORM ORDER BY.
// Falls back to "id ASC" when no ordering fields are provided.
func applyOrderBy(db *gorm.DB, orderBy ordering.OrderBy) *gorm.DB {
	if len(orderBy.Fields) == 0 {
		return db.Order("id ASC")
	}
	parts := make([]string, 0, len(orderBy.Fields))
	for _, f := range orderBy.Fields {
		dir := "ASC"
		if f.Desc {
			dir = "DESC"
		}
		parts = append(parts, fmt.Sprintf("%s %s", f.Path, dir))
	}
	return db.Order(strings.Join(parts, ", "))
}
