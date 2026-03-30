"""PCM utilities for the desktop realtime debug client."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
import wave


@dataclass(slots=True)
class PCMClip:
    """In-memory PCM clip."""

    sample_rate_hz: int
    channels: int
    sample_width_bytes: int
    frames: bytes

    @property
    def frame_count(self) -> int:
        """Return the number of samples per channel."""
        bytes_per_frame = self.channels * self.sample_width_bytes
        return len(self.frames) // bytes_per_frame


def load_pcm_wav(
    path: str | Path,
    expected_sample_rate_hz: int,
    expected_channels: int,
) -> PCMClip:
    """Load a 16-bit PCM WAV clip and validate it for the protocol baseline."""
    with wave.open(str(path), "rb") as wav_file:
        sample_rate_hz = wav_file.getframerate()
        channels = wav_file.getnchannels()
        sample_width = wav_file.getsampwidth()
        comptype = wav_file.getcomptype()
        if comptype != "NONE":
            raise ValueError("Only uncompressed PCM WAV files are supported.")
        if sample_width != 2:
            raise ValueError("Only 16-bit PCM WAV files are supported.")
        if sample_rate_hz != expected_sample_rate_hz:
            raise ValueError(
                f"Expected sample rate {expected_sample_rate_hz}, got {sample_rate_hz}."
            )
        if channels != expected_channels:
            raise ValueError(f"Expected {expected_channels} channel(s), got {channels}.")
        return PCMClip(
            sample_rate_hz=sample_rate_hz,
            channels=channels,
            sample_width_bytes=sample_width,
            frames=wav_file.readframes(wav_file.getnframes()),
        )


def generate_silence(duration_ms: int, sample_rate_hz: int, channels: int) -> PCMClip:
    """Generate a silence PCM clip for transport tests."""
    frame_count = int(sample_rate_hz * duration_ms / 1000)
    bytes_per_frame = channels * 2
    return PCMClip(
        sample_rate_hz=sample_rate_hz,
        channels=channels,
        sample_width_bytes=2,
        frames=b"\x00" * (frame_count * bytes_per_frame),
    )


def chunk_pcm_bytes(
    frames: bytes,
    sample_rate_hz: int,
    channels: int,
    frame_ms: int = 20,
) -> list[bytes]:
    """Split PCM bytes into transport frames with a fixed duration."""
    bytes_per_frame = channels * 2
    samples_per_chunk = int(sample_rate_hz * frame_ms / 1000)
    chunk_bytes = samples_per_chunk * bytes_per_frame
    if chunk_bytes <= 0:
        raise ValueError("frame_ms must produce a positive chunk size.")
    return [
        frames[offset : offset + chunk_bytes]
        for offset in range(0, len(frames), chunk_bytes)
        if frames[offset : offset + chunk_bytes]
    ]


def write_pcm_wav(
    path: str | Path,
    audio_bytes: bytes,
    sample_rate_hz: int,
    channels: int,
) -> None:
    """Write raw PCM16LE bytes into a WAV container."""
    with wave.open(str(path), "wb") as wav_file:
        wav_file.setnchannels(channels)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate_hz)
        wav_file.writeframes(audio_bytes)
