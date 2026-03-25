package mapper

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/petervdpas/goop2/internal/orm/schema"
)

type transformFn func(values []any, args []any) (any, error)

var transforms = map[string]transformFn{
	"uppercase":  txUppercase,
	"lowercase":  txLowercase,
	"trim":       txTrim,
	"concat":     txConcat,
	"round":      txRound,
	"default":    txDefault,
	"to_int":     txToInt,
	"to_text":    txToText,
	"to_float":   txToFloat,
	"substring":  txSubstring,
	"prefix":     txPrefix,
	"suffix":     txSuffix,
	"now":        txNow,
	"guid":       txGuid,
	"datetime":   txDatetime,
	"date":       txDate,
	"time":       txTime,
	"coalesce":   txCoalesce,
	"replace":    txReplace,
	"split":      txSplit,
	"join":       txJoin,
	"length":     txLength,
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == math.Trunc(val) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	}
	return 0, false
}

func firstString(values []any) string {
	if len(values) == 0 {
		return ""
	}
	return toString(values[0])
}

func argString(args []any, index int, fallback string) string {
	if index < len(args) {
		return toString(args[index])
	}
	return fallback
}

func argInt(args []any, index int, fallback int) int {
	if index < len(args) {
		if f, ok := toFloat(args[index]); ok {
			return int(f)
		}
	}
	return fallback
}

func txUppercase(values []any, _ []any) (any, error) {
	return strings.ToUpper(firstString(values)), nil
}

func txLowercase(values []any, _ []any) (any, error) {
	return strings.ToLower(firstString(values)), nil
}

func txTrim(values []any, _ []any) (any, error) {
	return strings.TrimSpace(firstString(values)), nil
}

func txConcat(values []any, args []any) (any, error) {
	sep := argString(args, 0, "")
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = toString(v)
	}
	return strings.Join(parts, sep), nil
}

func txRound(values []any, args []any) (any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	f, ok := toFloat(values[0])
	if !ok {
		return values[0], nil
	}
	decimals := argInt(args, 0, 0)
	pow := math.Pow(10, float64(decimals))
	return math.Round(f*pow) / pow, nil
}

func txDefault(values []any, args []any) (any, error) {
	if len(values) > 0 && values[0] != nil {
		s := toString(values[0])
		if s != "" {
			return values[0], nil
		}
	}
	if len(args) > 0 {
		return args[0], nil
	}
	return nil, nil
}

func txToInt(values []any, _ []any) (any, error) {
	if len(values) == 0 || values[0] == nil {
		return nil, nil
	}
	f, ok := toFloat(values[0])
	if !ok {
		return nil, fmt.Errorf("cannot convert %v to integer", values[0])
	}
	return int64(f), nil
}

func txToText(values []any, _ []any) (any, error) {
	return firstString(values), nil
}

func txToFloat(values []any, _ []any) (any, error) {
	if len(values) == 0 || values[0] == nil {
		return nil, nil
	}
	f, ok := toFloat(values[0])
	if !ok {
		return nil, fmt.Errorf("cannot convert %v to float", values[0])
	}
	return f, nil
}

func txSubstring(values []any, args []any) (any, error) {
	s := firstString(values)
	start := argInt(args, 0, 0)
	length := argInt(args, 1, len(s)-start)
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return "", nil
	}
	end := start + length
	if end > len(s) {
		end = len(s)
	}
	return s[start:end], nil
}

func txPrefix(values []any, args []any) (any, error) {
	prefix := argString(args, 0, "")
	return prefix + firstString(values), nil
}

func txSuffix(values []any, args []any) (any, error) {
	suffix := argString(args, 0, "")
	return firstString(values) + suffix, nil
}

func txNow(_ []any, _ []any) (any, error) {
	return schema.NowUTC(), nil
}

func txCoalesce(values []any, _ []any) (any, error) {
	for _, v := range values {
		if v != nil {
			s := toString(v)
			if s != "" {
				return v, nil
			}
		}
	}
	return nil, nil
}

func txReplace(values []any, args []any) (any, error) {
	s := firstString(values)
	old := argString(args, 0, "")
	new := argString(args, 1, "")
	return strings.ReplaceAll(s, old, new), nil
}

func txSplit(values []any, args []any) (any, error) {
	s := firstString(values)
	sep := argString(args, 0, ",")
	idx := argInt(args, 1, -1)
	parts := strings.Split(s, sep)
	if idx >= 0 && idx < len(parts) {
		return strings.TrimSpace(parts[idx]), nil
	}
	result := make([]any, len(parts))
	for i, p := range parts {
		result[i] = strings.TrimSpace(p)
	}
	return result, nil
}

func txJoin(values []any, args []any) (any, error) {
	sep := argString(args, 0, ",")
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = toString(v)
	}
	return strings.Join(parts, sep), nil
}

func txLength(values []any, _ []any) (any, error) {
	return int64(len(firstString(values))), nil
}

func txGuid(_ []any, _ []any) (any, error) {
	return schema.GenerateGUID(), nil
}

func txDatetime(_ []any, _ []any) (any, error) {
	return schema.NowUTC(), nil
}

func txDate(_ []any, _ []any) (any, error) {
	return schema.NowDate(), nil
}

func txTime(_ []any, _ []any) (any, error) {
	return schema.NowTime(), nil
}
