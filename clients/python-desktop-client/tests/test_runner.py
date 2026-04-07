from __future__ import annotations

import unittest

from agent_server_desktop_client.runner import (
    ScenarioMetrics,
    ScenarioResult,
    ValidationReport,
    _build_parser,
    build_quality_summary,
)


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
            turn_mode="client_wakeup_client_commit",
            voice_provider="funasr_http",
            tts_provider="mimo_v2_tts",
            scenarios=[
                ScenarioResult(name="text", session_id="sess_a", ok=True),
                ScenarioResult(name="audio", session_id="sess_b", ok=False),
            ],
        )
        self.assertFalse(report.ok)

    def test_quality_summary_aggregates_latency_and_audio_presence(self) -> None:
        summary = build_quality_summary(
            [
                ScenarioResult(
                    name="text",
                    session_id="sess_a",
                    ok=True,
                    response_texts=["ok"],
                    metrics=ScenarioMetrics(
                        response_start_latency_ms=120,
                        first_text_latency_ms=150,
                        response_complete_latency_ms=260,
                        audio_chunk_count=0,
                    ),
                ),
                ScenarioResult(
                    name="audio",
                    session_id="sess_b",
                    ok=True,
                    response_texts=["spoken"],
                    metrics=ScenarioMetrics(
                        response_start_latency_ms=220,
                        first_text_latency_ms=240,
                        first_audio_latency_ms=320,
                        response_complete_latency_ms=480,
                        audio_chunk_count=4,
                        received_audio_bytes=1280,
                    ),
                ),
            ]
        )
        self.assertEqual(summary.scenario_count, 2)
        self.assertEqual(summary.ok_scenarios, 2)
        self.assertEqual(summary.response_start_latency_ms_avg, 170.0)
        self.assertEqual(summary.first_text_latency_ms_avg, 195.0)
        self.assertEqual(summary.first_audio_latency_ms_avg, 320.0)
        self.assertEqual(summary.response_complete_latency_ms_avg, 370.0)
        self.assertEqual(summary.response_with_audio_ratio, 0.5)
        self.assertEqual(summary.non_empty_response_ratio, 1.0)


if __name__ == "__main__":
    unittest.main()
