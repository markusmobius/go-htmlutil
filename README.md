# Go-Dateutil

A native Go implementation of the subset of Python
[python-dateutil](https://github.com/dateutil/dateutil) used by Python
HtmlDate and DateParser. This is not a complete port of python-dateutil.

The supported subset is the union of the Python-dateutil functionality required
by **either** consumer. Missing dependency behavior belongs here, not in
consumer-local parser or arithmetic substitutes.

Version **2.9.1** corresponds to **python-dateutil 2.9.0.post0**, with separately
tracked **CPython 3.14.6** datetime compatibility. Python's `.post0` packaging suffix is
recorded here because it is not a Go semantic-version suffix.

## Installation

Requires Go 1.26.0 or later.

```sh
go get github.com/markusmobius/go-dateutil/v2@v2.9.1
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
are outside this subset. The relative-delta API does not implement absolute
field replacement, differences between two dates, delta arithmetic, or leap-day
and ordinal-day options. No consumer path currently being ported requires them.

## Added in v2.9.1

Version 2.9.1 adds `compat/datetime.FromISOFormat`, the separate
CPython 3.14.6 `datetime.fromisoformat` operation needed by HtmlDate. It lives in
this shared dependency so consumers do not carry their own ISO parser. This is
not `dateutil.parser.isoparse`. The consumer audit also corrects naive wall-time
normalization and adds an explicit local environment/fold parser API.
The published v2.9.0 tag remains unchanged; the new package is not in that tag.

The API accepts valid UTF-8 strings and returns `parser.Result`, retaining wall
fields, timezone awareness, and microsecond offset precision. `Result.Instant`
provides the corresponding instant for aware results. It supports CPython's
calendar/week-date parsing, clock syntax, fixed offsets, rejection behavior, and
midnight rollover. Callers retain responsibility for date bounds and formatting.
Lone Python surrogates have no equivalent valid-UTF-8 input; four upstream
examples are explicitly recorded outside this contract, not silently substituted.

Its independent oracle contains 6,944 CPython results. Source hashes, the exact
CPython commit, and the upstream test inputs are recorded separately from
python-dateutil in [UPSTREAM.md](UPSTREAM.md) and
[compat/datetime/reference.json](compat/datetime/reference.json). The original
[CPython license](compat/datetime/LICENSE-CPYTHON.txt) accompanies the port.

`compat/datetime.Timestamp(result, local, fold)` implements CPython's timestamp
conversion used by HtmlDate bounds checks: naive values use the supplied local
zone, aware values use their parsed offset, and floating-point rounding matches
Python. Pass `result.Fold` for Dateutil results, or `false` for ISO results.
The [timestamp oracle](compat/datetime/timestamp-reference.json) checks 816 exact
floating-point results. `Result.Instant` alone does not resolve naive local time.

The new `parser.ParseWithLocalTimezone(input, defaultTime, parserYear, local,
defaultFold)` accepts a `LocalTimezone` snapshot of standard/daylight names and
offsets plus an `IsDST` lookup on UTC instants. It preserves wall fields,
awareness, offset, and `Result.Fold`, following Dateutil's actual `tzlocal`
algorithm. `parser.Parse` remains the convenience entry point, inferring seasonal
context from the default's Go location and using fold zero. Use explicit context
when reproducing a particular Python environment, including Windows zone names.
The separate [context oracle](parser/context-reference.json) covers 2,804 cases
across ten environments and independently varied defaults, years, and folds.

Python-compatible `IsDigit`, `IsDigits`, `IsSpace`, and `ASCIIDecimal` helpers
reuse the parser's pinned Unicode tables for consumer character gates.
`ASCIIDecimal` translates decimal digits and preserves ASCII; it is not an integer
parser. Empty strings are not `IsDigits`, and superscript digits need not be
decimal digits. See the complete source/call inventory and consumer gates in
[UPSTREAM.md](UPSTREAM.md#combined-consumer-audit).

## Verification

The five fixtures provide **17,256 Python-derived comparisons**: 3,273 lexer,
3,273 parser, 146 relative-arithmetic, 6,944 ISO, 2,804 parser-context, and 816
timestamp outcomes. The original v2.9.0 fixtures and Unicode data are unchanged.

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

The generator does not depend on another checkout.
It verifies pinned source hashes, reruns Python, and checks the fixtures
and Unicode tables byte-for-byte. The ISO reference additionally downloads the
hash-pinned CPython source/test files from the recorded commit and checks its
separate fixture. Context and timestamp references also regenerate independently.
Omit `--check` only for a reviewed reference
refresh. Source mappings and compatibility boundaries are in
[UPSTREAM.md](UPSTREAM.md).

The implementation does not invoke Python, use CGO, or create worker threads.
Callers own concurrency. The standalone subset tests do not establish complete
HtmlDate or DateParser consumer compatibility; those projects have their own
integration tests.