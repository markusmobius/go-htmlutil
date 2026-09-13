package datetime

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

func TestFromISOFormat(test *testing.T) {
	cases := []struct {
		input    string
		expected string
		aware    bool
		offset   time.Duration
	}{
		{"2020-W53", "2020-12-28T00:00:00", false, 0},
		{"2020-W53-7", "2021-01-03T00:00:00", false, 0},
		{"2020W537", "2021-01-03T00:00:00", false, 0},
		{"2021-W01-1", "2021-01-04T00:00:00", false, 0},
		{"2021-W53-1", "", false, 0},
		{"2020-W53-7T23:59:59+05:30", "2021-01-03T23:59:59", true, 5*time.Hour + 30*time.Minute},
		{"2020-W53-7T24:00", "2021-01-04T00:00:00", false, 0},
	}
	for _, entry := range cases {
		actual, err := FromISOFormat(entry.input)
		if entry.expected == "" {
			if !errors.Is(err, ErrISOFormat) {
				test.Errorf("%q: expected an ISO parse error, got %+v, %v", entry.input, actual, err)
			}
			continue
		}
		if err != nil || actual.Time.Format("2006-01-02T15:04:05") != entry.expected || actual.Aware != entry.aware || actual.Offset != entry.offset {
			test.Errorf("%q: got %+v, %v; expected %+v", entry.input, actual, err, entry)
		}
	}
}

func TestPythonISOReference(test *testing.T) {
	contents, err := os.ReadFile("reference.json")
	if err != nil {
		test.Fatal(err)
	}
	var fixture struct {
		Python            string            `json:"python"`
		SourceCommit      string            `json:"source_commit"`
		UnsupportedInputs []json.RawMessage `json:"unsupported_inputs"`
		Cases             []struct {
			Input    string `json:"input"`
			Expected struct {
				Year, Month, Day, Hour, Minute, Second, Microsecond int
				Aware                                               bool
				OffsetMicroseconds                                  int64 `json:"offset_microseconds"`
				Error                                               string
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		test.Fatal(err)
	}
	if fixture.Python != "3.14.6" || fixture.SourceCommit != "c63aec69bd59c55314c06c23f4c22c03de76fe45" {
		test.Fatal("unexpected CPython reference provenance")
	}
	if len(fixture.Cases) != 6944 || len(fixture.UnsupportedInputs) != 4 {
		test.Fatal("unexpected CPython reference provenance or incomplete corpus")
	}
	for _, entry := range fixture.Cases {
		actual, err := FromISOFormat(entry.Input)
		expected := entry.Expected
		if expected.Error != "" {
			if !errors.Is(err, ErrISOFormat) {
				test.Errorf("%q: Python rejected input (%s), Go returned %+v, %v", entry.Input, expected.Error, actual, err)
			}
			continue
		}
		expectedTime := time.Date(expected.Year, time.Month(expected.Month), expected.Day, expected.Hour, expected.Minute, expected.Second, expected.Microsecond*1000, time.UTC)
		expectedOffset := time.Duration(expected.OffsetMicroseconds) * time.Microsecond
		if err != nil || !actual.Time.Equal(expectedTime) || actual.Aware != expected.Aware || actual.Offset != expectedOffset {
			test.Errorf("%q: Go=%+v, error=%v; Python=%+v", entry.Input, actual, err, expected)
		} else if !actual.Instant().Equal(expectedTime.Add(-expectedOffset)) {
			test.Errorf("%q: incorrect instant %s", entry.Input, actual.Instant())
		}
	}
	test.Logf("compared %d independent CPython ISO results", len(fixture.Cases))
}

func TestPythonTimestampReference(test *testing.T) {
	contents, err := os.ReadFile("timestamp-reference.json")
	if err != nil {
		test.Fatal(err)
	}
	var fixture struct {
		Python       string `json:"python"`
		SourceCommit string `json:"source_commit"`
		SourceSHA256 string `json:"source_sha256"`
		Cases        []struct {
			Zone     string `json:"zone"`
			Input    string `json:"input"`
			Fold     bool   `json:"fold"`
			Expected struct {
				Bits  uint64 `json:"bits"`
				Error string `json:"error"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		test.Fatal(err)
	}
	if fixture.Python != "3.14.6" || fixture.SourceCommit != "c63aec69bd59c55314c06c23f4c22c03de76fe45" || fixture.SourceSHA256 != "934a84bbfac41fc43c5c30e86338bf1c31cf282f5215945a4fc79801d1f01cf6" || len(fixture.Cases) != 816 {
		test.Fatal("unexpected CPython timestamp reference provenance")
	}
	for _, entry := range fixture.Cases {
		value, err := FromISOFormat(entry.Input)
		if err != nil {
			test.Fatalf("%q: %v", entry.Input, err)
		}
		local, err := time.LoadLocation(entry.Zone)
		if err != nil {
			test.Fatal(err)
		}
		actual, err := Timestamp(value, local, entry.Fold)
		if entry.Expected.Error != "" {
			if err == nil {
				test.Errorf("%q zone=%s fold=%t: Python rejected timestamp (%s), Go=%v", entry.Input, entry.Zone, entry.Fold, entry.Expected.Error, actual)
			}
		} else if err != nil || math.Float64bits(actual) != entry.Expected.Bits {
			test.Errorf("%q zone=%s fold=%t: Go=%.17g (%x), error=%v; Python=%.17g (%x)", entry.Input, entry.Zone, entry.Fold, actual, math.Float64bits(actual), err, math.Float64frombits(entry.Expected.Bits), entry.Expected.Bits)
		}
	}
}
