# Issues And Resolutions

## 2026-03-25

### Writing to E Drive from the Current Workspace

- Problem: the active writable workspace was under `C:\Users\wangt\Documents\New project`, while the new project had to live at `E:\agent-server`.
- Resolution: created `E:\agent-server` and attached it into the workspace via a junction at `C:\Users\wangt\Documents\New project\agent-server`.
- Status: resolved.

### Local Go Toolchain Missing

- Problem: `go` and `gofmt` are not available on the current machine PATH, so local compile-time verification could not run during initialization.
- Resolution: Go was installed at `C:\Program Files\Go\bin`. The current Codex terminal PATH still does not include it, so verification currently uses the absolute tool path or a session-local PATH prefix.
- Status: resolved with environment caveat.

### Go Proxy Reachability

- Problem: `go mod tidy` could not reach `https://proxy.golang.org` from the current network environment, which blocked dependency resolution for `github.com/gorilla/websocket`.
- Resolution: used `GOPROXY=https://goproxy.cn,direct` for module resolution and verification commands.
- Status: resolved for this environment.

### FunASR GPU Compatibility On RTX 5060

- Problem: the existing `xiaozhi-esp32-server` conda environment uses `torch 2.2.2+cu121`, and `SenseVoiceSmall` failed on the local RTX 5060 with `CUDA error: no kernel image is available for execution on the device`.
- Resolution: switched the local FunASR worker default device to `cpu`, which successfully loads the model and completes inference. GPU enablement now depends on upgrading the Python/Torch/CUDA stack in that environment.
- Status: resolved with CPU fallback.

### Git Safe Directory Warning On E Drive

- Problem: earlier sessions reported `E:\agent-server` as a dubious ownership repository, which blocked normal `git` inspection.
- Resolution: rechecked on 2026-03-30 and `git status` now runs cleanly from `E:\agent-server`, so the repository is no longer blocked on `safe.directory`.
- Status: resolved.

## 2026-03-30

### Pion Opus Decoder Output Sizing

- Problem: the first Go-side `opus` normalization attempt failed with the decoder error `out isn't large enough`, which blocked `opus` uplink support.
- Resolution: sized the decode buffer for the library's internally upsampled output, then normalized the decoded samples to `pcm16le/16000/mono` and added regression coverage with `testdata/opus-tiny.ogg`.
- Status: resolved.

### Windows Sandbox Temp Directory For Python Audio Tests

- Problem: `clients/python-desktop-client/tests/test_audio.py` can still fail in this Windows sandbox because `tempfile.TemporaryDirectory()` does not always get a writable location outside the workspace.
- Resolution: not changed in this task; repository-level Go verification remains the authoritative check here, and the Python audio test caveat stays environment-specific.
- Status: open environment caveat.
