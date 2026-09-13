package period

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var dayPeriodPattern = regexp.MustCompile(`^[1-9][0-9]*d$`)

func ParseSince(value string, now time.Time) (time.Time, error) {
	if dayPeriodPattern.MatchString(value) {
		days, err := strconv.Atoi(value[:len(value)-1])

		if err != nil {
			return time.Time{}, fmt.Errorf("parse day period: %w", err)
		}

		return now.AddDate(0, 0, -days), nil
	}

	cutoff, err := time.Parse(time.DateOnly, value)

	if err != nil {
		return time.Time{}, fmt.Errorf("review period must be Nd or YYYY-MM-DD: %w", err)
	}

	if cutoff.After(now) {
		return time.Time{}, fmt.Errorf("review period cannot be in the future")
	}

	return cutoff, nil
}
