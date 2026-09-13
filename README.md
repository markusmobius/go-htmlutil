# Go-Dateutil

A native Go implementation of the subset of Python
[python-dateutil](https://github.com/dateutil/dateutil) used by Python
HtmlDate and DateParser. This is not a complete port of python-dateutil.

The compatibility version is **2.9.0**, corresponding to upstream
**python-dateutil 2.9.0.post0**. Python's `.post0` packaging suffix is recorded
here because it is not a Go semantic-version suffix.

## Installation

Requires Go 1.26.0 or later.

```sh
go get github.com/markusmobius/go-dateutil/v2@v2.9.0
```

Import `github.com/markusmobius/go-dateutil/v2/parser` for parsing and
`github.com/markusmobius/go-dateutil/v2/relativedelta` for relative arithmetic.

## Scope

- `parser`: the English, non-fuzzy parser used by HtmlDate, with explicit
  default datetime and parser initialization year. Tokenization, field
  resolution, omitted-day clamping, numeric offsets, and rejection behavior
  follow the Python implementation.
- `relativedelta`: relative years/months/weeks/days/hours/minutes/seconds and
  microseconds. Month ends clamp before smaller units are added. Fractional
  years/months are rejected; supported fractional units round to microseconds.

Custom parser-info classes, fuzzy parsing, custom timezone callbacks, recurrence
rules, Easter calculations, timezone-file loading, and dateutil's other APIs
are outside this subset. CPython's `datetime.fromisoformat` is not dateutil and
is not part of this module. The relative-delta API does not implement absolute
field replacement, differences between two dates, delta arithmetic, or leap-day
and ordinal-day options. No consumer path currently being ported requires them.

## Verification

The parser's offline oracle contains 3,273 independent results from
python-dateutil 2.9.0.post0 under CPython 3.14.6. It checks token streams,
calendar and clock fields, microseconds, offset awareness, and acceptance or
rejection. Python exception wording and Python warning delivery are not Go
APIs. Python sources, Unicode version, and frozen defaults are recorded in the
fixture. This coverage is not a claim that all dateutil APIs are implemented.
Relative arithmetic has 146 Python-generated cases, including leap years,
month ends, mixed units, fractional values, overflows, DST gaps and folds.

Both test suites and their saved Python fixtures ship with the Go module:
[parser/pythondate_test.go](parser/pythondate_test.go) and
[relativedelta/relativedelta_test.go](relativedelta/relativedelta_test.go).
Normal tests do not require Python, another checkout, or a local replacement.

```sh
go test -mod=readonly ./...
go vet -mod=readonly ./...
```

Reference regeneration is opt-in and requires CPython 3.14.6 plus the packages
in [tools/python-reference/requirements.txt](tools/python-reference/requirements.txt):

```sh
python tools/python-reference/export.py --check
```

The generator uses only this repository's input cases, never another checkout.
It verifies pinned source hashes, reruns Python, and checks both fixture files
and Unicode tables byte-for-byte. Omit `--check` only for a reviewed reference
refresh. Source mappings and compatibility boundaries are in
[UPSTREAM.md](UPSTREAM.md).

The implementation does not invoke Python, use CGO, or create worker threads.
Callers own concurrency. The standalone subset tests do not establish complete
HtmlDate or DateParser consumer compatibility; those projects have their own
integration tests.