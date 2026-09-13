package parser

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

type expectedDate struct {
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
	Hour        int    `json:"hour"`
	Minute      int    `json:"minute"`
	Second      int    `json:"second"`
	Microsecond int    `json:"microsecond"`
	Aware       bool   `json:"aware"`
	Offset      int    `json:"offset"`
	Error       string `json:"error"`
}

type reference struct {
	Python       string `json:"python"`
	Dateutil     string `json:"dateutil"`
	Unicode      string `json:"unicode"`
	Default      string `json:"default"`
	ParserYear   int    `json:"parser_year"`
	ParserSHA256 string `json:"parser_sha256"`
	Cases        []struct {
		Input    string       `json:"input"`
		Tokens   []string     `json:"tokens"`
		Dateutil expectedDate `json:"dateutil"`
	} `json:"cases"`
}

func loadReference(t *testing.T) reference {
	t.Helper()
	content, err := os.ReadFile("reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture reference
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Python != "3.14.6" || fixture.Dateutil != "2.9.0.post0" || fixture.Unicode != "16.0.0" || fixture.ParserSHA256 != "ee494377289c92c401fd7825fb7500981c33098a2bd40219aa4948713e9d1fff" {
		t.Fatal("unexpected reference source")
	}
	if len(fixture.Cases) < 3273 {
		t.Fatal("incomplete reference fixture")
	}
	return fixture
}

func TestPythonLexer(t *testing.T) {
	fixture := loadReference(t)
	var failures int
	for _, testCase := range fixture.Cases {
		actual := lex(testCase.Input)
		if !reflect.DeepEqual(actual, testCase.Tokens) {
			failures++
			if failures <= 30 {
				t.Errorf("%q: tokens=%q, Python=%q", testCase.Input, actual, testCase.Tokens)
			}
		}
	}
	t.Logf("%d cases, %d differences", len(fixture.Cases), failures)
}

func matchesDate(actual Result, err error, expected expectedDate) bool {
	if expected.Error != "" {
		return err != nil
	}
	if err != nil {
		return false
	}
	return actual.Time.Year() == expected.Year && int(actual.Time.Month()) == expected.Month && actual.Time.Day() == expected.Day && actual.Time.Hour() == expected.Hour && actual.Time.Minute() == expected.Minute && actual.Time.Second() == expected.Second && actual.Time.Nanosecond()/1000 == expected.Microsecond && actual.Aware == expected.Aware && (!actual.Aware || int(actual.Offset/time.Second) == expected.Offset)
}

func TestPythonDateutil(t *testing.T) {
	fixture := loadReference(t)
	defaultTime, err := time.Parse("2006-01-02T15:04:05", fixture.Default)
	if err != nil {
		t.Fatal(err)
	}
	var failures int
	for _, testCase := range fixture.Cases {
		actual, err := Parse(testCase.Input, defaultTime, fixture.ParserYear)
		if !matchesDate(actual, err, testCase.Dateutil) {
			failures++
			if failures <= 30 {
				t.Errorf("%q: Go=%+v error=%v, Python=%+v", testCase.Input, actual, err, testCase.Dateutil)
			}
		}
	}
	t.Logf("%d cases, %d differences", len(fixture.Cases), failures)
}
