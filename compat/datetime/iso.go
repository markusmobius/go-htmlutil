package datetime

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/markusmobius/go-dateutil/v2/parser"
)

var ErrISOFormat = errors.New("invalid CPython ISO datetime")

func FromISOFormat(value string) (parser.Result, error) {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 7 {
		return parser.Result{}, ErrISOFormat
	}
	separator := dateSeparator(value)
	if separator < 0 || separator > len(value) {
		return parser.Result{}, ErrISOFormat
	}
	date, ok := dateParts(value[:separator])
	if !ok {
		return parser.Result{}, ErrISOFormat
	}
	var clock [4]int
	var offset time.Duration
	aware := false
	if separator < len(value) {
		_, width := utf8.DecodeRuneInString(value[separator:])
		clock, offset, aware, ok = timeParts(value[separator+width:])
		if !ok {
			return parser.Result{}, ErrISOFormat
		}
	}
	year, month, day := date[0], date[1], date[2]
	if clock[0] == 24 && month <= 12 && day <= monthDays(year, month) {
		if clock[1] != 0 || clock[2] != 0 || clock[3] != 0 {
			return parser.Result{}, ErrISOFormat
		}
		clock[0] = 0
		day++
		if day > monthDays(year, month) {
			day = 1
			month++
			if month > 12 {
				month = 1
				year++
			}
		}
	}
	if year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 || day > monthDays(year, month) ||
		clock[0] > 23 || clock[1] > 59 || clock[2] > 59 {
		return parser.Result{}, ErrISOFormat
	}
	return parser.Result{
		Time:   time.Date(year, time.Month(month), day, clock[0], clock[1], clock[2], clock[3]*1000, time.UTC),
		Aware:  aware,
		Offset: offset,
	}, nil
}

func dateSeparator(value string) int {
	length := len(value)
	if length == 7 {
		return 7
	}
	if value[4] == '-' {
		if value[5] != 'W' {
			return 10
		}
		if length > 8 && value[8] == '-' {
			if length == 9 {
				return -1
			}
			if length > 10 && digit(value[10]) {
				return 8
			}
			return 10
		}
		return 8
	}
	if value[4] != 'W' {
		return 8
	}
	position := 7
	for position < length && digit(value[position]) {
		position++
	}
	if position < 9 {
		return position
	}
	if position%2 == 0 {
		return 7
	}
	return 8
}

func dateParts(value string) ([3]int, bool) {
	var result [3]int
	year, ok := number(value, 0, 4)
	if !ok || len(value) < 5 {
		return result, false
	}
	position := 4
	separated := value[position] == '-'
	if separated {
		position++
	}
	if character(value, position) == 'W' {
		week, valid := number(value, position+1, 2)
		if !valid || year < 1 || week < 1 {
			return result, false
		}
		_, lastWeek := time.Date(year, 12, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
		if week > lastWeek {
			return result, false
		}
		position += 3
		day := 1
		if position < len(value) {
			if separated {
				if value[position] != '-' {
					return result, false
				}
				position++
			}
			day, valid = number(value, position, 1)
			if !valid || day < 1 || day > 7 {
				return result, false
			}
		}
		fourth := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
		mondayOffset := (int(fourth.Weekday()) + 6) % 7
		date := fourth.AddDate(0, 0, (week-1)*7+day-1-mondayOffset)
		return [3]int{date.Year(), int(date.Month()), date.Day()}, true
	}
	month, ok := number(value, position, 2)
	if !ok {
		return result, false
	}
	position += 2
	if separated {
		if character(value, position) != '-' {
			return result, false
		}
		position++
	}
	day, ok := number(value, position, 2)
	return [3]int{year, month, day}, ok
}

func timeParts(value string) ([4]int, time.Duration, bool, bool) {
	zonePosition := len(value)
	for position := 0; position < len(value); position++ {
		if value[position] == '+' || value[position] == '-' || value[position] == 'Z' {
			zonePosition = position
			break
		}
	}
	clock, status := clockParts(value, zonePosition)
	if status < 0 {
		return clock, 0, false, false
	}
	if zonePosition == len(value) {
		return clock, 0, false, status == 0
	}
	if value[zonePosition] == 'Z' {
		return clock, 0, true, character(value, zonePosition+1) == 0
	}
	zoneText := value[zonePosition+1:]
	zone, status := clockParts(zoneText, len(zoneText))
	if status != 0 {
		return clock, 0, false, false
	}
	seconds := zone[0]*3600 + zone[1]*60 + zone[2]
	if seconds == 0 {
		return clock, 0, true, true
	}
	offset := time.Duration(seconds)*time.Second + time.Duration(zone[3])*time.Microsecond
	if offset >= 24*time.Hour {
		return clock, 0, false, false
	}
	if value[zonePosition] == '-' {
		offset = -offset
	}
	return clock, offset, true, true
}

func clockParts(value string, end int) ([4]int, int) {
	var result [4]int
	position := 0
	separated := false
	for component := 0; component < 3; component++ {
		part, ok := number(value, position, 2)
		if !ok {
			return result, -1
		}
		result[component] = part
		position += 2
		next := character(value, position)
		position++
		if component == 0 {
			separated = next == ':'
		}
		if position >= end {
			return result, trailing(next)
		}
		if separated && next == ':' {
			if component == 2 {
				return result, -1
			}
			continue
		}
		if next == '.' || next == ',' {
			if component < 2 {
				return result, -1
			}
			break
		}
		if separated {
			return result, -1
		}
		position--
	}
	fractionLength := min(end-position, 6)
	fraction, ok := number(value, position, fractionLength)
	if !ok || fractionLength < 1 {
		return result, -1
	}
	position += fractionLength
	for fractionLength < 6 {
		fraction *= 10
		fractionLength++
	}
	result[3] = fraction
	for digit(character(value, position)) {
		position++
	}
	return result, trailing(character(value, position))
}

func number(value string, start, count int) (int, bool) {
	if count < 1 || start+count > len(value) {
		return 0, false
	}
	result := 0
	for _, character := range []byte(value[start : start+count]) {
		if !digit(character) {
			return 0, false
		}
		result = result*10 + int(character-'0')
	}
	return result, true
}

func character(value string, position int) byte {
	if position >= len(value) {
		return 0
	}
	return value[position]
}

func digit(value byte) bool { return value >= '0' && value <= '9' }

func trailing(value byte) int {
	if value == 0 {
		return 0
	}
	return 1
}

func monthDays(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	if month == 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		return 29
	}
	return [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[month]
}
