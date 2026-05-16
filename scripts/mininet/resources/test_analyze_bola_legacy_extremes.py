#!/usr/bin/env python3
"""
Unit tests for analyze_bola_legacy_extremes.py (no Mininet).
"""

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path

os.environ.setdefault("MPLBACKEND", "Agg")

_HERE = Path(__file__).resolve().parent
_MOD_PATH = _HERE / "analyze_bola_legacy_extremes.py"


def _load_module():
    spec = importlib.util.spec_from_file_location("analyze_bola_legacy_extremes", _MOD_PATH)
    mod = importlib.util.module_from_spec(spec)
    sys.modules["analyze_bola_legacy_extremes"] = mod
    spec.loader.exec_module(mod)
    return mod


am = _load_module()

STATS_HEADER = (
    "time_ns,segment,tile,priority,latency_ns,timedout,skipped,ok,"
    "tp,buffer_s,tile_missing_ratio,in_fov,on_time,bitrate\n"
)


def _write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


class TestAnalyzeBolaLegacyExtremes(unittest.TestCase):
    def test_collect_and_summary(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            run = root / "good" / "bola"
            _write(
                run / "experiment.env",
                "scenario_family=bola_legacy_extremes\ncondition_id=good\nabr_mode=bola\nfov_mode=normal\n",
            )
            _write(
                run / "statistics-1.csv",
                STATS_HEADER
                + "1,1,1,0,1,false,false,true,10,1,0,true,true,3\n"
                + "2,1,2,1,1,false,false,true,30,1,0,false,true,5\n"
                + "3,1,3,2,1,false,false,false,30,1,0,false,false,10\n"
                + "4,1,4,1,1,false,true,true,50,1,0,true,true,5\n",
            )
            data = am.collect(tmp)
            self.assertIn("good", data)
            self.assertIn("bola", data["good"])
            m = data["good"]["bola"]
            self.assertEqual(m["rows"], 3)
            self.assertEqual(m["ok"], 2)
            self.assertEqual(m["on_time"], 2)
            self.assertEqual(m["bitrate_counts"][3], 1)
            self.assertEqual(m["bitrate_counts"][10], 1)
            self.assertEqual(m["bitrate_counts_spatial"]["fov"][3], 1)
            self.assertEqual(m["bitrate_counts_spatial"]["near_fov"][5], 1)
            self.assertEqual(m["bitrate_counts_spatial"]["outside_fov"][10], 1)

            out_csv = root / "summary.csv"
            rows = am.write_summary_csv(data, str(out_csv))
            self.assertEqual(len(rows), 1)
            self.assertAlmostEqual(rows[0]["tile_missing_pct"], 100.0 / 3.0)
            self.assertEqual(rows[0]["zone_fov_rows"], 1)
            self.assertEqual(rows[0]["zone_near_fov_rows"], 1)
            self.assertEqual(rows[0]["zone_outside_fov_rows"], 1)
            r0 = rows[0]
            low_mix = (
                r0["mix_low_fov_of_total_pct"]
                + r0["mix_low_near_fov_of_total_pct"]
                + r0["mix_low_outside_fov_of_total_pct"]
            )
            self.assertAlmostEqual(low_mix, r0["bitrate_low_pct"])
            self.assertAlmostEqual(r0["mix_low_fov_of_total_pct"], 100.0 / 3.0)
            self.assertEqual(rows[0]["fov_mode"], "normal")
            self.assertEqual(rows[0]["low_req_total"], 1)
            self.assertEqual(rows[0]["low_req_fov"], 1)
            self.assertEqual(rows[0]["low_req_near_fov"], 0)
            self.assertEqual(rows[0]["total_req_nonfov_rows"], 2)
            self.assertEqual(rows[0]["low_req_nonfov"], 0)
            self.assertEqual(r0["delivered_ok_fov_low"], 1)
            self.assertEqual(r0["delivered_ok_near_fov_med"], 1)
            self.assertEqual(r0["delivered_ok_fov_med"], 0)
            self.assertEqual(r0["delivered_ok_fov_high"], 0)
            self.assertEqual(
                r0["delivered_ok_fov"]
                + r0["delivered_ok_near_fov"]
                + r0["delivered_ok_outside_fov"],
                r0["ok_rows"],
            )
            self.assertTrue(out_csv.exists())


if __name__ == "__main__":
    unittest.main()
