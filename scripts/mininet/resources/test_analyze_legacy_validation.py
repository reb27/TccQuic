#!/usr/bin/env python3

import importlib.util
import copy
import os
import unittest

MODULE_PATH = os.path.join(os.path.dirname(__file__), "analyze_legacy_validation.py")
SPEC = importlib.util.spec_from_file_location("legacy_analysis", MODULE_PATH)
legacy_analysis = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(legacy_analysis)


class LegacyAnalysisTest(unittest.TestCase):
    def test_skipped_tiles_remain_in_delivery_denominators(self):
        decisions = [{
            "segment": "1", "tier": "LOW", "buffer_s": "0",
            "avg_throughput_bps": "0", "threshold_med_bps": "10",
            "threshold_high_bps": "20",
        }]
        rows = [
            {"segment": "1", "tile": "1", "request_order": "1", "in_fov": "true", "priority": "0", "bitrate": "3", "ok": "true", "on_time": "true", "skipped": "false"},
            {"segment": "1", "tile": "2", "request_order": "2", "in_fov": "false", "priority": "1", "bitrate": "3", "ok": "true", "on_time": "false", "skipped": "false"},
            {"segment": "1", "tile": "3", "request_order": "3", "in_fov": "false", "priority": "2", "bitrate": "3", "ok": "false", "on_time": "false", "skipped": "true"},
        ]
        data = {condition: {"rows": rows, "decisions": decisions, "env": {"segment_limit": "1"}} for condition in legacy_analysis.CONDITIONS}

        summary, per_segment = legacy_analysis.summarize(data)

        for item in summary:
            self.assertEqual(3, item["planned_tiles"])
            self.assertEqual(1, item["skipped_tiles"])
            self.assertAlmostEqual(200 / 3, item["tile_completion_pct"])
            self.assertEqual(0.0, item["segment_completion_pct"])
            self.assertAlmostEqual(100 / 3, item["tile_on_time_pct"])
            self.assertAlmostEqual(200 / 3, item["tile_deadline_miss_pct"])
            self.assertEqual(100.0, item["priority_correct_pct"])
        self.assertTrue(all(row["planned_tiles"] == 3 for row in per_segment))

    def test_validate_accepts_all_three_medium_tiers_and_every_rule_boundary(self):
        data = self.valid_data()
        summary, _ = legacy_analysis.summarize(data)

        self.assertEqual([], legacy_analysis.validate(data, summary))

    def test_validate_rejects_any_decision_that_violates_threshold_rule(self):
        data = self.valid_data()
        data["good"]["decisions"][1]["tier"] = "MED"
        summary, _ = legacy_analysis.summarize(data)

        errors = legacy_analysis.validate(data, summary)
        self.assertTrue(any("violates rule" in error for error in errors), errors)

    def test_validate_rejects_unreachable_buffer_and_incomplete_medium_tiers(self):
        data = self.valid_data()
        data["medium"]["decisions"][4]["buffer_s"] = "2.1"
        for row in data["medium"]["decisions"]:
            if row["tier"] == "MED":
                row.update(self.decision(row["segment"], "LOW"))
        summary, _ = legacy_analysis.summarize(data)

        errors = legacy_analysis.validate(data, summary)
        self.assertTrue(any("buffer outside contiguous 0..2 s" in error for error in errors), errors)
        self.assertTrue(any("must contain LOW, MED and HIGH" in error for error in errors), errors)

    @staticmethod
    def decision(segment, tier):
        values = {
            "LOW": ("0", "0", "3", "3"),
            "MED": ("1", "10", "5", "3"),
            "HIGH": ("2", "20", "10", "5"),
        }
        buffer_s, throughput, fov_bitrate, near_bitrate = values[tier]
        return {
            "segment": str(segment), "tier": tier, "buffer_s": buffer_s,
            "avg_throughput_bps": throughput, "threshold_med_bps": "10",
            "threshold_high_bps": "20", "fov_bitrate": fov_bitrate,
            "near_fov_bitrate": near_bitrate, "background_bitrate": "3",
        }

    @classmethod
    def valid_data(cls):
        tiers = {
            "good": ["LOW"] + ["HIGH"] * 7,
            "medium": ["LOW", "MED", "MED", "MED", "HIGH", "HIGH", "HIGH", "HIGH"],
            "bad": ["LOW"] * 8,
        }
        data = {}
        for condition, condition_tiers in tiers.items():
            decisions = [cls.decision(i, tier) for i, tier in enumerate(condition_tiers, 1)]
            rows = [{
                "segment": str(i), "tile": "1", "request_order": "1",
                "in_fov": "true", "priority": "0", "bitrate": "3",
                "ok": "true", "on_time": "true", "skipped": "false",
            } for i in range(1, 9)]
            data[condition] = {"rows": rows, "decisions": decisions, "env": {"segment_limit": "8"}}
        return copy.deepcopy(data)


if __name__ == "__main__":
    unittest.main()
