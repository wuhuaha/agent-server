from __future__ import annotations

from argparse import Namespace
from types import SimpleNamespace
import unittest
from unittest import mock

from agent_server_desktop_client.runner import (
    RealtimeScenarioRunner,
    ScenarioMetrics,
    ScenarioResult,
    ValidationReport,
    _run_from_args,
    _build_parser,
    build_quality_summary,
)


class _AsyncConnect:
    def __init__(self, ws: object) -> None:
        self._ws = ws

    async def __aenter__(self) -> object:
        return self._ws

    async def __aexit__(self, exc_type, exc, tb) -> bool:
        return False


class RunnerTests(unittest.TestCase):
    def test_parser_defaults_to_full(self) -> None:
        parser = _build_parser()
        args = parser.parse_args([])
        self.assertEqual(args.scenario, "full")
        self.assertEqual(args.http_base, "http://127.0.0.1:8080")

    def test_parser_accepts_regression_scenarios(self) -> None:
        parser = _build_parser()
        for scenario in ("tool", "barge-in", "timeout", "server-endpoint-preview", "regression"):
            args = parser.parse_args(["--scenario", scenario])
            self.assertEqual(args.scenario, scenario)

    def test_validation_report_ok_tracks_all_scenarios(self) -> None:
        report = ValidationReport(
            generated_at="2026-04-09T00:00:00Z",
            run_id="run_123456789abc",
            http_base="http://127.0.0.1:8080",
            protocol_version="rtos-ws-v0",
            ws_path="/v1/realtime/ws",
            subprotocol="agent-server.realtime.v0",
            turn_mode="client_wakeup_client_commit",
            llm_provider="deepseek_chat",
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
                        thinking_latency_ms=90,
                        active_return_latency_ms=260,
                        response_start_latency_ms=120,
                        first_partial_latency_ms=130,
                        first_text_latency_ms=150,
                        response_complete_latency_ms=260,
                        playout_complete_latency_ms=260,
                        response_text_chars=8,
                        partial_response_count=1,
                        partial_response_chars=2,
                        heard_text_chars=2,
                        audio_chunk_count=0,
                    ),
                ),
                ScenarioResult(
                    name="audio",
                    session_id="sess_b",
                    ok=True,
                    response_texts=["spoken"],
                    metrics=ScenarioMetrics(
                        thinking_latency_ms=180,
                        speaking_latency_ms=260,
                        active_return_latency_ms=480,
                        response_start_latency_ms=220,
                        first_text_latency_ms=240,
                        first_audio_latency_ms=320,
                        barge_in_cutoff_latency_ms=110,
                        response_complete_latency_ms=480,
                        playout_complete_latency_ms=480,
                        response_text_chars=24,
                        heard_text_chars=10,
                        audio_chunk_count=4,
                        received_audio_bytes=1280,
                    ),
                ),
            ]
        )
        self.assertEqual(summary.scenario_count, 2)
        self.assertEqual(summary.ok_scenarios, 2)
        self.assertEqual(summary.thinking_latency_ms_avg, 135.0)
        self.assertEqual(summary.speaking_latency_ms_avg, 260.0)
        self.assertEqual(summary.active_return_latency_ms_avg, 370.0)
        self.assertEqual(summary.response_start_latency_ms_avg, 170.0)
        self.assertEqual(summary.first_partial_latency_ms_avg, 130.0)
        self.assertEqual(summary.first_text_latency_ms_avg, 195.0)
        self.assertEqual(summary.first_audio_latency_ms_avg, 320.0)
        self.assertEqual(summary.barge_in_cutoff_latency_ms_avg, 110.0)
        self.assertEqual(summary.response_complete_latency_ms_avg, 370.0)
        self.assertEqual(summary.playout_complete_latency_ms_avg, 370.0)
        self.assertEqual(summary.issue_scenario_count, 0)
        self.assertEqual(summary.received_audio_bytes_total, 1280)
        self.assertEqual(summary.response_text_chars_total, 32)
        self.assertEqual(summary.partial_response_ratio, 0.5)
        self.assertEqual(summary.heard_text_chars_total, 12)
        self.assertEqual(summary.response_with_audio_ratio, 0.5)
        self.assertEqual(summary.non_empty_response_ratio, 1.0)


class RunnerDispatchTests(unittest.IsolatedAsyncioTestCase):
    async def test_text_scenario_accepts_semantic_reply_without_echo(self) -> None:
        runner = RealtimeScenarioRunner.__new__(RealtimeScenarioRunner)
        runner._connect = mock.Mock(return_value=_AsyncConnect(object()))
        runner._start_session = mock.AsyncMock()
        runner._send_text = mock.AsyncMock()
        runner._end_session = mock.AsyncMock()
        runner._recv_json = mock.AsyncMock(return_value={"type": "session.end"})
        runner._write_scenario_artifacts = mock.Mock()

        async def populate_result(ws, result, **kwargs) -> None:
            result.events.append({"type": "response.start"})
            result.response_texts.extend(["明天", "是", "星期四", "。"])

        runner._collect_until_active_or_end = mock.AsyncMock(side_effect=populate_result)

        result = await RealtimeScenarioRunner.run_text_scenario(runner, "明天周几")

        self.assertTrue(result.ok)
        self.assertIn("received non-empty response text or audio", result.checks)
        self.assertNotIn("response includes echoed text input", result.issues)

    async def test_server_end_scenario_accepts_semantic_close_reply(self) -> None:
        runner = RealtimeScenarioRunner.__new__(RealtimeScenarioRunner)
        runner._connect = mock.Mock(return_value=_AsyncConnect(object()))
        runner._start_session = mock.AsyncMock()
        runner._send_text = mock.AsyncMock()
        runner._write_scenario_artifacts = mock.Mock()

        async def populate_result(ws, result, **kwargs) -> None:
            result.events.extend(
                [
                    {"type": "response.start"},
                    {"type": "session.end", "payload": {"reason": "completed"}},
                ]
            )
            result.response_texts.extend(["已", "结束", "当前", "会话", "。"])

        runner._collect_until_active_or_end = mock.AsyncMock(side_effect=populate_result)

        result = await RealtimeScenarioRunner.run_server_end_scenario(runner)

        self.assertTrue(result.ok)
        self.assertIn("received non-empty response text or audio before close", result.checks)
        self.assertNotIn("response includes /end trigger echo", result.issues)

    async def test_run_full_and_regression_report_server_endpoint_metadata(self) -> None:
        runner = RealtimeScenarioRunner.__new__(RealtimeScenarioRunner)
        runner.generated_at = "2026-04-15T00:00:00Z"
        runner.run_id = "run_report"
        runner.http_base = "http://127.0.0.1:8080"
        runner.discovery = SimpleNamespace(
            protocol_version="rtos-ws-v0",
            ws_path="/v1/realtime/ws",
            subprotocol="agent-server.realtime.v0",
            turn_mode="client_wakeup_client_commit",
            llm_provider="deepseek_chat",
            voice_provider="funasr_http",
            tts_provider="cosyvoice_http",
            server_endpoint_mode="server_vad_assisted",
            server_endpoint_enabled=True,
            server_endpoint_main_path_candidate=True,
        )
        runner._artifact_root = mock.Mock(return_value=None)
        runner.run_text_scenario = mock.AsyncMock(return_value=ScenarioResult(name="text", session_id="sess_text", ok=True))
        runner.run_audio_scenario = mock.AsyncMock(return_value=ScenarioResult(name="audio", session_id="sess_audio", ok=True))
        runner.run_server_end_scenario = mock.AsyncMock(
            return_value=ScenarioResult(name="server-end", session_id="sess_end", ok=True)
        )
        runner.run_tool_scenario = mock.AsyncMock(return_value=ScenarioResult(name="tool", session_id="sess_tool", ok=True))
        runner.run_barge_in_scenario = mock.AsyncMock(
            return_value=ScenarioResult(name="barge-in", session_id="sess_barge", ok=True)
        )
        runner.run_timeout_scenario = mock.AsyncMock(
            return_value=ScenarioResult(name="timeout", session_id="sess_timeout", ok=True)
        )

        full_report = await RealtimeScenarioRunner.run_full(runner, "hello", 1000, 20, wav_path=None, save_rx_dir=None)
        regression_report = await RealtimeScenarioRunner.run_regression(
            runner,
            "hello",
            1000,
            20,
            wav_path=None,
            save_rx_dir=None,
        )

        self.assertEqual(full_report.server_endpoint_mode, "server_vad_assisted")
        self.assertTrue(full_report.server_endpoint_enabled)
        self.assertTrue(full_report.server_endpoint_main_path_candidate)
        self.assertEqual(regression_report.server_endpoint_mode, "server_vad_assisted")
        self.assertTrue(regression_report.server_endpoint_enabled)
        self.assertTrue(regression_report.server_endpoint_main_path_candidate)

    async def test_run_from_args_dispatches_server_endpoint_preview(self) -> None:
        scenario = ScenarioResult(name="server-endpoint-preview", session_id="sess_preview", ok=True)
        fake_runner = mock.MagicMock()
        fake_runner.generated_at = "2026-04-11T00:00:00Z"
        fake_runner.run_id = "run_preview"
        fake_runner.http_base = "http://127.0.0.1:8080"
        fake_runner.discovery = SimpleNamespace(
            protocol_version="rtos-ws-v0",
            ws_path="/v1/realtime/ws",
            subprotocol="agent-server.realtime.v0",
            turn_mode="client_wakeup_client_commit",
            llm_provider="bootstrap",
            voice_provider="funasr_http",
            tts_provider="none",
            server_endpoint_mode="server_vad_assisted",
            server_endpoint_enabled=True,
            server_endpoint_main_path_candidate=True,
        )
        fake_runner._artifact_root.return_value = None
        fake_runner.run_server_endpoint_preview_scenario = mock.AsyncMock(return_value=scenario)

        with mock.patch("agent_server_desktop_client.runner.RealtimeScenarioRunner", return_value=fake_runner):
            report = await _run_from_args(
                Namespace(
                    http_base="http://127.0.0.1:8080",
                    scenario="server-endpoint-preview",
                    text="hello",
                    silence_ms=1200,
                    frame_ms=40,
                    wav_path="sample.wav",
                    device_id="desktop-script-001",
                    client_type="desktop-script",
                    firmware_version="script-runner-0.1.0",
                    timeout_sec=30.0,
                    output=None,
                    save_rx_dir=None,
                )
            )

        fake_runner.run_server_endpoint_preview_scenario.assert_awaited_once_with(
            1200,
            40,
            wav_path="sample.wav",
            artifact_root=None,
        )
        self.assertEqual(report.scenarios[0].name, "server-endpoint-preview")
        self.assertEqual(report.voice_provider, "funasr_http")
        self.assertEqual(report.server_endpoint_mode, "server_vad_assisted")
        self.assertTrue(report.server_endpoint_enabled)


if __name__ == "__main__":
    unittest.main()
