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
	Fold        bool   `json:"fold"`
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
	return actual.Time.Year() == expected.Year && int(actual.Time.Month()) == expected.Month && actual.Time.Day() == expected.Day && actual.Time.Hour() == expected.Hour && actual.Time.Minute() == expected.Minute && actual.Time.Second() == expected.Second && actual.Time.Nanosecond()/1000 == expected.Microsecond && actual.Aware == expected.Aware && actual.Fold == expected.Fold && (!actual.Aware || int(actual.Offset/time.Second) == expected.Offset)
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

func TestPythonUnicodeHelpers(test *testing.T) {
	cases := []struct {
		input     string
		digits    bool
		ascii     string
		converted bool
	}{
		{"", false, "", true},
		{"2024", true, "2024", true},
		{"\u0662\u0660\u0662\u0664", true, "2024", true},
		{"\u00b2", true, "", false},
		{"\U0001d7da\U0001d7d8\U0001d7da\U0001d7dc", true, "2024", true},
		{"2024-03-10", false, "2024-03-10", true},
	}
	for _, entry := range cases {
		ascii, converted := ASCIIDecimal(entry.input)
		if IsDigits(entry.input) != entry.digits || ascii != entry.ascii || converted != entry.converted {
			test.Errorf("%q: digits=%t, ASCII=%q converted=%t; expected %+v", entry.input, IsDigits(entry.input), ascii, converted, entry)
		}
	}
}

func TestPythonNaiveGapWallTime(test *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		test.Fatal(err)
	}
	defaultTime := time.Date(2026, time.January, 31, 0, 0, 0, 0, location)
	for _, input := range []string{"2024 March 10 02:30", "2024 March 10 02:30 XYZ"} {
		actual, err := Parse(input, defaultTime, 2026)
		if err != nil {
			test.Errorf("%q: %v", input, err)
			continue
		}
		if actual.Aware || actual.Time.Format("2006-01-02T15:04:05") != "2024-03-10T02:30:00" {
			test.Errorf("%q: Go=%+v, Python preserves the naive gap wall time", input, actual)
		}
	}
}

func TestPythonParserContexts(test *testing.T) {
	content, err := os.ReadFile("context-reference.json")
	if err != nil {
		test.Fatal(err)
	}
	var fixture struct {
		Python        string `json:"python"`
		CPythonCommit string `json:"cpython_commit"`
		Dateutil      string `json:"dateutil"`
		ParserSHA256  string `json:"parser_sha256"`
		TZLocalSHA256 string `json:"tzlocal_sha256"`
		TZData        string `json:"tzdata"`
		Contexts      map[string]struct {
			Zone           string    `json:"zone"`
			Names          [2]string `json:"names"`
			StandardOffset int       `json:"standard_offset"`
			DaylightOffset int       `json:"daylight_offset"`
		} `json:"contexts"`
		Cases []struct {
			Context     string       `json:"context"`
			Input       string       `json:"input"`
			Default     string       `json:"default"`
			DefaultFold bool         `json:"default_fold"`
			ParserYear  int          `json:"parser_year"`
			Expected    expectedDate `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		test.Fatal(err)
	}
	if fixture.Python != "3.14.6" || fixture.CPythonCommit != "c63aec69bd59c55314c06c23f4c22c03de76fe45" || fixture.Dateutil != "2.9.0.post0" || fixture.ParserSHA256 != "ee494377289c92c401fd7825fb7500981c33098a2bd40219aa4948713e9d1fff" || fixture.TZLocalSHA256 != "1149c474c7de4e15e263a978b21f7205a6d9eb7fcf3b3cb4d734ac87db619f5a" || fixture.TZData != "2026.3" {
		test.Fatal("unexpected parser context reference source")
	}
	if len(fixture.Cases) != 2804 || len(fixture.Contexts) != 10 {
		test.Fatal("incomplete parser context reference")
	}
	for _, testCase := range fixture.Cases {
		context, ok := fixture.Contexts[testCase.Context]
		if !ok {
			test.Fatalf("missing context %q", testCase.Context)
		}
		location, err := time.LoadLocation(context.Zone)
		if err != nil {
			test.Fatal(err)
		}
		defaultTime, err := time.Parse("2006-01-02T15:04:05.999999", testCase.Default)
		if err != nil {
			test.Fatal(err)
		}
		local := LocalTimezone{
			StandardName: context.Names[0], DaylightName: context.Names[1],
			StandardOffset: context.StandardOffset, DaylightOffset: context.DaylightOffset,
			IsDST: func(instant time.Time) bool { return instant.In(location).IsDST() },
		}
		actual, err := ParseWithLocalTimezone(testCase.Input, defaultTime, testCase.ParserYear, local, testCase.DefaultFold)
		if !matchesDate(actual, err, testCase.Expected) {
			test.Errorf("context=%s default=%s year=%d input=%q: Go=%+v error=%v, Python=%+v", testCase.Context, testCase.Default, testCase.ParserYear, testCase.Input, actual, err, testCase.Expected)
		}
		if err == nil && testCase.Expected.Error == "" {
			expected := testCase.Expected
			instant := time.Date(expected.Year, time.Month(expected.Month), expected.Day, expected.Hour, expected.Minute, expected.Second, expected.Microsecond*1000, time.UTC).Add(-time.Duration(expected.Offset) * time.Second)
			if !actual.Instant().Equal(instant) {
				test.Errorf("context=%s input=%q: instant=%v, expected=%v", testCase.Context, testCase.Input, actual.Instant(), instant)
			}
		}
		if testCase.Context != "Windows_Eastern" && !testCase.DefaultFold {
			localDefault := time.Date(defaultTime.Year(), defaultTime.Month(), defaultTime.Day(), defaultTime.Hour(), defaultTime.Minute(), defaultTime.Second(), defaultTime.Nanosecond(), location)
			inferred, inferredErr := Parse(testCase.Input, localDefault, testCase.ParserYear)
			if !matchesDate(inferred, inferredErr, testCase.Expected) {
				test.Errorf("inferred context=%s default=%s year=%d input=%q: Go=%+v error=%v, Python=%+v", testCase.Context, testCase.Default, testCase.ParserYear, testCase.Input, inferred, inferredErr, testCase.Expected)
			}
		}
	}
	test.Logf("%d Python parser context cases", len(fixture.Cases))
}
