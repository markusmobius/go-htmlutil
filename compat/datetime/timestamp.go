package datetime

import (
	"errors"
	"math/big"
	"time"

	"github.com/markusmobius/go-dateutil/v2/parser"
)

var ErrTimestamp = errors.New("datetime is outside the CPython timestamp range")

func Timestamp(value parser.Result, local *time.Location, fold bool) (float64, error) {
	if value.Time.Year() < 1 || value.Time.Year() > 9999 {
		return 0, ErrTimestamp
	}
	wall := time.Date(value.Time.Year(), value.Time.Month(), value.Time.Day(), value.Time.Hour(), value.Time.Minute(), value.Time.Second(), value.Time.Nanosecond(), time.UTC)
	if value.Aware {
		if value.Offset <= -24*time.Hour || value.Offset >= 24*time.Hour {
			return 0, ErrTimestamp
		}
		microseconds := wall.Unix()*1000000 + int64(wall.Nanosecond()/1000) - int64(value.Offset/time.Microsecond)
		result, _ := new(big.Rat).SetFrac64(microseconds, 1000000).Float64()
		return result, nil
	}
	if local == nil {
		local = time.Local
	}
	seconds, err := localSeconds(wall.Unix(), local, fold)
	if err != nil {
		return 0, err
	}
	return float64(seconds) + float64(wall.Nanosecond()/1000)/1000000, nil
}

func localSeconds(target int64, location *time.Location, fold bool) (int64, error) {
	local := func(seconds int64) (int64, error) {
		value := time.Unix(seconds, 0).In(location)
		if value.Year() < 1 || value.Year() > 9999 {
			return 0, ErrTimestamp
		}
		return time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), 0, time.UTC).Unix(), nil
	}
	localTarget, err := local(target)
	if err != nil {
		return 0, err
	}
	firstOffset := localTarget - target
	first := target - firstOffset
	firstWall, err := local(first)
	if err != nil {
		return 0, err
	}
	secondOffset := firstWall - first
	if firstWall == target {
		probe := first - 86400
		if fold {
			probe = first + 86400
		}
		probeWall, err := local(probe)
		if err != nil {
			return 0, err
		}
		secondOffset = probeWall - probe
		if firstOffset == secondOffset {
			return first, nil
		}
	}
	second := target - secondOffset
	secondWall, err := local(second)
	if err != nil {
		return 0, err
	}
	if secondWall == target {
		return second, nil
	}
	if firstWall == target {
		return first, nil
	}
	if fold {
		return min(first, second), nil
	}
	return max(first, second), nil
}
