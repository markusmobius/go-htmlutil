"""Regenerate only the python-dateutil functionality used by HtmlDate/DateParser."""

import argparse
import ast
from datetime import datetime, timezone
import hashlib
import importlib.metadata
import importlib.resources
from itertools import product
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
from types import SimpleNamespace
from unittest.mock import patch
import unicodedata
from urllib.request import urlopen
import warnings
from zoneinfo import ZoneInfo

from dateutil.parser import _parser
import dateutil.relativedelta as relative_module
from dateutil.relativedelta import relativedelta
import dateutil.tz.tz as timezone_module


ROOT = Path(__file__).resolve().parents[2]
DEFAULT = datetime(2026, 9, 13)
CPYTHON_COMMIT = "c63aec69bd59c55314c06c23f4c22c03de76fe45"
CPYTHON_SOURCES = {
    "Modules/_datetimemodule.c": "934a84bbfac41fc43c5c30e86338bf1c31cf282f5215945a4fc79801d1f01cf6",
    "Lib/_pydatetime.py": "2110f90af43143761566480888666874549b2a8996af947fa55fec12fc359cca",
    "Lib/test/datetimetester.py": "821d59dcafac9d0e88440914494e94a9f61f764727404bf7a8d74f6a6673eb09",
}


def write(path, value, check):
    expected = (json.dumps(value, ensure_ascii=True, indent=2) + "\n").encode()
    if check:
        if path.read_bytes() != expected:
            raise RuntimeError(f"reference changed: {path.relative_to(ROOT)}")
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(expected)


def parse_result(parser, text, default=DEFAULT, include_fold=False):
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("ignore")
            value = parser.parse(text, default=default, fuzzy=False)
        offset = value.utcoffset()
        result = {"year": value.year, "month": value.month, "day": value.day,
                  "hour": value.hour, "minute": value.minute, "second": value.second,
                  "microsecond": value.microsecond, "aware": offset is not None,
                  "offset": int(offset.total_seconds()) if offset is not None else 0, "error": ""}
        if include_fold:
            result["fold"] = bool(value.fold)
        return result
    except (ValueError, OverflowError, TypeError) as error:
        return {"error": type(error).__name__}


def main():
    arguments = argparse.ArgumentParser(description=__doc__)
    arguments.add_argument("--check", action="store_true")
    arguments.add_argument("--gofmt", default="gofmt")
    arguments.add_argument("--parser-context-only", action="store_true")
    arguments.add_argument("--timestamp-only", action="store_true")
    args = arguments.parse_args()
    if platform.python_version() != "3.14.6":
        raise RuntimeError("expected CPython 3.14.6")
    for requirement in (Path(__file__).with_name("requirements.txt")).read_text().splitlines():
        name, version = requirement.split("==")
        if importlib.metadata.version(name) != version:
            raise RuntimeError(f"expected {requirement}")
    parser_hash = hashlib.sha256(Path(_parser.__file__).read_bytes()).hexdigest()
    if parser_hash != "ee494377289c92c401fd7825fb7500981c33098a2bd40219aa4948713e9d1fff":
        raise RuntimeError("python-dateutil parser source changed")
    relative_hash = hashlib.sha256(Path(relative_module.__file__).read_bytes()).hexdigest()
    if relative_hash != "218fe6825323a196d87ebbe5a1363671ed366a25633c892265faa03536517ee0":
        raise RuntimeError("python-dateutil relativedelta source changed")
    if unicodedata.unidata_version != "16.0.0":
        raise RuntimeError("unexpected Python Unicode version")
    if args.parser_context_only:
        export_parser_context(args.check)
        return
    if args.timestamp_only:
        export_timestamp(args.check)
        return
    native = ["package parser", ""]
    for name, predicate in (("letterRanges", str.isalpha), ("digitRanges", str.isdigit), ("spaceRanges", str.isspace)):
        spans = []
        for value in range(0x110000):
            if predicate(chr(value)):
                if spans and spans[-1][1] == value - 1:
                    spans[-1][1] = value
                else:
                    spans.append([value, value])
        native.append(f"var {name} = [][2]rune{{")
        native.extend(f"\t{{0x{first:x}, 0x{last:x}}}," for first, last in spans)
        native.extend(["}", ""])
    native.append("var decimalZeroes = []rune{")
    native.extend(f"\t0x{value:x}," for value in range(0x110000) if unicodedata.decimal(chr(value), -1) == 0)
    native.extend(["}", ""])
    native_bytes = subprocess.run([args.gofmt], input="\n".join(native).encode(), stdout=subprocess.PIPE, check=True).stdout
    native_path = ROOT / "parser/unicode.go"
    if args.check:
        if native_path.read_bytes() != native_bytes:
            raise RuntimeError("generated Python Unicode tables changed")
    else:
        native_path.write_bytes(native_bytes)
    fixture = json.loads((ROOT / "parser/reference.json").read_text())
    parser = _parser.parser()
    parser.info._year = DEFAULT.year
    parser.info._century = DEFAULT.year // 100 * 100
    fixture["cases"] = [{"input": case["input"], "tokens": _parser._timelex.split(case["input"]),
                         "dateutil": parse_result(parser, case["input"])} for case in fixture["cases"]]
    write(ROOT / "parser/reference.json", fixture, args.check)

    bases = [datetime(2023, 1, 31, 12, 34, 56, 123456, timezone.utc),
             datetime(2024, 2, 29, 23, 59, 59, 999999, timezone.utc),
             datetime(2026, 9, 13, 12, tzinfo=timezone.utc),
             datetime(1, 1, 1, tzinfo=timezone.utc), datetime(9999, 12, 31, tzinfo=timezone.utc)]
    deltas = [{}, {"years": 1}, {"years": -1}, {"months": 1}, {"months": -1},
              {"months": 13}, {"months": -25}, {"years": 1, "months": -12},
              {"years": 0.5}, {"years": -1.5}, {"months": 1.5}, {"months": -0.1},
              {"days": 0.5}, {"days": -1.25}, {"weeks": 1.5}, {"hours": 25.5},
              {"minutes": -90.25}, {"seconds": 0.5}, {"seconds": -0.0000015},
              {"microseconds": 0.5}, {"microseconds": 1.5}, {"microseconds": 1000000.5},
              {"years": 2, "months": 3, "days": 4.5, "hours": -2, "minutes": 12.3, "seconds": 0.75},
              {"days": 1000000000}, {"years": 10000}, {"months": 120000}]
    cases = []
    for base in bases:
        for delta in deltas:
            cases.append(relative_case(base, delta))
    for text in ("2024-03-09T02:30:00", "2024-03-09T12:00:00", "2024-11-02T01:30:00", "2024-11-02T12:00:00"):
        base = datetime.fromisoformat(text).replace(tzinfo=ZoneInfo("America/New_York"))
        for delta in ({"days": 1}, {"hours": 24}, {"hours": 25}, {"months": 1}):
            cases.append(relative_case(base, delta, "America/New_York"))
    output = {"dateutil": "2.9.0.post0", "python": platform.python_version(),
              "source_sha256": relative_hash,
              "tzdata": importlib.metadata.version("tzdata"), "cases": cases}
    write(ROOT / "relativedelta/reference.json", output, args.check)
    print(f"Validated Python references: {len(fixture['cases'])} parser, {len(cases)} relative arithmetic")
    export_iso(fixture["cases"], args.check)
    export_parser_context(args.check)
    export_timestamp(args.check)


def relative_case(base, delta, zone=""):
    try:
        expected = (base + relativedelta(**delta)).isoformat()
        error = ""
    except (ValueError, OverflowError, TypeError) as exception:
        expected, error = "", type(exception).__name__
    return {"base": base.isoformat(), "zone": zone, "delta": delta, "expected": expected, "error": error}


def export_parser_context(check):
    if not sys._git[2] or not CPYTHON_COMMIT.startswith(sys._git[2]):
        raise RuntimeError("reference interpreter must be built from the pinned CPython commit")
    timezone_hash = hashlib.sha256(Path(timezone_module.__file__).read_bytes()).hexdigest()
    if timezone_hash != "1149c474c7de4e15e263a978b21f7205a6d9eb7fcf3b3cb4d734ac87db619f5a":
        raise RuntimeError("python-dateutil tzlocal source changed")
    environments = [
        ("UTC", "UTC", "UTC", "UTC", 0, 0),
        ("New_York", "America/New_York", "EST", "EDT", -18000, -14400),
        ("London", "Europe/London", "GMT", "BST", 0, 3600),
        ("Kolkata", "Asia/Kolkata", "IST", "IST", 19800, 19800),
        ("Lord_Howe", "Australia/Lord_Howe", "+1030", "+11", 37800, 39600),
        ("Dublin", "Europe/Dublin", "GMT", "IST", 0, 3600),
        ("Moscow", "Europe/Moscow", "MSK", "MSK", 10800, 10800),
        ("Apia", "Pacific/Apia", "+13", "+13", 46800, 46800),
        ("Santiago", "America/Santiago", "-04", "-03", -14400, -10800),
        ("Windows_Eastern", "America/New_York", "Eastern Standard Time", "Eastern Daylight Time", -18000, -14400),
    ]
    contexts, cases = {}, []
    for name, key, standard_name, daylight_name, standard_offset, daylight_offset in environments:
        contexts[name] = {"zone": key, "names": [standard_name, daylight_name],
                          "standard_offset": standard_offset, "daylight_offset": daylight_offset}
        with importlib.resources.files("tzdata.zoneinfo").joinpath(key).open("rb") as source:
            zone = ZoneInfo.from_file(source, key=key)

        def localtime(timestamp=None):
            if timestamp is None:
                return DEFAULT.timetuple()
            return datetime.fromtimestamp(timestamp, zone).timetuple()

        environment = SimpleNamespace(tzname=(standard_name, daylight_name),
                                      timezone=-standard_offset, altzone=-daylight_offset,
                                      daylight=int(standard_offset != daylight_offset), localtime=localtime)
        clocks = ["1995 June 15 12:30", "2000 January 1 00:30", "2011 December 30 00:30",
                  "2024 March 10 02:30", "2024 November 3 01:30", "2024 March 31 01:30",
                  "2024 October 27 01:30", "2024 October 6 02:15", "2024 April 7 01:45",
                  "2024 September 8 00:30"]
        suffixes = ["", " XYZ", " UTC", " GMT", " Z", " EST", " EDT", " BST", " IST", " MSK",
                    " +0530", " -0400", " EST+05"]
        inputs = [(f"{clock}{suffix}", DEFAULT, 2026) for clock, suffix in product(clocks, suffixes)]
        if name == "UTC":
            defaults = [("2026-01-31T00:00:00", 2026), ("2024-02-29T00:00:00", 2026),
                        ("2026-09-13T12:34:56.123456", 1999), ("2027-01-01T00:00:00", 2026),
                        ("9999-12-31T00:00:00", 2026), ("0001-01-01T00:00:00", 2026)]
            texts = ["2024 February", "2023 February", "2023 February 29", "February", "2024",
                     "01", "75", "76", "77", "Jan 1 49", "Jan 1 50", "Thursday",
                     "2024 February Monday", "2024 February 01 Monday",
                     "2024 March 10 02:30:00.1234567", "2024 March 10 02:30 XYZ", "bad"]
            inputs.extend((text, datetime.fromisoformat(default), year)
                          for (default, year), text in product(defaults, texts))
        with patch.object(_parser, "time", environment), patch.object(timezone_module, "time", environment):
            for (text, default, year), fold in product(inputs, [False, True]):
                parser = _parser.parser()
                parser.info._year = year
                parser.info._century = year // 100 * 100
                cases.append({"context": name, "input": text, "default": default.isoformat(),
                              "default_fold": fold, "parser_year": year,
                              "expected": parse_result(parser, text, default.replace(fold=int(fold)), True)})
    output = {"python": platform.python_version(), "cpython_commit": CPYTHON_COMMIT,
              "dateutil": "2.9.0.post0", "parser_sha256": hashlib.sha256(Path(_parser.__file__).read_bytes()).hexdigest(),
              "tzlocal_sha256": timezone_hash, "tzdata": importlib.metadata.version("tzdata"),
              "environment": "Unmodified dateutil parser and tzlocal with explicit time-module snapshots; localtime uses pinned tzdata ZoneInfo",
              "contexts": contexts, "cases": cases}
    write(ROOT / "parser/context-reference.json", output, check)
    print(f"Validated parser contexts: {len(cases)} cases across {len(contexts)} explicit local environments")


def export_timestamp(check):
    if not sys._git[2] or not CPYTHON_COMMIT.startswith(sys._git[2]):
        raise RuntimeError("reference interpreter must be built from the pinned CPython commit")
    inputs = []
    dates = ["1995-01-01T00:00:00", "2000-01-01T00:00:00", "2024-03-10T01:59:59",
             "2024-03-10T02:00:00", "2024-03-10T02:30:00", "2024-03-10T03:00:00",
             "2024-11-03T00:59:59", "2024-11-03T01:00:00", "2024-11-03T01:30:00",
             "2024-11-03T02:00:00", "2026-09-13T23:59:59"]
    for text, fraction, fold in product(dates, ["", ".000001", ".123456", ".999999"], [False, True]):
        inputs.append({"input": text + fraction, "fold": fold})
    aware_dates = dates + ["0001-01-01T00:00:00", "1969-12-31T23:59:59", "2243-01-01T00:00:00",
                           "3000-01-01T00:00:00", "9999-12-31T23:59:59"]
    for text, fraction, offset in product(aware_dates, ["", ".000001", ".123456", ".999999"],
                                          ["Z", "+05:30", "-04:00", "+00:00:01.000001", "-23:59:59.999999"]):
        inputs.append({"input": text + fraction + offset, "fold": False})
    worker = '''import json, struct, sys
from datetime import datetime
results = []
for case in json.load(sys.stdin):
    value = datetime.fromisoformat(case["input"]).replace(fold=int(case["fold"]))
    try:
        bits = struct.unpack(">Q", struct.pack(">d", value.timestamp()))[0]
        expected = {"bits": bits, "error": ""}
    except (ValueError, OverflowError, OSError) as error:
        expected = {"error": type(error).__name__}
    results.append({**case, "expected": expected})
print(json.dumps(results))
'''
    cases = []
    for name, environment in [("UTC", "UTC"), ("America/New_York", "EST5EDT")]:
        result = subprocess.run([sys.executable, "-c", worker], input=json.dumps(inputs).encode(),
                                stdout=subprocess.PIPE, env={**os.environ, "TZ": environment}, check=True)
        cases.extend({"zone": name, **case} for case in json.loads(result.stdout))
    output = {"python": platform.python_version(), "source_commit": CPYTHON_COMMIT,
              "source_sha256": CPYTHON_SOURCES["Modules/_datetimemodule.c"],
              "scope": "C datetime.timestamp: modern naive local times in UTC/EST5EDT, both folds, and aware full-range exact integer-microsecond division",
              "cases": cases}
    write(ROOT / "compat/datetime/timestamp-reference.json", output, check)
    print(f"Validated CPython timestamps: {len(cases)} exact floating-point results")


def export_iso(parser_cases, check):
    if not sys._git[2] or not CPYTHON_COMMIT.startswith(sys._git[2]):
        raise RuntimeError("reference interpreter must be built from the pinned CPython commit")
    sources = {}
    for path, digest in CPYTHON_SOURCES.items():
        url = f"https://raw.githubusercontent.com/python/cpython/{CPYTHON_COMMIT}/{path}"
        with urlopen(url, timeout=60) as response:
            contents = response.read()
        if hashlib.sha256(contents).hexdigest() != digest:
            raise RuntimeError(f"CPython source changed: {path}")
        sources[path] = contents
    license_path = ROOT / "compat/datetime/LICENSE-CPYTHON.txt"
    if hashlib.sha256(license_path.read_bytes()).hexdigest() != "b0e25a78cffb43f4d92de8b61ccfa1f1f98ecbc22330b54b5251e7b6ba010231":
        raise RuntimeError("CPython license changed")
    inputs = {case["input"] for case in parser_cases}
    tree = ast.parse(sources["Lib/test/datetimetester.py"])
    for function in ast.walk(tree):
        if isinstance(function, ast.FunctionDef) and "fromisoformat" in function.name:
            for node in ast.walk(function):
                if isinstance(node, ast.Constant) and isinstance(node.value, str) and node.value[:4].isdigit():
                    inputs.add(node.value)
    dates = ["0001-01-01", "0000-12-31", "2020-02-29", "2021-02-29", "2020-02-00",
             "2020-00-00", "2020-01-32", "2020-13-01", "2020-12-31", "20211231",
             "2020W01", "2020W017", "2020-W01", "2020-W01-7", "2020-W53-7",
             "2021-W53-1", "9999-12-31", "9999-W52-7"]
    inputs.update(dates)
    inputs.update(f"{date}T{clock}" for date, clock in product(dates, ["00:00", "24:00", "24:01"]))
    clocks = ["", "00", "12", "12:", "12x", "1234", "12:34", "123456", "12:34:56",
              "12:3456", "1234:56", "12.5", "12:34.5", "12:34:56.", "12:34:56.123456789",
              "12:34:56,123", "12345678", "24", "24:00", "24:00:00.0000009",
              "24:00:00.000001", "25:00", "23:60:00", "23:59:60"]
    offsets = ["", "Z", "+00", "+00:00:00.5", "+05:30", "-04:00:01.234567",
               "+23:59:59.999999", "+24:00", "+01:60", "+00:99:99", "+0", "Z\0junk"]
    for date, separator, clock, offset in product(
        ["2020-02-29", "2020-W53-7", "2020W537"], ["T", " ", "\u2028", "\0"], clocks, offsets
    ):
        inputs.add(f"{date}{separator}{clock}{offset}")
    cases, unsupported = [], []
    for text in sorted(inputs):
        if any(0xD800 <= ord(character) <= 0xDFFF for character in text):
            unsupported.append({"codepoints": [ord(character) for character in text],
                                "reason": "lone surrogate is not a Unicode scalar/valid UTF-8 input"})
            continue
        try:
            value = datetime.fromisoformat(text)
            offset = value.utcoffset()
            offset_microseconds = ((offset.days * 86400 + offset.seconds) * 1000000 + offset.microseconds) if offset is not None else 0
            expected = {"year": value.year, "month": value.month, "day": value.day,
                        "hour": value.hour, "minute": value.minute, "second": value.second,
                        "microsecond": value.microsecond, "aware": offset is not None,
                        "offset_microseconds": offset_microseconds, "error": ""}
        except (ValueError, OverflowError, TypeError) as error:
            expected = {"error": type(error).__name__}
        cases.append({"input": text, "expected": expected})
    output = {"python": platform.python_version(), "source_commit": CPYTHON_COMMIT,
              "source_sha256": CPYTHON_SOURCES,
              "input_sources": ["unchanged Dateutil parser corpus", "CPython fromisoformat test literals", "deterministic ISO boundary matrix"],
              "unsupported_inputs": unsupported, "cases": cases}
    write(ROOT / "compat/datetime/reference.json", output, check)
    print(f"Validated CPython ISO reference: {len(cases)} cases; {len(unsupported)} explicitly unrepresentable surrogate inputs")


if __name__ == "__main__":
    main()