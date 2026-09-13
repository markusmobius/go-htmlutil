package parser

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrParse = errors.New("invalid Python date expression")

type Result struct {
	Time   time.Time
	Aware  bool
	Offset time.Duration
}

func (result Result) Instant() time.Time {
	if !result.Aware {
		return result.Time
	}
	wall := time.Date(result.Time.Year(), result.Time.Month(), result.Time.Day(), result.Time.Hour(), result.Time.Minute(), result.Time.Second(), result.Time.Nanosecond(), time.UTC)
	return wall.Add(-result.Offset)
}

var jumps = map[string]bool{" ": true, ".": true, ",": true, ";": true, "-": true, "/": true, "'": true, "at": true, "on": true, "and": true, "ad": true, "m": true, "t": true, "of": true, "st": true, "nd": true, "rd": true, "th": true}
var months = wordMap([][]string{{"jan", "january"}, {"feb", "february"}, {"mar", "march"}, {"apr", "april"}, {"may"}, {"jun", "june"}, {"jul", "july"}, {"aug", "august"}, {"sep", "sept", "september"}, {"oct", "october"}, {"nov", "november"}, {"dec", "december"}})
var weekdays = wordMap([][]string{{"mon", "monday"}, {"tue", "tuesday"}, {"wed", "wednesday"}, {"thu", "thursday"}, {"fri", "friday"}, {"sat", "saturday"}, {"sun", "sunday"}})
var timeUnits = wordMap([][]string{{"h", "hour", "hours"}, {"m", "minute", "minutes"}, {"s", "second", "seconds"}})
var meridiems = wordMap([][]string{{"am", "a"}, {"pm", "p"}})

func wordMap(groups [][]string) map[string]int {
	result := make(map[string]int)
	for index, group := range groups {
		for _, word := range group {
			result[word] = index
		}
	}
	return result
}

func lookup(words map[string]int, value string) int {
	if index, ok := words[strings.ToLower(value)]; ok {
		return index
	}
	return -1
}

func jump(value string) bool { return jumps[strings.ToLower(value)] }

func utcZone(value string) bool {
	value = strings.ToLower(value)
	return value == "utc" || value == "gmt" || value == "z"
}

func intToken(value string) (int, error) {
	converted, ok := asciiDecimal(value)
	if !ok {
		return 0, ErrParse
	}
	result, err := strconv.Atoi(converted)
	if err != nil {
		return 0, ErrParse
	}
	return result, nil
}

func slice(value string, start, end int) string {
	characters := []rune(value)
	if start > len(characters) {
		start = len(characters)
	}
	if end > len(characters) {
		end = len(characters)
	}
	return string(characters[start:end])
}

func numeric(value string) bool {
	converted, ok := asciiDecimal(value)
	if !ok {
		return false
	}
	_, err := strconv.ParseFloat(converted, 64)
	if err == nil {
		return true
	}
	var numberError *strconv.NumError
	return errors.As(err, &numberError) && errors.Is(numberError.Err, strconv.ErrRange)
}

type decimal struct {
	coefficient *big.Int
	scale       int
}

func parseDecimal(value string) (decimal, error) {
	converted, ok := asciiDecimal(value)
	if !ok {
		return decimal{}, ErrParse
	}
	integer, fraction, found := strings.Cut(converted, ".")
	if strings.Contains(fraction, ".") {
		return decimal{}, ErrParse
	}
	if integer == "" {
		integer = "0"
	}
	coefficient, ok := new(big.Int).SetString(integer+fraction, 10)
	if !ok {
		return decimal{}, ErrParse
	}
	length := 0
	if found {
		length = len(fraction)
	}
	return decimal{coefficient, length}, nil
}

func powerTen(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}

func (value decimal) integer() (int, error) {
	integer := new(big.Int).Quo(value.coefficient, powerTen(value.scale))
	if !integer.IsInt64() {
		return 0, ErrParse
	}
	result := integer.Int64()
	if int64(int(result)) != result {
		return 0, ErrParse
	}
	return int(result), nil
}

func (value decimal) compare(other int) int {
	return value.coefficient.Cmp(new(big.Int).Mul(big.NewInt(int64(other)), powerTen(value.scale)))
}

func (value decimal) fractionSixty() (int, bool, error) {
	remainder := new(big.Int).Rem(value.coefficient, powerTen(value.scale))
	if remainder.Sign() == 0 {
		return 0, false, nil
	}
	product := new(big.Int).Mul(remainder, big.NewInt(60))
	scale := value.scale
	digits := len(new(big.Int).Abs(product).String())
	if digits > 28 {
		divisor := powerTen(digits - 28)
		quotient, residue := new(big.Int), new(big.Int)
		quotient.QuoRem(product, divisor, residue)
		comparison := new(big.Int).Lsh(new(big.Int).Abs(residue), 1).Cmp(divisor)
		if comparison > 0 || comparison == 0 && quotient.Bit(0) != 0 {
			quotient.Add(quotient, big.NewInt(int64(product.Sign())))
		}
		product = quotient
		scale -= digits - 28
	}
	if scale >= 0 {
		product.Quo(product, powerTen(scale))
	} else {
		product.Mul(product, powerTen(-scale))
	}
	if !product.IsInt64() {
		return 0, false, ErrParse
	}
	return int(product.Int64()), true, nil
}

type yearMonthDay struct {
	values  []int
	labels  [3]int
	century bool
}

func newYMD() yearMonthDay { return yearMonthDay{labels: [3]int{-1, -1, -1}} }

func (date *yearMonthDay) append(value int, label int, century bool) error {
	if century {
		date.century = true
		if label != -1 && label != 0 {
			return ErrParse
		}
		label = 0
	}
	date.values = append(date.values, value)
	if label >= 0 {
		if date.labels[label] >= 0 {
			return ErrParse
		}
		date.labels[label] = len(date.values) - 1
	}
	return nil
}

func (date *yearMonthDay) appendString(value string, label int) error {
	integer, err := intToken(value)
	if err != nil {
		return err
	}
	return date.append(integer, label, IsDigits(value) && utf8.RuneCountInString(value) > 2)
}

func (date *yearMonthDay) appendNumber(value decimal) error {
	integer, err := value.integer()
	if err != nil {
		return err
	}
	return date.append(integer, -1, value.compare(100) > 0)
}

func daysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (date *yearMonthDay) couldBeDay(value decimal) bool {
	if date.labels[2] >= 0 {
		return false
	}
	lastDay := 31
	if date.labels[1] >= 0 {
		year := 2000
		if date.labels[0] >= 0 {
			year = date.values[date.labels[0]]
		}
		lastDay = daysInMonth(year, date.values[date.labels[1]])
	}
	return value.compare(1) >= 0 && value.compare(lastDay) <= 0
}

func (date *yearMonthDay) resolve() (int, int, int, error) {
	year, month, day := -1, -1, -1
	count := len(date.values)
	labels := date.labels
	labeled := 0
	for _, position := range labels {
		if position >= 0 {
			labeled++
		}
	}
	if count == labeled && count > 0 || count == 3 && labeled == 2 {
		if count == 3 && labeled == 2 {
			missingLabel, missingPosition := -1, -1
			for index, position := range labels {
				if position < 0 {
					missingLabel = index
				}
			}
			for position := 0; position < 3; position++ {
				if position != labels[0] && position != labels[1] && position != labels[2] {
					missingPosition = position
				}
			}
			labels[missingLabel] = missingPosition
		}
		values := [3]int{-1, -1, -1}
		for index, position := range labels {
			if position >= 0 {
				values[index] = date.values[position]
			}
		}
		return values[0], values[1], values[2], nil
	}
	monthPosition := labels[1]
	switch {
	case count > 3:
		return -1, -1, -1, ErrParse
	case count == 1 || monthPosition >= 0 && count == 2:
		other := date.values[0]
		if monthPosition >= 0 {
			month = date.values[monthPosition]
			other = date.values[(monthPosition+count-1)%count]
		}
		if count > 1 || monthPosition < 0 {
			if other > 31 {
				year = other
			} else {
				day = other
			}
		}
	case count == 2:
		first, second := date.values[0], date.values[1]
		if first > 31 {
			year, month = first, second
		} else if second > 31 {
			month, year = first, second
		} else {
			month, day = first, second
		}
	case count == 3:
		first, second, third := date.values[0], date.values[1], date.values[2]
		switch monthPosition {
		case 0:
			if second > 31 {
				month, year, day = first, second, third
			} else {
				month, day, year = first, second, third
			}
		case 1:
			if first > 31 {
				year, month, day = first, second, third
			} else {
				day, month, year = first, second, third
			}
		case 2:
			if second > 31 {
				day, year, month = first, second, third
			} else {
				year, day, month = first, second, third
			}
		default:
			if first > 31 || labels[0] == 0 {
				year, month, day = first, second, third
			} else if first > 12 {
				day, month, year = first, second, third
			} else {
				month, day, year = first, second, third
			}
		}
	}
	return year, month, day, nil
}

type parserResult struct {
	year, month, day, weekday         int
	hour, minute, second, microsecond int
	ampm                              int
	zoneName                          string
	zoneOffset                        int
	hasOffset                         bool
}

func newParserResult() parserResult {
	return parserResult{year: -1, month: -1, day: -1, weekday: -1, hour: -1, minute: -1, second: -1, microsecond: -1, ampm: -1}
}

func convertYear(year, currentYear int, century bool) int {
	if year < 100 && !century {
		year += currentYear / 100 * 100
		if year >= currentYear+50 {
			year -= 100
		} else if year < currentYear-50 {
			year += 100
		}
	}
	return year
}

func adjustAMPM(hour, ampm int) int {
	if hour < 12 && ampm == 1 {
		return hour + 12
	}
	if hour == 12 && ampm == 0 {
		return 0
	}
	return hour
}

func parseSeconds(value string) (int, int, error) {
	integer, fraction, found := strings.Cut(value, ".")
	seconds, err := intToken(integer)
	if err != nil {
		return 0, 0, err
	}
	if !found {
		return seconds, 0, nil
	}
	fraction = slice(fraction+"000000", 0, 6)
	microseconds, err := intToken(fraction)
	return seconds, microseconds, err
}

func findUnit(index int, tokens []string) int {
	if index+1 < len(tokens) && lookup(timeUnits, tokens[index+1]) >= 0 {
		return index + 1
	}
	if index+2 < len(tokens) && tokens[index+1] == " " && lookup(timeUnits, tokens[index+2]) >= 0 {
		return index + 2
	}
	if index > 0 && lookup(timeUnits, tokens[index-1]) >= 0 {
		return index - 1
	}
	if index > 1 && index == len(tokens)-1 && tokens[index-1] == " " && lookup(timeUnits, tokens[index-2]) >= 0 {
		return index - 2
	}
	return -1
}

func assignUnit(result *parserResult, value decimal, representation string, unit int) error {
	integer, err := value.integer()
	if err != nil {
		return err
	}
	switch unit {
	case 0:
		result.hour = integer
		if minute, found, err := value.fractionSixty(); err != nil {
			return err
		} else if found {
			result.minute = minute
		}
	case 1:
		result.minute, result.second = integer, -1
		if second, found, err := value.fractionSixty(); err != nil {
			return err
		} else if found {
			result.second = second
		}
	case 2:
		result.second, result.microsecond, err = parseSeconds(representation)
	}
	return err
}

func parseNumeric(tokens []string, index int, date *yearMonthDay, result *parserResult) (int, error) {
	representation := tokens[index]
	value, err := parseDecimal(representation)
	if err != nil {
		return index, err
	}
	length := utf8.RuneCountInString(representation)
	unitIndex := findUnit(index, tokens)
	switch {
	case len(date.values) == 3 && (length == 2 || length == 4) && result.hour < 0 && (index+1 >= len(tokens) || tokens[index+1] != ":" && lookup(timeUnits, tokens[index+1]) < 0):
		result.hour, err = intToken(slice(representation, 0, 2))
		if err == nil && length == 4 {
			result.minute, err = intToken(slice(representation, 2, 4))
		}
	case length == 6 || length > 6 && strings.IndexRune(representation, '.') == len(slice(representation, 0, 6)):
		if len(date.values) == 0 && !strings.Contains(representation, ".") {
			for _, part := range []string{slice(representation, 0, 2), slice(representation, 2, 4), slice(representation, 4, length)} {
				if err = date.appendString(part, -1); err != nil {
					return index, err
				}
			}
		} else {
			result.hour, err = intToken(slice(representation, 0, 2))
			if err != nil {
				return index, err
			}
			result.minute, err = intToken(slice(representation, 2, 4))
			if err != nil {
				return index, err
			}
			result.second, result.microsecond, err = parseSeconds(slice(representation, 4, length))
		}
	case length == 8 || length == 12 || length == 14:
		if err = date.appendString(slice(representation, 0, 4), 0); err != nil {
			return index, err
		}
		if err = date.appendString(slice(representation, 4, 6), -1); err != nil {
			return index, err
		}
		if err = date.appendString(slice(representation, 6, 8), -1); err != nil {
			return index, err
		}
		if length > 8 {
			result.hour, err = intToken(slice(representation, 8, 10))
			if err != nil {
				return index, err
			}
			result.minute, err = intToken(slice(representation, 10, 12))
			if err == nil && length > 12 {
				result.second, err = intToken(slice(representation, 12, length))
			}
		}
	case unitIndex >= 0:
		unit := lookup(timeUnits, tokens[unitIndex])
		if unitIndex > index {
			index = unitIndex
		} else {
			unit++
		}
		err = assignUnit(result, value, representation, unit)
	case index+2 < len(tokens) && tokens[index+1] == ":":
		result.hour, err = value.integer()
		if err != nil {
			return index, err
		}
		minutes, err := parseDecimal(tokens[index+2])
		if err != nil {
			return index, err
		}
		if err = assignUnit(result, minutes, tokens[index+2], 1); err != nil {
			return index, err
		}
		if index+4 < len(tokens) && tokens[index+3] == ":" {
			result.second, result.microsecond, err = parseSeconds(tokens[index+4])
			if err != nil {
				return index, err
			}
			index += 2
		}
		index += 2
	case index+1 < len(tokens) && (tokens[index+1] == "-" || tokens[index+1] == "/" || tokens[index+1] == "."):
		separator := tokens[index+1]
		if err = date.appendString(representation, -1); err != nil {
			return index, err
		}
		if index+2 < len(tokens) && !jump(tokens[index+2]) {
			if IsDigits(tokens[index+2]) {
				err = date.appendString(tokens[index+2], -1)
			} else if month := lookup(months, tokens[index+2]); month >= 0 {
				err = date.append(month+1, 1, false)
			} else {
				return index, ErrParse
			}
			if err != nil {
				return index, err
			}
			if index+3 < len(tokens) && tokens[index+3] == separator {
				if index+4 >= len(tokens) {
					return index, ErrParse
				}
				if month := lookup(months, tokens[index+4]); month >= 0 {
					err = date.append(month+1, 1, false)
				} else {
					err = date.appendString(tokens[index+4], -1)
				}
				if err != nil {
					return index, err
				}
				index += 2
			}
			index++
		}
		index++
	case index+1 >= len(tokens) || jump(tokens[index+1]):
		if index+2 < len(tokens) && lookup(meridiems, tokens[index+2]) >= 0 {
			hour, parseError := value.integer()
			if parseError != nil {
				return index, parseError
			}
			result.hour = adjustAMPM(hour, lookup(meridiems, tokens[index+2]))
			index++
		} else {
			err = date.appendNumber(value)
		}
		index++
	case lookup(meridiems, tokens[index+1]) >= 0 && value.compare(0) >= 0 && value.compare(24) < 0:
		hour, parseError := value.integer()
		if parseError != nil {
			return index, parseError
		}
		result.hour = adjustAMPM(hour, lookup(meridiems, tokens[index+1]))
		index++
	case date.couldBeDay(value):
		err = date.appendNumber(value)
	default:
		err = ErrParse
	}
	return index, err
}

func couldBeZone(result *parserResult, token string, ignoreOffset bool) bool {
	if result.hour < 0 || result.zoneName != "" || result.hasOffset && !ignoreOffset || utf8.RuneCountInString(token) > 5 {
		return false
	}
	if token == "UTC" || token == "GMT" || token == "Z" || token == "z" {
		return true
	}
	for _, character := range token {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func parseTokens(input string, parserYear int) (parserResult, error) {
	result := newParserResult()
	date := newYMD()
	tokens := lex(input)
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		switch {
		case numeric(token):
			var err error
			index, err = parseNumeric(tokens, index, &date, &result)
			if err != nil {
				return result, err
			}
		case lookup(weekdays, token) >= 0:
			result.weekday = lookup(weekdays, token)
		case lookup(months, token) >= 0:
			if err := date.append(lookup(months, token)+1, 1, false); err != nil {
				return result, err
			}
			if index+1 < len(tokens) && (tokens[index+1] == "-" || tokens[index+1] == "/") {
				separator := tokens[index+1]
				if index+2 >= len(tokens) {
					return result, ErrParse
				}
				if err := date.appendString(tokens[index+2], -1); err != nil {
					return result, err
				}
				if index+3 < len(tokens) && tokens[index+3] == separator {
					if index+4 >= len(tokens) {
						return result, ErrParse
					}
					if err := date.appendString(tokens[index+4], -1); err != nil {
						return result, err
					}
					index += 2
				}
				index += 2
			} else if index+4 < len(tokens) && tokens[index+1] == " " && tokens[index+3] == " " && strings.EqualFold(tokens[index+2], "of") {
				if IsDigits(tokens[index+4]) {
					year, err := intToken(tokens[index+4])
					if err != nil {
						return result, err
					}
					if err = date.appendString(strconv.Itoa(convertYear(year, parserYear, false)), 0); err != nil {
						return result, err
					}
				}
				index += 4
			}
		case lookup(meridiems, token) >= 0:
			if result.hour < 0 || result.hour > 12 {
				return result, ErrParse
			}
			result.ampm = lookup(meridiems, token)
			result.hour = adjustAMPM(result.hour, result.ampm)
		case couldBeZone(&result, token, false):
			result.zoneName = token
			if token == "utc" || token == "gmt" || token == "z" {
				result.zoneOffset, result.hasOffset = 0, true
			}
			if index+1 < len(tokens) && (tokens[index+1] == "+" || tokens[index+1] == "-") {
				if tokens[index+1] == "+" {
					tokens[index+1] = "-"
				} else {
					tokens[index+1] = "+"
				}
				result.hasOffset = false
				if utcZone(result.zoneName) {
					result.zoneName = ""
				}
			}
		case result.hour >= 0 && (token == "+" || token == "-"):
			if index+1 >= len(tokens) {
				return result, ErrParse
			}
			sign := 1
			if token == "-" {
				sign = -1
			}
			hourOffset, minuteOffset := 0, 0
			length := utf8.RuneCountInString(tokens[index+1])
			var err error
			if length == 4 {
				hourOffset, err = intToken(slice(tokens[index+1], 0, 2))
				if err != nil {
					return result, err
				}
				minuteOffset, err = intToken(slice(tokens[index+1], 2, 4))
			} else if index+2 < len(tokens) && tokens[index+2] == ":" {
				if index+3 >= len(tokens) {
					return result, ErrParse
				}
				hourOffset, err = intToken(tokens[index+1])
				if err != nil {
					return result, err
				}
				minuteOffset, err = intToken(tokens[index+3])
				index += 2
			} else if length <= 2 {
				hourOffset, err = intToken(tokens[index+1])
			} else {
				return result, ErrParse
			}
			if err != nil {
				return result, err
			}
			if hourOffset > 1_000_000 || minuteOffset > 1_000_000 {
				return result, ErrParse
			}
			result.zoneOffset, result.hasOffset = sign*(hourOffset*3600+minuteOffset*60), true
			if index+5 < len(tokens) && jump(tokens[index+2]) && tokens[index+3] == "(" && tokens[index+5] == ")" && utf8.RuneCountInString(tokens[index+4]) >= 3 && couldBeZone(&result, tokens[index+4], true) {
				result.zoneName = tokens[index+4]
				index += 4
			}
			index++
		case !jump(token):
			return result, ErrParse
		}
	}
	var err error
	result.year, result.month, result.day, err = date.resolve()
	if err != nil {
		return result, err
	}
	if result.year >= 0 {
		result.year = convertYear(result.year, parserYear, date.century)
	}
	if result.hasOffset && result.zoneOffset == 0 && result.zoneName == "" || result.zoneName == "Z" || result.zoneName == "z" {
		result.zoneName, result.zoneOffset, result.hasOffset = "UTC", 0, true
	} else if (!result.hasOffset || result.zoneOffset != 0) && result.zoneName != "" && utcZone(result.zoneName) {
		result.zoneOffset, result.hasOffset = 0, true
	}
	return result, nil
}

func Parse(input string, defaultTime time.Time, parserYear int) (Result, error) {
	parsed, err := parseTokens(input, parserYear)
	if err != nil {
		return Result{}, err
	}
	if parsed.year < 0 && parsed.month < 0 && parsed.day < 0 && parsed.weekday < 0 && parsed.hour < 0 && parsed.minute < 0 && parsed.second < 0 && parsed.microsecond < 0 && parsed.ampm < 0 && parsed.zoneName == "" && !parsed.hasOffset {
		return Result{}, ErrParse
	}
	values := []int{parsed.year, parsed.month, parsed.day, parsed.hour, parsed.minute, parsed.second, parsed.microsecond}
	defaults := []int{defaultTime.Year(), int(defaultTime.Month()), defaultTime.Day(), defaultTime.Hour(), defaultTime.Minute(), defaultTime.Second(), defaultTime.Nanosecond() / 1000}
	for index := range values {
		if values[index] < 0 {
			values[index] = defaults[index]
		}
	}
	if parsed.day < 0 && values[2] > daysInMonth(values[0], values[1]) {
		values[2] = daysInMonth(values[0], values[1])
	}
	if values[0] < 1 || values[0] > 9999 || values[1] < 1 || values[1] > 12 || values[2] < 1 || values[2] > daysInMonth(values[0], values[1]) || values[3] < 0 || values[3] > 23 || values[4] < 0 || values[4] > 59 || values[5] < 0 || values[5] > 59 || values[6] < 0 || values[6] > 999999 {
		return Result{}, ErrParse
	}
	wall := time.Date(values[0], time.Month(values[1]), values[2], values[3], values[4], values[5], values[6]*1000, time.UTC)
	if parsed.weekday >= 0 && parsed.day <= 0 {
		weekday := (int(wall.Weekday()) + 6) % 7
		wall = wall.AddDate(0, 0, (parsed.weekday-weekday+7)%7)
		if wall.Year() > 9999 {
			return Result{}, ErrParse
		}
	}
	location := defaultTime.Location()
	aware := false
	offset := 0
	if parsed.zoneName != "" {
		winterName, _ := time.Date(parserYear, 1, 1, 0, 0, 0, 0, location).Zone()
		summerName, _ := time.Date(parserYear, 7, 1, 0, 0, 0, 0, location).Zone()
		if parsed.zoneName == winterName || parsed.zoneName == summerName {
			local := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), location)
			localName, localOffset := local.Zone()
			if localName != parsed.zoneName {
				_, otherOffset := local.Add(24 * time.Hour).Zone()
				folded := local.Add(time.Duration(localOffset-otherOffset) * time.Second)
				if name, _ := folded.Zone(); name == parsed.zoneName && folded.Hour() == wall.Hour() && folded.Day() == wall.Day() {
					local = folded
				}
			}
			localName, offset = local.Zone()
			aware = true
			if localName != parsed.zoneName && (parsed.zoneName == "UTC" || parsed.zoneName == "GMT" || parsed.zoneName == "Z" || parsed.zoneName == "z") {
				offset = 0
				location = time.UTC
			} else {
				location = local.Location()
			}
		}
	}
	if !aware && parsed.hasOffset {
		if parsed.zoneOffset <= -86400 || parsed.zoneOffset >= 86400 {
			return Result{}, ErrParse
		}
		offset, aware = parsed.zoneOffset, true
		location = time.FixedZone(parsed.zoneName, offset)
		if offset == 0 {
			location = time.UTC
		}
	}
	if aware {
		location = time.FixedZone(parsed.zoneName, offset)
	}
	value := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), location)
	return Result{Time: value, Aware: aware, Offset: time.Duration(offset) * time.Second}, nil
}
