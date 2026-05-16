#!/usr/bin/env python3
"""
Unit tests for plot_bola_bitrate_scenarios.py (no Mininet).

Mesmo padrão que test_plot_tile_missing_ratio.py — descoberta via:
  python -m unittest discover -s scripts/mininet/resources -p "test_*.py" -v
"""

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path

os.environ.setdefault("MPLBACKEND", "Agg")

_HERE = Path(__file__).resolve().parent
_PLOT_PATH = _HERE / "plot_bola_bitrate_scenarios.py"


def _load_plot_module():
    spec = importlib.util.spec_from_file_location("plot_bola_bitrate_scenarios", _PLOT_PATH)
    mod = importlib.util.module_from_spec(spec)
    sys.modules["plot_bola_bitrate_scenarios"] = mod
    spec.loader.exec_module(mod)
    return mod


pm = _load_plot_module()

STATS_HEADER = (
    "time_ns,segment,tile,priority,latency_ns,timedout,skipped,ok,"
    "tp,buffer_s,tile_missing_ratio,in_fov,on_time,bitrate\n"
)


def _write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


class TestPlotBolaBitrateScenarios(unittest.TestCase):
    def test_discover_runs_empty_tree(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            self.assertEqual(pm._discover_runs(tmp), [])

    def test_discover_runs_one_scenario(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            run = root / "cbwA"
            run.mkdir()
            _write(
                run / "experiment.env",
                "scenario=cbwA\nplot_label=Test A\nclient_bw_mbps=30\n",
            )
            _write(
                run / "statistics-1.csv",
                STATS_HEADER
                + "1,1,1,1,1,false,false,true,1,1,0,true,true,3\n"
                + "2,1,2,1,1,false,false,true,1,1,0,true,true,10\n",
            )
            rows = pm._discover_runs(tmp)
            self.assertEqual(len(rows), 1)
            _dirpath, dirname, env, counts, total = rows[0]
            self.assertEqual(dirname, "cbwA")
            self.assertEqual(total, 2)
            self.assertEqual(counts[3], 1)
            self.assertEqual(counts[10], 1)

    def test_scenario_label_precedence(self) -> None:
        self.assertEqual(
            pm._scenario_label({"plot_label": "X"}, "ignored"),
            "X",
        )
        self.assertEqual(
            pm._scenario_label({"client_bw_mbps": "80"}, "d"),
            "Cliente 80 Mbps",
        )
        self.assertEqual(
            pm._scenario_label({}, "dirname"),
            "dirname",
        )


if __name__ == "__main__":
    unittest.main()
