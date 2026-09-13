package relativedelta

import (
	"encoding/json"
	"os"
	"testing"
	"time"
	_ "time/tzdata"
)

func TestPythonRelativeDelta(t *testing.T) {
	content, err := os.ReadFile("reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Dateutil     string `json:"dateutil"`
		SourceSHA256 string `json:"source_sha256"`
		Cases        []struct {
			Base     string `json:"base"`
			Zone     string `json:"zone"`
			Delta    Delta  `json:"delta"`
			Expected string `json:"expected"`
			Error    string `json:"error"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Dateutil != "2.9.0.post0" || fixture.SourceSHA256 != "218fe6825323a196d87ebbe5a1363671ed366a25633c892265faa03536517ee0" || len(fixture.Cases) < 100 {
		t.Fatal("invalid Python reference")
	}
	failures := 0
	for index, testCase := range fixture.Cases {
		base, err := time.Parse(time.RFC3339Nano, testCase.Base)
		if err != nil {
			t.Fatal(err)
		}
		if testCase.Zone != "" {
			location, err := time.LoadLocation(testCase.Zone)
			if err != nil {
				t.Fatal(err)
			}
			base = base.In(location)
		}
		actual, err := testCase.Delta.Apply(base)
		matches := err != nil && testCase.Error != ""
		if err == nil && testCase.Error == "" {
			expected, parseError := time.Parse(time.RFC3339Nano, testCase.Expected)
			if parseError != nil {
				t.Fatal(parseError)
			}
			_, actualOffset := actual.Zone()
			_, expectedOffset := expected.Zone()
			matches = actual.Equal(expected) && actualOffset == expectedOffset
		}
		if !matches {
			failures++
			if failures <= 30 {
				t.Errorf("case %d base=%s delta=%+v Go=%s error=%v Python=%s error=%s", index, testCase.Base, testCase.Delta, actual.Format(time.RFC3339Nano), err, testCase.Expected, testCase.Error)
			}
		}
	}
	t.Logf("%d independent Python cases, %d differences", len(fixture.Cases), failures)
}
