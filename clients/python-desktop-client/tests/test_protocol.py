from __future__ import annotations

import unittest

from agent_server_desktop_client.protocol import (
    build_event,
    http_base_to_ws_base,
    join_ws_url,
    normalize_raw_event,
)


class ProtocolTests(unittest.TestCase):
    def test_http_base_to_ws_base(self) -> None:
        self.assertEqual(http_base_to_ws_base("http://127.0.0.1:8080"), "ws://127.0.0.1:8080")
        self.assertEqual(http_base_to_ws_base("https://example.com/base"), "wss://example.com/base")

    def test_join_ws_url(self) -> None:
        self.assertEqual(
            join_ws_url("ws://127.0.0.1:8080", "/v1/realtime/ws"),
            "ws://127.0.0.1:8080/v1/realtime/ws",
        )

    def test_build_event(self) -> None:
        event = build_event("session.start", 1, {"a": 1}, session_id="sess_123")
        self.assertEqual(event["type"], "session.start")
        self.assertEqual(event["seq"], 1)
        self.assertEqual(event["payload"]["a"], 1)
        self.assertEqual(event["session_id"], "sess_123")
        self.assertIn("ts", event)

    def test_normalize_raw_event(self) -> None:
        event = normalize_raw_event('{"type":"text.in","payload":{"text":"hi"}}', 7, "sess_abc")
        self.assertEqual(event["type"], "text.in")
        self.assertEqual(event["seq"], 7)
        self.assertEqual(event["session_id"], "sess_abc")
        self.assertEqual(event["payload"]["text"], "hi")


if __name__ == "__main__":
    unittest.main()
