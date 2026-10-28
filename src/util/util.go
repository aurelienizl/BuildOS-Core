package util

import (
	"fmt"
	"regexp"
	"strconv"
)

var (
	modeRe = regexp.MustCompile(`^[0-7]{3,4}$`)
)

func NormalizeMode(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "644", nil
	case string:
		if !modeRe.MatchString(t) {
			return "", fmt.Errorf("invalid mode: %v", t)
		}
		return t, nil
	case int:
		s := strconv.Itoa(t)
		if !modeRe.MatchString(s) {
			return "", fmt.Errorf("invalid mode: %v", t)
		}
		return s, nil
	default:
		return "", fmt.Errorf("invalid mode type: %T", v)
	}
}
