from __future__ import annotations

import unittest

from agent_server_desktop_client.rtos_mock import _build_parser


class RTOSMockTests(unittest.TestCase):
    def test_parser_defaults(self) -> None:
        parser = _build_parser()
        args = parser.parse_args([])
        self.assertEqual(args.http_base, "http://127.0.0.1:8080")
        self.assertEqual(args.device_id, "rtos-mock-001")
        self.assertFalse(args.no_auto_end)
        self.assertFalse(args.no_interrupt_update)


if __name__ == "__main__":
    unittest.main()
