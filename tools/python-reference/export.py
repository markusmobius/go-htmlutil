"""Regenerate only the python-dateutil functionality used by HtmlDate/DateParser."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.metadata
import json
from pathlib import Path
import platform
import subprocess
import unicodedata
import warnings
from zoneinfo import ZoneInfo

from dateutil.parser import _parser
import dateutil.relativedelta as relative_module
from dateutil.relativedelta import relativedelta


ROOT = Path(__file__).resolve().parents[2]
DEFAULT = datetime(2026, 9, 13)


def write(path, value, check):
    expected = (json.dumps(value, ensure_ascii=True, indent=2) + "\n").encode()
    if check:
        if path.read_bytes() != expected:
            raise RuntimeError(f"reference changed: {path.relative_to(ROOT)}")
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(expected)


def parse_result(parser, text):
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("ignore")
            value = parser.parse(text, default=DEFAULT, fuzzy=False)
        offset = value.utcoffset()
        return {"year": value.year, "month": value.month, "day": value.day,
                "hour": value.hour, "minute": value.minute, "second": value.second,
                "microsecond": value.microsecond, "aware": offset is not None,
                "offset": int(offset.total_seconds()) if offset is not None else 0, "error": ""}
    except (ValueError, OverflowError, TypeError) as error:
        return {"error": type(error).__name__}


def main():
    arguments = argparse.ArgumentParser(description=__doc__)
    arguments.add_argument("--check", action="store_true")
    arguments.add_argument("--gofmt", default="gofmt")
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


def relative_case(base, delta, zone=""):
    try:
        expected = (base + relativedelta(**delta)).isoformat()
        error = ""
    except (ValueError, OverflowError, TypeError) as exception:
        expected, error = "", type(exception).__name__
    return {"base": base.isoformat(), "zone": zone, "delta": delta, "expected": expected, "error": error}


if __name__ == "__main__":
    main()