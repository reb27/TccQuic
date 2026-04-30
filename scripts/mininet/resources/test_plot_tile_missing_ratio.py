#!/usr/bin/env python3
"""
Unit tests for plot_tile_missing_ratio.py (no Mininet).

Run from repo root:
  python -m unittest discover -s scripts/mininet/resources -p "test_*.py" -v

Or:
  cd scripts/mininet && python -m unittest discover -s resources -p "test_*.py" -v
"""

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

# Headless matplotlib before loading the module under test.
os.environ.setdefault("MPLBACKEND", "Agg")

_HERE = Path(__file__).resolve().parent
_PLOT_PATH = _HERE / "plot_tile_missing_ratio.py"


def _load_plot_module():
    spec = importlib.util.spec_from_file_location("plot_tile_missing_ratio", _PLOT_PATH)
    mod = importlib.util.module_from_spec(spec)
    sys.modules["plot_tile_missing_ratio"] = mod
    spec.loader.exec_module(mod)
    return mod


plotm = _load_plot_module()

STATS_HEADER = (
    "time_ns,segment,tile,priority,latency_ns,timedout,skipped,ok,"
    "tp,buffer_s,tile_missing_ratio,in_fov,on_time,bitrate\n"
)


def _write(path: Path, text: str, encoding: str = "utf-8") -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding=encoding)


def _csv_row(priority: int, ok: bool, skipped: bool = False, bitrate: int = 3) -> str:
    ok_s = "true" if ok else "false"
    sk_s = "true" if skipped else "false"
    return (
        f"0,1,1,{priority},1,false,{sk_s},{ok_s},0.0,0.00,0.00,false,true,{bitrate}\n"
    )


class TestParseHelpers(unittest.TestCase):
    def test_parse_bool(self):
        pb = plotm._parse_bool
        self.assertTrue(pb("true"))
        self.assertTrue(pb("TRUE"))
        self.assertTrue(pb("1"))
        self.assertTrue(pb("yes"))
        self.assertFalse(pb("false"))
        self.assertFalse(pb(""))

    def test_parse_env_strips_bom(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "experiment.env"
            p.write_bytes(b"\xef\xbb\xbfscenario=matrix\nloss_pct=5\n")
            env = plotm._parse_env(str(p))
            self.assertEqual(env.get("scenario"), "matrix")
            self.assertEqual(env.get("loss_pct"), "5")


class TestMissingRatioFromCsv(unittest.TestCase):
    def test_per_priority_missing(self):
        with tempfile.TemporaryDirectory() as td:
            csv_path = Path(td) / "statistics-unit.csv"
            body = (
                _csv_row(0, True)
                + _csv_row(0, False)
                + _csv_row(2, True)
                + _csv_row(2, False)
                + _csv_row(2, False)
            )
            _write(csv_path, STATS_HEADER + body)
            r = plotm._compute_missing_ratio([str(csv_path)])
            self.assertAlmostEqual(r[0], 50.0)
            self.assertAlmostEqual(r[2], 100.0 * 2 / 3)

    def test_skipped_excluded(self):
        with tempfile.TemporaryDirectory() as td:
            csv_path = Path(td) / "statistics-unit.csv"
            body = _csv_row(0, False, skipped=True) + _csv_row(0, True)
            _write(csv_path, STATS_HEADER + body)
            r = plotm._compute_missing_ratio([str(csv_path)])
            self.assertAlmostEqual(r[0], 0.0)

    def test_overall_missing(self):
        with tempfile.TemporaryDirectory() as td:
            csv_path = Path(td) / "statistics-unit.csv"
            body = _csv_row(0, True) + _csv_row(1, False) + _csv_row(2, True)
            _write(csv_path, STATS_HEADER + body)
            overall = plotm._compute_missing_overall([str(csv_path)])
            self.assertAlmostEqual(overall, 100.0 / 3)


class TestCsvList(unittest.TestCase):
    def test_filters_summary(self):
        with tempfile.TemporaryDirectory() as td:
            d = Path(td)
            _write(d / "statistics-a.csv", "h\n")
            _write(d / "statistics-summary.csv", "h\n")
            _write(d / "other.csv", "h\n")
            lst = plotm._csv_list(str(d))
            self.assertEqual(len(lst), 1)
            self.assertIn("statistics-a.csv", lst[0])


class TestCollectMatrix(unittest.TestCase):
    def _matrix_env(
        self, bg: int, loss: float, sched: str, abr: str
    ) -> str:
        return (
            f"scenario=matrix\n"
            f"background_load_pct={bg}\n"
            f"server_mode={sched}\n"
            f"abr_mode={abr}\n"
            f"loss_pct={loss}\n"
        )

    def test_detect_background_levels(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            e1 = root / "a" / "experiment.env"
            e2 = root / "b" / "experiment.env"
            _write(e1, self._matrix_env(10, 0, "fifo", "bola"))
            _write(e2, self._matrix_env(25, 0, "fifo", "bola"))
            for exp_dir in (e1.parent, e2.parent):
                _write(
                    exp_dir / "statistics-x.csv",
                    STATS_HEADER
                    + _csv_row(0, True),
                )
            levels = plotm.detect_matrix_background_levels(str(root))
            self.assertEqual(levels, [10, 25])

    def test_collect_matrix_fifo_overall_and_sp_high_low(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            fifo_dir = root / "bg10" / "l0" / "fifo" / "bola"
            sp_dir = root / "bg10" / "l0" / "sp" / "bola"
            _write(fifo_dir / "experiment.env", self._matrix_env(10, 0, "fifo", "bola"))
            _write(sp_dir / "experiment.env", self._matrix_env(10, 0, "sp", "bola"))
            _write(fifo_dir / "statistics-f.csv", STATS_HEADER + _csv_row(0, True) + _csv_row(2, False))
            _write(
                sp_dir / "statistics-s.csv",
                STATS_HEADER
                + _csv_row(0, True)
                + _csv_row(0, False)
                + _csv_row(1, True)
                + _csv_row(1, False)
                + _csv_row(2, True)
                + _csv_row(2, False),
            )
            data = plotm.collect_matrix_data(str(root), 10)
            self.assertAlmostEqual(data[("bola", "fifo", "overall", 0.0)], 50.0)
            self.assertAlmostEqual(data[("bola", "sp", "high", 0.0)], 50.0)
            self.assertAlmostEqual(data[("bola", "sp", "medium", 0.0)], 50.0)
            self.assertAlmostEqual(data[("bola", "sp", "low", 0.0)], 50.0)

    def test_collect_matrix_ignores_wrong_bg(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            d = root / "run"
            _write(d / "experiment.env", self._matrix_env(25, 0, "fifo", "bola"))
            _write(d / "statistics-f.csv", STATS_HEADER + _csv_row(0, False))
            data = plotm.collect_matrix_data(str(root), 10)
            self.assertEqual(data, {})


class TestCollectLegacy(unittest.TestCase):
    def test_only_sp_wfq(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            sp_dir = root / "sp" / "bola"
            fifo_dir = root / "fifo" / "bola"
            row = _csv_row(0, False)
            _write(
                sp_dir / "experiment.env",
                "server_mode=sp\nabr_mode=bola\nloss_pct=5\n",
            )
            _write(sp_dir / "statistics-1.csv", STATS_HEADER + row)
            _write(
                fifo_dir / "experiment.env",
                "server_mode=fifo\nabr_mode=bola\nloss_pct=5\n",
            )
            _write(fifo_dir / "statistics-1.csv", STATS_HEADER + row)
            data = plotm.collect_data(str(root))
            self.assertIn(("bola", "sp", "high", 5.0), data)
            self.assertEqual(len(data), 1)


class TestPlotWritesPng(unittest.TestCase):
    def test_plot_matrix_writes_file(self):
        data = {
            ("bola", "fifo", "overall", 0.0): 10.0,
            ("bola", "fifo", "overall", 5.0): 20.0,
            ("bola", "sp", "high", 0.0): 5.0,
            ("bola", "sp", "high", 5.0): 15.0,
            ("bola", "sp", "medium", 0.0): 12.0,
            ("bola", "sp", "medium", 5.0): 22.0,
            ("bola", "sp", "low", 0.0): 30.0,
            ("bola", "sp", "low", 5.0): 40.0,
            ("bola", "wfq", "high", 0.0): 6.0,
            ("bola", "wfq", "high", 5.0): 16.0,
            ("bola", "wfq", "medium", 0.0): 14.0,
            ("bola", "wfq", "medium", 5.0): 24.0,
            ("bola", "wfq", "low", 0.0): 32.0,
            ("bola", "wfq", "low", 5.0): 42.0,
            ("legacy", "fifo", "overall", 0.0): 12.0,
            ("legacy", "fifo", "overall", 5.0): 22.0,
            ("legacy", "sp", "high", 0.0): 8.0,
            ("legacy", "sp", "high", 5.0): 18.0,
            ("legacy", "sp", "medium", 0.0): 13.0,
            ("legacy", "sp", "medium", 5.0): 23.0,
            ("legacy", "sp", "low", 0.0): 28.0,
            ("legacy", "sp", "low", 5.0): 38.0,
            ("legacy", "wfq", "high", 0.0): 9.0,
            ("legacy", "wfq", "high", 5.0): 19.0,
            ("legacy", "wfq", "medium", 0.0): 15.0,
            ("legacy", "wfq", "medium", 5.0): 25.0,
            ("legacy", "wfq", "low", 0.0): 29.0,
            ("legacy", "wfq", "low", 5.0): 39.0,
        }
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out.png"
            with patch("builtins.print"):
                plotm.plot_matrix(data, str(out), "10%")
            self.assertTrue(out.is_file())
            self.assertGreater(out.stat().st_size, 500)

    def test_plot_legacy_writes_file(self):
        data = {
            ("bola", "sp", "high", 0.0): 1.0,
            ("bola", "sp", "high", 5.0): 2.0,
            ("bola", "sp", "medium", 0.0): 3.0,
            ("bola", "sp", "medium", 5.0): 4.0,
            ("bola", "sp", "low", 0.0): 5.0,
            ("bola", "sp", "low", 5.0): 6.0,
            ("legacy", "wfq", "high", 0.0): 7.0,
            ("legacy", "wfq", "high", 5.0): 8.0,
            ("legacy", "wfq", "medium", 0.0): 9.0,
            ("legacy", "wfq", "medium", 5.0): 10.0,
            ("legacy", "wfq", "low", 0.0): 11.0,
            ("legacy", "wfq", "low", 5.0): 12.0,
        }
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "legacy.png"
            with patch("builtins.print"):
                plotm.plot_legacy(data, str(out))
            self.assertTrue(out.is_file())
            self.assertGreater(out.stat().st_size, 500)


class TestSyntheticMatrix(unittest.TestCase):
    def test_keys_cover_schedulers(self):
        d = plotm._generate_synthetic_matrix(10)
        losses = {k[3] for k in d}
        self.assertGreater(len(losses), 10)
        for abr in ("bola", "legacy"):
            for sched in ("fifo", "sp", "wfq"):
                sample = [k for k in d if k[0] == abr and k[1] == sched]
                self.assertTrue(sample, f"missing series {abr}/{sched}")
            for sched in ("sp", "wfq"):
                for prio in ("high", "medium", "low"):
                    sub = [k for k in d if k[0] == abr and k[1] == sched and k[2] == prio]
                    self.assertTrue(sub, f"missing {abr}/{sched}/{prio}")


if __name__ == "__main__":
    unittest.main()
