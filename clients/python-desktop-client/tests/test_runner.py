from __future__ import annotations

import unittest

from agent_server_desktop_client.runner import ScenarioResult, ValidationReport, _build_parser


class RunnerTests(unittest.TestCase):
    def test_parser_defaults_to_full(self) -> None:
        parser = _build_parser()
        args = parser.parse_args([])
        self.assertEqual(args.scenario, "full")
        self.assertEqual(args.http_base, "http://127.0.0.1:8080")

    def test_validation_report_ok_tracks_all_scenarios(self) -> None:
        report = ValidationReport(
            http_base="http://127.0.0.1:8080",
            protocol_version="rtos-ws-v0",
            ws_path="/v1/realtime/ws",
            subprotocol="agent-server.realtime.v0",
            scenarios=[
                ScenarioResult(name="text", session_id="sess_a", ok=True),
                ScenarioResult(name="audio", session_id="sess_b", ok=False),
            ],
        )
        self.assertFalse(report.ok)


if __name__ == "__main__":
    unittest.main()
