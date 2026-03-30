# Speech synthesis

Speech Synthesis (Text-to-Speech) automatically converts input text into natural and fluent speech output. You can configure parameters such as speech style to generate expressive and vivid speech content.

**Core Capabilities**

- **Provides built-in voices:** Built-in default tones meet the needs for quick use.

- **Diverse speech styles:** Supports specifying speech styles for more vivid and natural voices.

## Supported Models

Only the `mimo-v2-tts` model is currently supported.

## Preparation  

For preparations such as obtaining the API Key, please refer to [First API Call](https://platform.xiaomimimo.com/#/docs/quick-start/first-api-call).

## Available Built-in Voices

You may set the built-in voice in `{"audio": {"voice": "mimo_default"}}`.

| **Voice Name** | **Voice Parameter** |
| --- | --- |
| MiMo-Default | mimo_default |
| MiMo-Chinese Female Voice | default_zh |
| MiMo-English Female Voice | default_en |

> Currently, voice cloning is not supported.

## Style Control

### Overall Voice Style Control

Place `<style>style</style>` at the beginning of the target text for conversion, where `style` is the audio style to be generated. If multiple styles need to be set, place multiple style names within the same `<style>` tag, with no restrictions on the separator.

**Format example:** `<style>Style 1 Style 2</style>Content to be synthesized`.

The following are some recommended styles, and styles not on the list are also supported.

| **Style Type** | **Style Example** |
| --- | --- |
| Speech rate control | *Speed up / Slow down* |
| Emotional changes | *Happy / Sad / Angry* |
| Role-playing | *Sun Wukong / Lin Daiyu* |
| Style change | *Whisper / Clamped voice / Taiwanese accent* |
| Dialect | *Northeastern dialect / Sichuan dialect / Henan dialect / Cantonese* |

**Sample:**
- `<style>Happy</style>Tomorrow is Friday, so happy!`
- `<style>Whisper</style>Oh my goodness, it's so cold today! You know that wind, it's howling like a knife, cutting into your face!`

### Fine-grained Control of Audio Tags

Through [Audio Tags], you can exercise fine-grained control over sound, precisely adjusting tone, emotion, and expression style—whether it's a whisper, a hearty laugh, or a little rant with a touch of emotion. You can also flexibly insert breaths, pauses, coughs, etc., all of which can be easily achieved. The speaking speed can also be flexibly adjusted, ensuring that every sentence has its proper rhythm.

**Sample:**
- Achoo! Ahem. I—I really [cough] think I am coming down with  a terrible [cough] terrible cold.
- [heavy breathing] Just... give me... a second. I ran... all the way... from the station.
- I just feel... *long sigh*... like I'm constantly treading water, you know?
- It's just so stupid! (sobbing) We spent all that money on the cake and the dog just... (sudden laugh) he just ate the whole thing in one bite!

## Code Sample

<div className='mdx-highlight'>

**Notes**
- The target text for speech synthesis must be placed in a message with `role`: `assistant`, not in a message with `role`: `user`.
- The message of the `user` role is an optional parameter, but it is recommended that users carry it. You can adjust the tone and style of speech synthesis in some scenarios.
- To specify the speech style, place `<style>style</style>` at the beginning of the target text.
- To achieve a better singing style, you must add only the tag `<style>唱歌</style>` at the very beginning of the target text, in the format: `<style>唱歌</style>target_text`.

</div>

### Non-streaming Call

**Curl**

```bash
curl --location --request POST 'https://api.xiaomimimo.com/v1/chat/completions' \
--header "api-key: $MIMO_API_KEY" \
--header 'Content-Type: application/json' \
--data-raw '{
    "model": "mimo-v2-tts",
    "messages": [
        {
            "role": "user",
            "content": "Hello, MiMo, have you had lunch?"
        },
        {
            "role": "assistant",
            "content": "Yes, I had a sandwich."
        }
    ],
    "audio": {
        "format": "wav",
        "voice": "mimo_default"
    }
}'
```

**Python**

```python
import os
from openai import OpenAI
import base64

client = OpenAI(
    api_key=os.environ.get("MIMO_API_KEY"),
    base_url="https://api.xiaomimimo.com/v1"
)

completion = client.chat.completions.create(
    model="mimo-v2-tts",
    messages=[
        {
            "role": "user",
            "content": "Hello, MiMo, have you had lunch?"
        },
        {
            "role": "assistant",
            "content": "Yes, I had a sandwich."
        }
    ],
    audio={
        "format": "wav",
        "voice": "mimo_default"
    }
)

message = completion.choices[0].message
audio_bytes = base64.b64decode(message.audio.data)
with open("audio_file.wav", "wb") as f:
    f.write(audio_bytes)
```

### Streaming Call

<div className='mdx-highlight'>

**Notes**
- When using streaming calls, please specify the format of the output audio as `pcm16` to facilitate splicing into a complete audio. For a splicing example, please refer to the Python calling method.

</div>

**Curl**

```bash
curl --location --request POST 'https://api.xiaomimimo.com/v1/chat/completions' \
--header "api-key: $MIMO_API_KEY" \
--header 'Content-Type: application/json' \
--data-raw '{
    "model": "mimo-v2-tts",
    "messages": [
        {
            "role": "assistant",
            "content": "You are UN-BE-LIEVABLE! I am sooooo done with your constant lies. GET. OUT!"
        }
    ],
    "audio": {
        "format": "pcm16",
        "voice": "default_en"
    },
    "stream": true
}'
```

**Python**

```python
import base64
import os
import numpy as np
import soundfile as sf
from openai import OpenAI

client = OpenAI(
    api_key=os.environ.get("MIMO_API_KEY"),
    base_url="https://api.xiaomimimo.com/v1"
)

completion = client.chat.completions.create(
    model="mimo-v2-tts",
    messages=[
        {
            "role": "assistant",
            "content": "You are UN-BE-LIEVABLE! I am sooooo done with your constant lies. GET. OUT!"
        }
    ],
    audio={
        "format": "pcm16",
        "voice": "default_en"
    },
    stream=True
)

# 24kHz PCM16LE mono audio
collected_chunks: np.ndarray = np.array([], dtype=np.float32)

for chunk in completion:
    if not chunk.choices:
        continue
    delta = chunk.choices[0].delta
    audio = getattr(delta, "audio", None)

    if audio is not None:
        assert isinstance(audio, dict), f"Expected audio to be a dict, got {type(audio)}"
        pcm_bytes = base64.b64decode(audio["data"])
        np_pcm = np.frombuffer(pcm_bytes, dtype=np.int16).astype(np.float32) / 32768.0
        collected_chunks = np.concatenate((collected_chunks, np_pcm))
        print(f"Received audio chunk of size {len(pcm_bytes)} bytes")

# Save the collected audio to a file
os.makedirs("tmp", exist_ok=True)
sf.write("tmp/output.wav", collected_chunks, samplerate=24000)
print("Audio saved to tmp/output.wav")
```

## Price

- Billing: Free for a limited time.

- View Bill: You can view your usage on the [Billing](https://platform.xiaomimimo.com/#/console/usage) page in the Console.

