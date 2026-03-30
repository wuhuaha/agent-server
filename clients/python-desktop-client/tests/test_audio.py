from __future__ import annotations

from pathlib import Path
import tempfile
import unittest

from agent_server_desktop_client.audio import (
    chunk_pcm_bytes,
    generate_silence,
    load_pcm_wav,
    write_pcm_wav,
)


class AudioTests(unittest.TestCase):
    def test_generate_silence_size(self) -> None:
        clip = generate_silence(1000, 16000, 1)
        self.assertEqual(len(clip.frames), 32000)

    def test_chunk_pcm_bytes_uses_20ms_frames(self) -> None:
        chunks = chunk_pcm_bytes(b"\x00" * 3200, 16000, 1, frame_ms=20)
        self.assertEqual(len(chunks), 5)
        self.assertTrue(all(len(chunk) == 640 for chunk in chunks))

    def test_write_and_load_pcm_wav_round_trip(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            path = Path(tmp_dir) / "round-trip.wav"
            original = b"\x01\x00" * 160
            write_pcm_wav(path, original, 16000, 1)
            clip = load_pcm_wav(path, 16000, 1)
            self.assertEqual(clip.frames, original)
            self.assertEqual(clip.sample_rate_hz, 16000)
            self.assertEqual(clip.channels, 1)


if __name__ == "__main__":
    unittest.main()
