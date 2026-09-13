package parser

import (
	"sort"
	"strings"
)

func inRanges(value rune, ranges [][2]rune) bool {
	index := sort.Search(len(ranges), func(index int) bool { return ranges[index][1] >= value })
	return index < len(ranges) && ranges[index][0] <= value
}

func IsDigit(value rune) bool {
	return inRanges(value, digitRanges)
}

func IsDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !IsDigit(character) {
			return false
		}
	}
	return true
}

func IsSpace(value rune) bool {
	return inRanges(value, spaceRanges)
}

func ASCIIDecimal(value string) (string, bool) {
	return asciiDecimal(value)
}

func decimalDigit(value rune) (int, bool) {
	index := sort.Search(len(decimalZeroes), func(index int) bool { return decimalZeroes[index]+9 >= value })
	if index < len(decimalZeroes) && value >= decimalZeroes[index] {
		return int(value - decimalZeroes[index]), true
	}
	return 0, false
}

func asciiDecimal(value string) (string, bool) {
	var result strings.Builder
	for _, character := range value {
		if digit, ok := decimalDigit(character); ok {
			result.WriteByte(byte('0' + digit))
		} else if character < 128 {
			result.WriteRune(character)
		} else {
			return "", false
		}
	}
	return result.String(), true
}

func lex(input string) []string {
	characters := []rune(strings.ReplaceAll(input, "\x00", ""))
	result := []string{}
	for cursor := 0; cursor < len(characters); {
		state := ""
		seenLetters := false
		token := []rune{}
		for cursor < len(characters) {
			character := characters[cursor]
			cursor++
			word := inRanges(character, letterRanges)
			number := IsDigit(character)
			if state == "" {
				token = append(token, character)
				switch {
				case word:
					state = "a"
				case number:
					state = "0"
				case IsSpace(character):
					token[0] = ' '
				}
				if state == "" {
					break
				}
				continue
			}
			accepted := false
			switch state {
			case "a":
				seenLetters = true
				if word {
					accepted = true
				} else if character == '.' {
					accepted, state = true, "a."
				}
			case "0":
				if number {
					accepted = true
				} else if character == '.' || character == ',' && len(token) >= 2 {
					accepted, state = true, "0."
				}
			case "a.":
				seenLetters = true
				if character == '.' || word {
					accepted = true
				} else if number && token[len(token)-1] == '.' {
					accepted, state = true, "0."
				}
			case "0.":
				if character == '.' || number {
					accepted = true
				} else if word && token[len(token)-1] == '.' {
					accepted, state = true, "a."
				}
			}
			if !accepted {
				cursor--
				break
			}
			token = append(token, character)
		}
		text := string(token)
		if (state == "a." || state == "0.") && (seenLetters || strings.Count(text, ".") > 1 || strings.HasSuffix(text, ".") || strings.HasSuffix(text, ",")) {
			start := 0
			for index, character := range text {
				if character == '.' || character == ',' {
					if index > start {
						result = append(result, text[start:index])
					}
					result = append(result, string(character))
					start = index + 1
				}
			}
			if start < len(text) {
				result = append(result, text[start:])
			}
		} else {
			if state == "0." && !strings.Contains(text, ".") {
				text = strings.ReplaceAll(text, ",", ".")
			}
			result = append(result, text)
		}
	}
	return result
}
