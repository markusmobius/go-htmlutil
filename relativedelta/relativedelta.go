package relativedelta

import (
	"errors"
	"math"
	"math/big"
	"time"
)

var ErrRange = errors.New("relative date is outside Python datetime range")
var ErrFractionalCalendar = errors.New("non-integer years and months are not supported")

type Delta struct {
	Years        float64 `json:"years"`
	Months       float64 `json:"months"`
	Weeks        float64 `json:"weeks"`
	Days         float64 `json:"days"`
	Hours        float64 `json:"hours"`
	Minutes      float64 `json:"minutes"`
	Seconds      float64 `json:"seconds"`
	Microseconds float64 `json:"microseconds"`
}

func (delta Delta) Apply(base time.Time) (time.Time, error) {
	values := []float64{delta.Years, delta.Months, delta.Weeks, delta.Days, delta.Hours, delta.Minutes, delta.Seconds, delta.Microseconds}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return time.Time{}, ErrRange
		}
	}
	if math.Trunc(delta.Years) != delta.Years || math.Trunc(delta.Months) != delta.Months {
		return time.Time{}, ErrFractionalCalendar
	}
	years, _ := new(big.Float).SetFloat64(delta.Years).Int(nil)
	months, _ := new(big.Float).SetFloat64(delta.Months).Int(nil)
	carry, remainder := new(big.Int), new(big.Int)
	carry.QuoRem(months, big.NewInt(12), remainder)
	years.Add(years, carry)
	days := delta.Days + delta.Weeks*7
	hours, minutes, seconds, microseconds := delta.Hours, delta.Minutes, delta.Seconds, delta.Microseconds
	for _, step := range []struct {
		value, next  *float64
		limit, scale float64
	}{
		{&microseconds, &seconds, 999999, 1000000},
		{&seconds, &minutes, 59, 60},
		{&minutes, &hours, 59, 60},
		{&hours, &days, 23, 24},
	} {
		if math.Abs(*step.value) > step.limit {
			sign := math.Copysign(1, *step.value)
			quotient := math.Floor(math.Abs(*step.value) / step.scale)
			remainder := math.Mod(math.Abs(*step.value), step.scale)
			*step.value = remainder * sign
			*step.next += quotient * sign
		}
	}
	years.Add(years, big.NewInt(int64(base.Year())))
	month := int64(base.Month()) + remainder.Int64()
	if month > 12 {
		years.Add(years, big.NewInt(1))
		month -= 12
	} else if month < 1 {
		years.Sub(years, big.NewInt(1))
		month += 12
	}
	if !years.IsInt64() {
		return time.Time{}, ErrRange
	}
	year := years.Int64()
	if year < 1 || year > 9999 || math.IsInf(days, 0) || math.Abs(days) > 999999999 {
		return time.Time{}, ErrRange
	}
	lastDay := time.Date(int(year), time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := min(base.Day(), lastDay)
	wholeDays, fractionalDays := math.Modf(days)
	wholeSeconds, fractionalSeconds := math.Modf(seconds + minutes*60 + hours*3600)
	wholeMicros, fractionalMicros := math.Modf(microseconds)
	daySeconds := fractionalDays * 86400
	dayWholeSeconds, dayFractionalSeconds := math.Modf(daySeconds)
	totalSeconds := wholeSeconds + dayWholeSeconds
	leftoverMicros := (fractionalSeconds+dayFractionalSeconds)*1000000 + fractionalMicros
	totalMicros := int64(math.RoundToEven(wholeMicros + leftoverMicros))
	wall := time.Date(int(year), time.Month(month), day, base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), time.UTC)
	wall = wall.AddDate(0, 0, int(wholeDays))
	wall = time.Unix(wall.Unix()+int64(totalSeconds)+totalMicros/1000000, int64(wall.Nanosecond())+totalMicros%1000000*1000).UTC()
	if wall.Year() < 1 || wall.Year() > 9999 {
		return time.Time{}, ErrRange
	}
	return localWall(wall, base.Location()), nil
}

func localWall(wall time.Time, location *time.Location) time.Time {
	guess := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), location)
	beforeName, beforeOffset := guess.AddDate(0, 0, -1).Zone()
	_, afterOffset := guess.AddDate(0, 0, 1).Zone()
	var earliest time.Time
	for _, offset := range []int{beforeOffset, afterOffset} {
		candidate := wall.Add(-time.Duration(offset) * time.Second).In(location)
		if candidate.Year() == wall.Year() && candidate.Month() == wall.Month() && candidate.Day() == wall.Day() && candidate.Hour() == wall.Hour() && candidate.Minute() == wall.Minute() && candidate.Second() == wall.Second() {
			if earliest.IsZero() || candidate.Before(earliest) {
				earliest = candidate
			}
		}
	}
	if !earliest.IsZero() {
		return earliest
	}
	return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.FixedZone(beforeName, beforeOffset))
}
