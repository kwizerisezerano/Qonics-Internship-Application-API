package utils

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validate is the shared validator instance used to validate filter structs
// before their tags are turned into SQL.
var Validate = validator.New()

// ValidationError describes a single failed validation on a filter field.
type ValidationError struct {
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// BuildWhereClause reflects over a filter struct and builds a parameterised
// " WHERE a AND b" clause (and an optional " HAVING ...") from its `field`,
// `op`, `having` and `hash` tags. Fields without a `field` tag, unexported
// fields and empty values are skipped. Placeholders are numbered from $1, so
// the returned params can be reused verbatim by both the COUNT and the SELECT
// query.
//
// op defaults to "=" and supports =, !=, >, >=, <, <=, IN, LIKE, NOT LIKE,
// ILIKE and SEARCH. When `hash:"true"` the column is compared against the
// SHA-256 (hex) digest of the value — this is how the encrypted PII columns
// (e.g. full_name_hash) are matched without decrypting them.
func BuildWhereClause(filter any) (whereClause string, havingClause string, params []any, err error) {
	if err := Validate.Struct(filter); err != nil {
		validationErrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return "", "", nil, err
		}
		verrs := make([]ValidationError, len(validationErrors))
		for i, e := range validationErrors {
			value := "Validation error"
			if s, ok := e.Value().(string); ok {
				value = s
			}
			verrs[i] = ValidationError{Field: e.Field(), Tag: e.Tag(), Value: value}
		}
		return "", "", nil, fmt.Errorf("validation failed: %v", verrs)
	}

	var whereConditions, havingConditions []string
	paramCount := 1

	v := reflect.ValueOf(filter)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		if !field.IsExported() {
			continue
		}

		fieldName, ok := field.Tag.Lookup("field")
		if !ok || fieldName == "" {
			continue
		}

		op := field.Tag.Get("op")
		isHaving := field.Tag.Get("having") == "true"
		isHash := field.Tag.Get("hash") == "true"

		var actualValue any
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				continue
			}
			actualValue = value.Elem().Interface()
		} else {
			actualValue = value.Interface()
		}

		if actualValue == "" || actualValue == nil {
			continue
		}

		condition, conditionParams, newParamCount, condErr := buildCondition(
			fieldName, op, actualValue, paramCount, isHash, field.Name,
		)
		if condErr != nil {
			return "", "", nil, condErr
		}

		params = append(params, conditionParams...)
		paramCount = newParamCount

		if isHaving {
			havingConditions = append(havingConditions, condition)
		} else {
			whereConditions = append(whereConditions, condition)
		}
	}

	if len(whereConditions) > 0 {
		whereClause = " WHERE " + strings.Join(whereConditions, " AND ")
	}

	if len(havingConditions) > 0 {
		havingClause = " HAVING " + strings.Join(havingConditions, " AND ")
	}

	return whereClause, havingClause, params, nil
}

func buildCondition(fieldName, op string, actualValue any, paramCount int, isHash bool, structFieldName string) (string, []any, int, error) {
	switch av := actualValue.(type) {
	case string:
		return buildStringCondition(fieldName, op, av, paramCount, isHash, structFieldName)
	case int:
		return buildNumericCondition(fieldName, op, float64(av), paramCount, structFieldName)
	case float64:
		return buildNumericCondition(fieldName, op, av, paramCount, structFieldName)
	default:
		return buildDefaultCondition(fieldName, op, actualValue, paramCount, isHash)
	}
}

func buildStringCondition(fieldName, op, value string, paramCount int, isHash bool, structFieldName string) (string, []any, int, error) {
	if isHash {
		return buildHashCondition(fieldName, op, value, paramCount)
	}

	switch op {
	case "IN":
		return buildInCondition(fieldName, value, paramCount, false)
	case "ILIKE":
		return buildILikeCondition(fieldName, value, paramCount, structFieldName)
	case "NOT ILIKE":
		return buildNotILikeCondition(fieldName, value, paramCount, structFieldName)
	case "LIKE":
		return buildLikeCondition(fieldName, value, paramCount, structFieldName)
	case "NOT LIKE":
		return buildNotLikeCondition(fieldName, value, paramCount, structFieldName)
	case "SEARCH":
		return buildSearchCondition(fieldName, value, paramCount, structFieldName)
	case "CUSTOM":
		return "", nil, paramCount, fmt.Errorf("CUSTOM op is not supported; build the condition explicitly")
	default:
		return buildDefaultCondition(fieldName, op, value, paramCount, false)
	}
}

// buildHashCondition compares a hash column against the SHA-256 hex digest of
// the supplied value (matching utils.HashData / PostgreSQL encode(digest(..))).
func buildHashCondition(fieldName, op, value string, paramCount int) (string, []any, int, error) {
	if op == "" {
		op = "="
	}
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", fieldName)
	}
	condition := fmt.Sprintf("%s %s encode(digest($%d, 'sha256'), 'hex')", fieldName, op, paramCount)
	return condition, []any{value}, paramCount + 1, nil
}

func buildInCondition(fieldName, value string, paramCount int, isHash bool) (string, []any, int, error) {
	values := strings.Split(value, ",")
	if len(values) > 15 {
		return "", nil, paramCount, fmt.Errorf("too many values for IN clause in field %s", fieldName)
	}

	var placeholders []string
	var conditionParams []any

	for _, val := range values {
		val = strings.TrimSpace(val)
		if val == "" {
			return "", nil, paramCount, fmt.Errorf("empty value in IN clause for field %s", fieldName)
		}
		if isHash {
			placeholders = append(placeholders, fmt.Sprintf("encode(digest($%d, 'sha256'), 'hex')", paramCount))
		} else {
			placeholders = append(placeholders, fmt.Sprintf("$%d", paramCount))
		}
		conditionParams = append(conditionParams, val)
		paramCount++
	}

	condition := fmt.Sprintf("%s IN (%s)", fieldName, strings.Join(placeholders, ","))
	return condition, conditionParams, paramCount, nil
}

func buildILikeCondition(fieldName, value string, paramCount int, structFieldName string) (string, []any, int, error) {
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", structFieldName)
	}

	pattern := "%" + value + "%"
	if strings.HasSuffix(structFieldName, "StartsWith") {
		pattern = value + "%"
	} else if strings.HasSuffix(structFieldName, "EndsWith") {
		pattern = "%" + value
	}

	condition := fmt.Sprintf("lower(%s) ILIKE lower($%d)", fieldName, paramCount)
	return condition, []any{pattern}, paramCount + 1, nil
}

func buildNotILikeCondition(fieldName, value string, paramCount int, structFieldName string) (string, []any, int, error) {
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", structFieldName)
	}
	condition := fmt.Sprintf("lower(%s) NOT ILIKE lower($%d)", fieldName, paramCount)
	return condition, []any{"%" + value + "%"}, paramCount + 1, nil
}

func buildLikeCondition(fieldName, value string, paramCount int, structFieldName string) (string, []any, int, error) {
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", structFieldName)
	}

	pattern := "%" + strings.ToLower(value) + "%"
	if strings.HasSuffix(structFieldName, "StartsWith") {
		pattern = strings.ToLower(value) + "%"
	} else if strings.HasSuffix(structFieldName, "EndsWith") {
		pattern = "%" + strings.ToLower(value)
	}

	condition := fmt.Sprintf("lower(%s) LIKE $%d", fieldName, paramCount)
	return condition, []any{pattern}, paramCount + 1, nil
}

func buildNotLikeCondition(fieldName, value string, paramCount int, structFieldName string) (string, []any, int, error) {
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", structFieldName)
	}
	condition := fmt.Sprintf("lower(%s) NOT LIKE $%d", fieldName, paramCount)
	return condition, []any{"%" + strings.ToLower(value) + "%"}, paramCount + 1, nil
}

func buildSearchCondition(fieldName, value string, paramCount int, structFieldName string) (string, []any, int, error) {
	if len(value) > 100 {
		return "", nil, paramCount, fmt.Errorf("search term too long for field %s", structFieldName)
	}
	condition := fmt.Sprintf("to_tsvector('english', %s) @@ plainto_tsquery('english', $%d)", fieldName, paramCount)
	return condition, []any{value}, paramCount + 1, nil
}

func buildDefaultCondition(fieldName, op string, value any, paramCount int, isHash bool) (string, []any, int, error) {
	if op == "" {
		op = "="
	}
	if isHash {
		if s, ok := value.(string); ok {
			return buildHashCondition(fieldName, op, s, paramCount)
		}
	}
	condition := fmt.Sprintf("%s %s $%d", fieldName, op, paramCount)
	return condition, []any{value}, paramCount + 1, nil
}

func buildNumericCondition(fieldName, op string, value float64, paramCount int, structFieldName string) (string, []any, int, error) {
	if op == "" {
		op = "="
	}
	if (op == ">" || op == "<") && value < 0 {
		return "", nil, paramCount, fmt.Errorf("value out of range for field %s", structFieldName)
	}
	condition := fmt.Sprintf("%s %s $%d", fieldName, op, paramCount)
	return condition, []any{value}, paramCount + 1, nil
}

// AppendCondition joins a raw condition onto a clause returned by
// BuildWhereClause (or an empty string), preserving the " WHERE " prefix.
func AppendCondition(whereClause string, condition string) string {
	if condition == "" {
		return whereClause
	}
	if whereClause == "" {
		return " WHERE " + condition
	}
	return whereClause + " AND " + condition
}

// AppendSearchClause appends a cross-column search OR-group to a clause built by
// BuildWhereClause, keeping the parameter numbering contiguous. `likeColumns`
// are matched with a case-insensitive contains; `hashColumns` are matched
// against the SHA-256 (hex) digest of the term — the mechanism used for the
// encrypted PII columns (e.g. full_name_hash / email_hash), following the leazi
// pattern. An empty term is a no-op.
func AppendSearchClause(whereClause string, params []any, likeColumns []string, hashColumns []string, term string) (string, []any) {
	if term == "" {
		return whereClause, params
	}

	next := len(params) + 1
	var parts []string
	var extra []any

	if len(likeColumns) > 0 {
		for _, col := range likeColumns {
			parts = append(parts, fmt.Sprintf("lower(%s) LIKE lower($%d)", col, next))
		}
		extra = append(extra, "%"+term+"%")
		next++
	}

	if len(hashColumns) > 0 {
		for _, col := range hashColumns {
			parts = append(parts, fmt.Sprintf("%s = encode(digest($%d, 'sha256'), 'hex')", col, next))
		}
		extra = append(extra, term)
		next++
	}

	if len(parts) == 0 {
		return whereClause, params
	}

	whereClause = AppendCondition(whereClause, "("+strings.Join(parts, " OR ")+")")
	params = append(params, extra...)
	return whereClause, params
}

// BuildQuerySort renders an " order by ..." clause from an ordered map of
// column -> direction (ASC/DESC). Empty directions are skipped.
func BuildQuerySort(sort map[string]any) string {
	query := ""
	for key, value := range sort {
		if value != "" {
			if len(query) != 0 {
				query += ", "
			}
			query += fmt.Sprintf("%s %s", key, value)
		}
	}
	if len(query) != 0 {
		query = " order by " + query
	}
	return query
}

// GeneratePaginationData builds the standard leazi pagination meta block.
func GeneratePaginationData(total int, limit int, page int) map[string]any {
	pages := int(math.Ceil(float64(total) / float64(limit)))
	pagination := map[string]any{
		"total":    total,
		"per_page": limit,
		"current":  page,
		"pages":    pages,
	}
	if page < pages {
		pagination["next"] = page + 1
	} else {
		pagination["next"] = nil
	}
	if page > 1 {
		pagination["prev"] = page - 1
	} else {
		pagination["prev"] = nil
	}
	return pagination
}
