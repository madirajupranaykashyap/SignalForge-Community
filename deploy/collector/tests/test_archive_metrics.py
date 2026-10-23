import importlib.util
import os
import tempfile
import unittest
from pathlib import Path

MODULE_PATH = Path(__file__).parents[1] / "metrics" / "archive_metrics.py"
spec = importlib.util.spec_from_file_location("archive_metrics", MODULE_PATH)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ArchiveMetricsTests(unittest.TestCase):
    def test_reports_bytes_presence_and_incremental_line_count(self):
        with tempfile.TemporaryDirectory() as directory:
            module.ARCHIVE_DIR = Path(directory)
            module.STATE = module.ArchiveState()
            raw = Path(directory) / "logs.jsonl"
            raw.write_bytes(b'{"body":"one"}\n{"body":"two"}\n')

            first = module.STATE.sample()
            self.assertIn('signalforge_archive_file_bytes{stream="raw"} 30', first)
            self.assertIn('signalforge_archive_file_present{stream="raw"} 1', first)
            self.assertIn('signalforge_archive_records_total{stream="raw"} 2', first)
            self.assertIn('signalforge_archive_file_present{stream="optimized"} 0', first)

            with raw.open("ab") as handle:
                handle.write(b'{"body":"three"}\n')
            second = module.STATE.sample()
            self.assertIn('signalforge_archive_records_total{stream="raw"} 3', second)

    def test_detects_file_replacement(self):
        with tempfile.TemporaryDirectory() as directory:
            module.ARCHIVE_DIR = Path(directory)
            module.STATE = module.ArchiveState()
            raw = Path(directory) / "logs.jsonl"
            raw.write_bytes(b'old\n')
            module.STATE.sample()
            replacement = Path(directory) / "replacement.jsonl"
            replacement.write_bytes(b'new\n')
            os.replace(replacement, raw)
            metrics = module.STATE.sample()
            self.assertIn('signalforge_archive_records_total{stream="raw"} 1', metrics)


if __name__ == "__main__":
    unittest.main()
