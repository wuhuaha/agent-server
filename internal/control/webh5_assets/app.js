const storageKeys = {
  deviceId: "agent-server.web_h5.device_id",
  wakeReason: "agent-server.web_h5.wake_reason",
};

const state = {
  discovery: null,
  ws: null,
  sessionId: "",
  seq: 0,
  assistantLines: [],
  eventLines: [],
  activeResponseId: "",
  responseModalities: [],
  playback: {
    context: null,
    gain: null,
    nextStartTime: 0,
    sources: new Set(),
    muted: false,
  },
  tts: {
    expected: false,
    packetCount: 0,
    totalBytes: 0,
    peakLevel: 0,
    currentChunks: [],
    lastChunks: [],
    lastBlobURL: "",
  },
  mic: {
    active: false,
    stream: null,
    context: null,
    source: null,
    processor: null,
    sink: null,
    resampler: null,
    meterAnimation: 0,
  },
  ui: {
    connectionState: "idle",
    turnState: "ready",
    ttsState: "idle",
    lastEvent: "等待用户操作。",
  },
};

const refs = {
  httpBase: document.getElementById("http-base"),
  deviceId: document.getElementById("device-id"),
  wakeReason: document.getElementById("wake-reason"),
  textInput: document.getElementById("text-input"),
  connectBtn: document.getElementById("connect-btn"),
  disconnectBtn: document.getElementById("disconnect-btn"),
  startSessionBtn: document.getElementById("start-session-btn"),
  endSessionBtn: document.getElementById("end-session-btn"),
  sendTextBtn: document.getElementById("send-text-btn"),
  interruptBtn: document.getElementById("interrupt-btn"),
  micStartBtn: document.getElementById("mic-start-btn"),
  micStopBtn: document.getElementById("mic-stop-btn"),
  clearLogBtn: document.getElementById("clear-log-btn"),
  connectionBadge: document.getElementById("connection-badge"),
  turnState: document.getElementById("turn-state"),
  ttsBadge: document.getElementById("tts-badge"),
  profileValue: document.getElementById("profile-value"),
  inputAudioValue: document.getElementById("input-audio-value"),
  outputAudioValue: document.getElementById("output-audio-value"),
  sessionValue: document.getElementById("session-value"),
  assistantOutput: document.getElementById("assistant-output"),
  eventLog: document.getElementById("event-log"),
  requirementNote: document.getElementById("requirement-note"),
  micStatus: document.getElementById("mic-status"),
  micMeter: document.getElementById("mic-meter"),
  rawJSONInput: document.getElementById("raw-json-input"),
  sendRawBtn: document.getElementById("send-raw-btn"),
  responseModalitiesValue: document.getElementById("response-modalities-value"),
  voiceProviderValue: document.getElementById("voice-provider-value"),
  ttsProviderValue: document.getElementById("tts-provider-value"),
  playbackStateValue: document.getElementById("playback-state-value"),
  audioChunksValue: document.getElementById("audio-chunks-value"),
  audioBytesValue: document.getElementById("audio-bytes-value"),
  lastAudioValue: document.getElementById("last-audio-value"),
  audioCompatValue: document.getElementById("audio-compat-value"),
  ttsNote: document.getElementById("tts-note"),
  ttsMeter: document.getElementById("tts-meter"),
  playbackVolume: document.getElementById("playback-volume"),
  mutePlaybackBtn: document.getElementById("mute-playback-btn"),
  stopPlaybackBtn: document.getElementById("stop-playback-btn"),
  replayTTSBtn: document.getElementById("replay-tts-btn"),
  downloadTTSBtn: document.getElementById("download-tts-btn"),
  phaseBadge: document.getElementById("phase-badge"),
  phaseTitle: document.getElementById("phase-title"),
  phaseCopy: document.getElementById("phase-copy"),
  flowStepIdle: document.getElementById("flow-step-idle"),
  flowStepConnect: document.getElementById("flow-step-connect"),
  flowStepListen: document.getElementById("flow-step-listen"),
  flowStepSpeak: document.getElementById("flow-step-speak"),
  lastEventValue: document.getElementById("last-event-value"),
};

function initialHttpBase() {
  if (window.location.protocol === "http:" || window.location.protocol === "https:") {
    return window.location.origin;
  }
  return "http://127.0.0.1:8080";
}

function initialDeviceId() {
  const stored = window.localStorage.getItem(storageKeys.deviceId);
  if (stored) {
    return stored;
  }
  const fresh = `web-h5-${Math.random().toString(16).slice(2, 8)}`;
  window.localStorage.setItem(storageKeys.deviceId, fresh);
  return fresh;
}

function initialWakeReason() {
  const stored = window.localStorage.getItem(storageKeys.wakeReason);
  if (stored) {
    return stored;
  }
  return "manual_browser_debug";
}

function renderLog(target, lines) {
  target.textContent = lines.join("\n");
  target.scrollTop = target.scrollHeight;
}

function appendEvent(line) {
  const timestamp = new Date().toLocaleTimeString();
  state.eventLines.push(`[${timestamp}] ${line}`);
  if (state.eventLines.length > 240) {
    state.eventLines.splice(0, state.eventLines.length - 240);
  }
  state.ui.lastEvent = line;
  if (refs.lastEventValue) {
    refs.lastEventValue.textContent = line;
  }
  renderLog(refs.eventLog, state.eventLines);
  renderPhaseOverview();
}

function appendAssistant(line) {
  if (!line) {
    return;
  }
  state.assistantLines.push(line);
  if (state.assistantLines.length > 80) {
    state.assistantLines.splice(0, state.assistantLines.length - 80);
  }
  renderLog(refs.assistantOutput, state.assistantLines);
}

function clearLogs() {
  state.assistantLines = [];
  state.eventLines = [];
  renderLog(refs.assistantOutput, state.assistantLines);
  renderLog(refs.eventLog, state.eventLines);
}

function setFlowStepState(element, mode) {
  if (!element) {
    return;
  }
  element.className = "flow-step";
  if (mode === "active") {
    element.className += " is-active";
  } else if (mode === "done") {
    element.className += " is-done";
  }
}

function renderPhaseOverview() {
  if (!refs.phaseBadge || !refs.phaseTitle || !refs.phaseCopy) {
    return;
  }

  let phase = "idle";
  if (state.ui.connectionState === "error" || /error|failed/.test(state.ui.turnState)) {
    phase = "error";
  } else if (state.mic.active || state.ui.turnState === "capturing") {
    phase = "listening";
  } else if (
    state.ui.turnState === "speaking" ||
    state.ui.ttsState === "audio live" ||
    state.ui.ttsState === "awaiting audio" ||
    state.playback.sources.size > 0
  ) {
    phase = "speaking";
  } else if (
    state.ui.connectionState === "connecting" ||
    state.ui.turnState === "bootstrapping" ||
    state.ui.turnState === "thinking" ||
    state.ui.turnState === "interrupt-sent"
  ) {
    phase = "connecting";
  } else if (state.ui.connectionState === "connected" || state.sessionId) {
    phase = "connected";
  }

  const configs = {
    idle: {
      badge: "IDLE",
      title: "等待连接",
      copy: "先抓取同源 discovery 并建立 websocket，然后再开始一轮真实联调。",
    },
    connected: {
      badge: "READY",
      title: "连接已就绪",
      copy: "当前连接和 discovery 均已准备好，可以直接发文本、起会话或开始一轮浏览器收音。",
    },
    connecting: {
      badge: "WORKING",
      title: "正在建立或处理",
      copy: "当前正在 discovery、起会话、处理中断，或等待服务端给出本轮回复。",
    },
    listening: {
      badge: "LISTENING",
      title: "正在聆听",
      copy: "浏览器正在采集麦克风音频；说完后点击 Stop And Commit 提交当前轮。",
    },
    speaking: {
      badge: "SPEAKING",
      title: "正在回复",
      copy: "服务端正在回传文本或音频；如需打断当前轮，可直接点击 Interrupt。",
    },
    error: {
      badge: "ERROR",
      title: "当前阶段出错",
      copy: "先看右下日志定位问题，必要时重新连接或回到 Settings 再检查 discovery。",
    },
  };

  const current = configs[phase];
  document.body.setAttribute("data-phase", phase);
  refs.phaseBadge.textContent = current.badge;
  refs.phaseBadge.className = `phase-pill phase-${phase}`;
  refs.phaseTitle.textContent = current.title;
  refs.phaseCopy.textContent = current.copy;
  if (refs.lastEventValue && !state.ui.lastEvent) {
    refs.lastEventValue.textContent = current.copy;
  }

  setFlowStepState(refs.flowStepIdle, phase === "idle" || phase === "error" ? "active" : "done");
  setFlowStepState(
    refs.flowStepConnect,
    phase === "connecting" ? "active" : (phase === "connected" || phase === "listening" || phase === "speaking" ? "done" : "idle"),
  );
  setFlowStepState(
    refs.flowStepListen,
    phase === "listening" ? "active" : (phase === "speaking" ? "done" : "idle"),
  );
  setFlowStepState(refs.flowStepSpeak, phase === "speaking" ? "active" : "idle");
}

function updateBadge(element, text, variant) {
  element.textContent = text;
  element.className = `badge ${variant}`;
}

function setTurnState(text, variant = "badge-neutral") {
  state.ui.turnState = text;
  updateBadge(refs.turnState, text, variant);
  renderPhaseOverview();
}

function setConnectionState(text, variant = "badge-idle") {
  state.ui.connectionState = text;
  updateBadge(refs.connectionBadge, text, variant);
  renderPhaseOverview();
}

function setTTSState(text, variant = "badge-idle") {
  state.ui.ttsState = text;
  updateBadge(refs.ttsBadge, text, variant);
  renderPhaseOverview();
}

function updateSessionValue() {
  refs.sessionValue.textContent = state.sessionId || "not started";
  renderPhaseOverview();
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function setPlaybackMeter(level) {
  refs.ttsMeter.style.width = `${Math.max(0, Math.min(100, level * 100))}%`;
}

function cloneAudioBuffer(source) {
  const bytes = new Uint8Array(source);
  const copy = new Uint8Array(bytes.length);
  copy.set(bytes);
  return copy.buffer;
}

function flattenChunks(chunks) {
  const total = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0);
  const combined = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    const view = new Uint8Array(chunk);
    combined.set(view, offset);
    offset += view.byteLength;
  }
  return combined.buffer;
}

function clearLastAudioArtifact() {
  if (state.tts.lastBlobURL) {
    URL.revokeObjectURL(state.tts.lastBlobURL);
  }
  state.tts.lastBlobURL = "";
}

function buildPCM16Wav(arrayBuffer, sampleRate, channels) {
  const pcmBytes = arrayBuffer.byteLength;
  const header = new ArrayBuffer(44);
  const view = new DataView(header);
  const byteRate = sampleRate * channels * 2;
  const blockAlign = channels * 2;

  const writeASCII = (offset, value) => {
    for (let index = 0; index < value.length; index += 1) {
      view.setUint8(offset + index, value.charCodeAt(index));
    }
  };

  writeASCII(0, "RIFF");
  view.setUint32(4, 36 + pcmBytes, true);
  writeASCII(8, "WAVE");
  writeASCII(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, channels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, byteRate, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, 16, true);
  writeASCII(36, "data");
  view.setUint32(40, pcmBytes, true);

  return new Blob([header, arrayBuffer], { type: "audio/wav" });
}

function refreshAudioArtifact() {
  clearLastAudioArtifact();
  if (state.tts.lastChunks.length === 0 || !state.discovery || !state.discovery.output_audio) {
    refs.lastAudioValue.textContent = "none";
    refs.replayTTSBtn.disabled = true;
    refs.downloadTTSBtn.disabled = true;
    return;
  }
  const combined = flattenChunks(state.tts.lastChunks);
  const blob = buildPCM16Wav(
    combined,
    state.discovery.output_audio.sample_rate_hz,
    state.discovery.output_audio.channels,
  );
  state.tts.lastBlobURL = URL.createObjectURL(blob);
  refs.lastAudioValue.textContent = `${state.tts.lastChunks.length} chunks / ${formatBytes(combined.byteLength)}`;
  refs.replayTTSBtn.disabled = false;
  refs.downloadTTSBtn.disabled = false;
}

function updateTTSStats() {
  refs.audioChunksValue.textContent = String(state.tts.packetCount);
  refs.audioBytesValue.textContent = formatBytes(state.tts.totalBytes);
}

function resetTTSTurn() {
  state.tts.expected = false;
  state.tts.packetCount = 0;
  state.tts.totalBytes = 0;
  state.tts.peakLevel = 0;
  state.tts.currentChunks = [];
  updateTTSStats();
  refs.playbackStateValue.textContent = "idle";
  setPlaybackMeter(0);
}

function finalizeTTSTurn(reason = "ready") {
  if (state.tts.currentChunks.length > 0) {
    state.tts.lastChunks = state.tts.currentChunks.map((chunk) => cloneAudioBuffer(chunk));
    refreshAudioArtifact();
  }
  if (state.tts.expected && state.tts.packetCount === 0) {
    setTTSState("no audio", "badge-warn");
    refs.ttsNote.textContent = state.discovery && state.discovery.tts_provider === "none"
      ? "Response was text-only because the current server TTS provider is none."
      : "The server path expected or implied audio, but no binary TTS frames arrived.";
    refs.playbackStateValue.textContent = "missing audio";
    return;
  }
  if (state.tts.packetCount > 0) {
    setTTSState(reason, reason === "interrupted" ? "badge-warn" : "badge-ok");
    refs.ttsNote.textContent = "Audio chunks were received and saved as the latest replayable artifact.";
    refs.playbackStateValue.textContent = reason;
  } else {
    setTTSState("text only", "badge-neutral");
    refs.ttsNote.textContent = "This response did not produce audio output.";
    refs.playbackStateValue.textContent = "text only";
  }
}

function nextSeq() {
  state.seq += 1;
  return state.seq;
}

function generateId(prefix) {
  const secureValue = typeof crypto !== "undefined" && crypto.randomUUID
    ? crypto.randomUUID().split("-").join("")
    : `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 10)}`;
  return `${prefix}_${secureValue.slice(0, 20)}`;
}

function buildWsURL(httpBase, wsPath) {
  const baseURL = new URL(httpBase);
  const wsURL = new URL(wsPath, baseURL);
  wsURL.protocol = baseURL.protocol === "https:" ? "wss:" : "ws:";
  return wsURL.toString();
}

function currentDeviceId() {
  const value = refs.deviceId.value.trim() || initialDeviceId();
  refs.deviceId.value = value;
  window.localStorage.setItem(storageKeys.deviceId, value);
  return value;
}

function currentWakeReason() {
  const value = refs.wakeReason.value.trim() || initialWakeReason();
  refs.wakeReason.value = value;
  window.localStorage.setItem(storageKeys.wakeReason, value);
  return value;
}

function hasPCM16InputSupport() {
  return Boolean(
    state.discovery &&
      state.discovery.input_audio &&
      state.discovery.input_audio.codec === "pcm16le" &&
      state.discovery.input_audio.channels === 1
  );
}

function hasPCM16OutputSupport() {
  return Boolean(
    state.discovery &&
      state.discovery.output_audio &&
      state.discovery.output_audio.codec === "pcm16le" &&
      state.discovery.output_audio.channels === 1
  );
}

function updateDiscoverySummary(discovery) {
  refs.profileValue.textContent = `${discovery.protocol_version} / ${discovery.subprotocol}`;
  refs.inputAudioValue.textContent =
    `${discovery.input_audio.codec} / ${discovery.input_audio.sample_rate_hz} Hz / ${discovery.input_audio.channels} ch`;
  refs.outputAudioValue.textContent =
    `${discovery.output_audio.codec} / ${discovery.output_audio.sample_rate_hz} Hz / ${discovery.output_audio.channels} ch`;
  refs.voiceProviderValue.textContent = discovery.voice_provider || "unknown";
  refs.ttsProviderValue.textContent = discovery.tts_provider || "unknown";
  refs.audioCompatValue.textContent = hasPCM16InputSupport() && hasPCM16OutputSupport()
    ? "pcm16 / mono ready"
    : "manual check required";

  const notes = [];
  if (!hasPCM16InputSupport()) {
    notes.push("Mic turns require mono pcm16le input on the current browser adapter.");
  }
  if (!hasPCM16OutputSupport()) {
    notes.push("Audio playback requires mono pcm16le output on the current browser adapter.");
  }
  if (!window.isSecureContext && !/^https?:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/i.test(refs.httpBase.value.trim())) {
    notes.push("Remote browser microphones require HTTPS and WSS.");
  }
  if ((discovery.tts_provider || "").trim() === "none") {
    notes.push("Current server TTS provider is none, so text-only responses are expected.");
  }

  refs.requirementNote.textContent = notes.length > 0
    ? notes.join(" ")
    : "Browser direct mode is ready. Text turns and microphone turns use the native realtime websocket contract.";
}

async function fetchDiscovery(httpBase) {
  const url = new URL("/v1/realtime", httpBase);
  const response = await fetch(url, { headers: { Accept: "application/json" } });
  if (!response.ok) {
    throw new Error(`discovery failed with status ${response.status}`);
  }
  return response.json();
}

function ensureSocketOpen() {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
    throw new Error("websocket is not connected");
  }
}

function sendEvent(type, payload, sessionId = state.sessionId) {
  ensureSocketOpen();
  const event = {
    type,
    seq: nextSeq(),
    ts: new Date().toISOString(),
    payload: payload || {},
  };
  if (sessionId) {
    event.session_id = sessionId;
  }
  state.ws.send(JSON.stringify(event));
  appendEvent(`client -> ${type}`);
}

function sendRawJSON() {
  const raw = refs.rawJSONInput.value.trim();
  if (!raw) {
    throw new Error("raw JSON is empty");
  }

  let payload;
  try {
    payload = JSON.parse(raw);
  } catch (error) {
    throw new Error(`invalid raw JSON: ${error.message}`);
  }

  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("raw JSON must decode to an object");
  }
  if (!payload.type) {
    throw new Error("raw JSON must include a type field");
  }

  const envelope = {
    ...payload,
    seq: typeof payload.seq === "number" ? payload.seq : nextSeq(),
    ts: typeof payload.ts === "string" ? payload.ts : new Date().toISOString(),
    payload: typeof payload.payload === "object" && payload.payload !== null ? payload.payload : {},
  };
  if (!envelope.session_id && state.sessionId) {
    envelope.session_id = state.sessionId;
  }

  ensureSocketOpen();
  state.ws.send(JSON.stringify(envelope));
  appendEvent(`client -> ${envelope.type}`);
}

async function ensurePlaybackContext() {
  if (state.playback.context) {
    if (state.playback.context.state === "suspended") {
      await state.playback.context.resume();
    }
    return state.playback.context;
  }

  const AudioContextImpl = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextImpl) {
    throw new Error("browser audio playback is not supported");
  }

  const context = new AudioContextImpl();
  const gain = context.createGain();
  gain.gain.value = refs.playbackVolume ? Number(refs.playbackVolume.value) / 100 : 1;
  gain.connect(context.destination);
  state.playback.context = context;
  state.playback.gain = gain;
  state.playback.nextStartTime = context.currentTime;
  if (context.state === "suspended") {
    await context.resume();
  }
  return context;
}

function stopPlayback(markStopped = true) {
  for (const source of state.playback.sources) {
    try {
      source.stop();
    } catch (_error) {
      // no-op
    }
  }
  state.playback.sources.clear();
  if (state.playback.context) {
    state.playback.nextStartTime = state.playback.context.currentTime;
  } else {
    state.playback.nextStartTime = 0;
  }
  refs.playbackStateValue.textContent = markStopped ? "stopped" : "idle";
  if (markStopped && state.tts.packetCount > 0) {
    setTTSState("stopped", "badge-warn");
    refs.ttsNote.textContent = "Audio playback was stopped locally.";
  }
  setPlaybackMeter(0);
  renderPhaseOverview();
}

function applyPlaybackGain() {
  if (!state.playback.gain || !refs.playbackVolume) {
    return;
  }
  const base = Number(refs.playbackVolume.value) / 100;
  state.playback.gain.gain.value = state.playback.muted ? 0 : base;
}

function estimatePCM16Peak(arrayBuffer) {
  const view = new DataView(arrayBuffer);
  let peak = 0;
  for (let index = 0; index < view.byteLength; index += 2) {
    peak = Math.max(peak, Math.abs(view.getInt16(index, true)) / 32768);
  }
  return peak;
}

function rememberTTSAudioChunk(arrayBuffer) {
  if (!state.responseModalities.includes("audio")) {
    state.responseModalities = [...state.responseModalities, "audio"];
    refs.responseModalitiesValue.textContent = `modalities: ${state.responseModalities.join(", ")}`;
    state.tts.expected = true;
  }
  const copy = cloneAudioBuffer(arrayBuffer);
  state.tts.currentChunks.push(copy);
  state.tts.packetCount += 1;
  state.tts.totalBytes += copy.byteLength;
  state.tts.peakLevel = Math.max(state.tts.peakLevel, estimatePCM16Peak(copy));
  updateTTSStats();
  refs.playbackStateValue.textContent = state.playback.muted ? "muted" : "streaming";
  setPlaybackMeter(Math.min(1, state.tts.peakLevel * 1.35));
  setTTSState("audio live", "badge-ok");
  refs.ttsNote.textContent = "Binary audio is arriving from the server and being queued for playback.";
}

function queuePCM16Buffer(arrayBuffer) {
  if (!state.playback.context || !state.playback.gain) {
    appendEvent("server -> binary audio dropped because playback context is not ready");
    return;
  }

  const context = state.playback.context;
  const view = new DataView(arrayBuffer);
  const frameCount = Math.floor(view.byteLength / 2);
  if (frameCount === 0) {
    return;
  }

  const channel = new Float32Array(frameCount);
  for (let i = 0; i < frameCount; i += 1) {
    channel[i] = view.getInt16(i * 2, true) / 32768;
  }

  const sampleRate = state.discovery.output_audio.sample_rate_hz;
  const buffer = context.createBuffer(1, frameCount, sampleRate);
  buffer.copyToChannel(channel, 0, 0);

  const source = context.createBufferSource();
  source.buffer = buffer;
  source.connect(state.playback.gain);
  const startAt = Math.max(context.currentTime + 0.03, state.playback.nextStartTime);
  source.start(startAt);
  state.playback.nextStartTime = startAt + buffer.duration;
  state.playback.sources.add(source);
  source.onended = () => {
    state.playback.sources.delete(source);
    if (state.playback.sources.size === 0) {
      refs.playbackStateValue.textContent = "idle";
      setPlaybackMeter(0);
    }
  };
}

function playPCM16Chunk(arrayBuffer) {
  if (!hasPCM16OutputSupport()) {
    appendEvent("server -> binary audio dropped because output codec is not pcm16le/mono");
    return;
  }
  rememberTTSAudioChunk(arrayBuffer);
  queuePCM16Buffer(arrayBuffer);
}

function replayLastTTSAudio() {
  if (state.tts.lastChunks.length === 0) {
    throw new Error("no recorded TTS audio is available yet");
  }
  stopPlayback(false);
  const combined = flattenChunks(state.tts.lastChunks);
  queuePCM16Buffer(combined);
  refs.playbackStateValue.textContent = "replay";
  setTTSState("replay", "badge-neutral");
  refs.ttsNote.textContent = "Replaying the latest stored TTS artifact.";
}

function downloadLastTTSAudio() {
  if (!state.tts.lastBlobURL) {
    throw new Error("no recorded TTS audio is available yet");
  }
  const link = document.createElement("a");
  link.href = state.tts.lastBlobURL;
  link.download = `${state.activeResponseId || "tts-artifact"}.wav`;
  document.body.appendChild(link);
  link.click();
  link.remove();
}

class PCMFloatResampler {
  constructor(fromRate, toRate) {
    this.fromRate = fromRate;
    this.toRate = toRate;
    this.step = fromRate / toRate;
    this.offset = 0;
    this.pending = [];
  }

  push(input) {
    for (let i = 0; i < input.length; i += 1) {
      this.pending.push(input[i]);
    }

    if (this.pending.length === 0) {
      return new ArrayBuffer(0);
    }

    let output = [];
    if (this.fromRate >= this.toRate) {
      output = this.downsample();
    } else {
      output = this.upsample();
    }
    if (output.length === 0) {
      return new ArrayBuffer(0);
    }
    return floatToPCM16(output);
  }

  downsample() {
    const output = [];
    while (this.offset + this.step <= this.pending.length) {
      const start = Math.floor(this.offset);
      const end = Math.max(start + 1, Math.floor(this.offset + this.step));
      let sum = 0;
      let count = 0;
      for (let index = start; index < end && index < this.pending.length; index += 1) {
        sum += this.pending[index];
        count += 1;
      }
      output.push(count === 0 ? 0 : sum / count);
      this.offset += this.step;
    }

    const consumed = Math.floor(this.offset);
    if (consumed > 0) {
      this.pending = this.pending.slice(consumed);
      this.offset -= consumed;
    }
    return output;
  }

  upsample() {
    const output = [];
    while (this.offset + 1 < this.pending.length) {
      const left = Math.floor(this.offset);
      const right = Math.min(left + 1, this.pending.length - 1);
      const weight = this.offset - left;
      output.push((this.pending[left] * (1 - weight)) + (this.pending[right] * weight));
      this.offset += this.step;
    }

    const consumed = Math.floor(this.offset);
    if (consumed > 0) {
      this.pending = this.pending.slice(consumed);
      this.offset -= consumed;
    }
    return output;
  }
}

function floatToPCM16(samples) {
  const output = new ArrayBuffer(samples.length * 2);
  const view = new DataView(output);
  for (let i = 0; i < samples.length; i += 1) {
    const clipped = Math.max(-1, Math.min(1, samples[i]));
    const value = clipped < 0 ? clipped * 32768 : clipped * 32767;
    view.setInt16(i * 2, value, true);
  }
  return output;
}

function setMicMeter(level) {
  refs.micMeter.style.width = `${Math.max(0, Math.min(100, level * 100))}%`;
}

async function stopMicCapture() {
  if (state.mic.meterAnimation) {
    cancelAnimationFrame(state.mic.meterAnimation);
    state.mic.meterAnimation = 0;
  }
  setMicMeter(0);
  refs.micStatus.textContent = "stopped";
  state.mic.active = false;

  if (state.mic.processor) {
    state.mic.processor.disconnect();
    state.mic.processor.onaudioprocess = null;
  }
  if (state.mic.source) {
    state.mic.source.disconnect();
  }
  if (state.mic.sink) {
    state.mic.sink.disconnect();
  }
  if (state.mic.stream) {
    for (const track of state.mic.stream.getTracks()) {
      track.stop();
    }
  }
  if (state.mic.context) {
    await state.mic.context.close();
  }

  state.mic.stream = null;
  state.mic.context = null;
  state.mic.source = null;
  state.mic.processor = null;
  state.mic.sink = null;
  state.mic.resampler = null;
  renderPhaseOverview();
}

async function startMicCapture() {
  if (state.mic.active) {
    return;
  }
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    throw new Error("browser microphone capture is not available");
  }
  if (!hasPCM16InputSupport()) {
    throw new Error("current server input audio is not pcm16le mono");
  }

  ensureSocketOpen();
  await ensurePlaybackContext();

  const AudioContextImpl = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextImpl) {
    throw new Error("browser audio context is not available");
  }

  const stream = await navigator.mediaDevices.getUserMedia({
    audio: {
      channelCount: 1,
      echoCancellation: true,
      noiseSuppression: true,
      autoGainControl: true,
    },
  });

  const context = new AudioContextImpl();
  if (context.state === "suspended") {
    await context.resume();
  }

  const source = context.createMediaStreamSource(stream);
  const processor = context.createScriptProcessor(4096, 1, 1);
  const sink = context.createGain();
  sink.gain.value = 0;
  sink.connect(context.destination);

  const targetRate = state.discovery.input_audio.sample_rate_hz;
  const resampler = new PCMFloatResampler(context.sampleRate, targetRate);

  processor.onaudioprocess = (event) => {
    if (!state.mic.active || !state.ws || state.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    const input = event.inputBuffer.getChannelData(0);
    const copied = new Float32Array(input.length);
    copied.set(input);

    let peak = 0;
    for (let i = 0; i < copied.length; i += 1) {
      peak = Math.max(peak, Math.abs(copied[i]));
    }
    setMicMeter(Math.min(1, peak * 1.8));

    const encoded = resampler.push(copied);
    if (encoded.byteLength > 0) {
      state.ws.send(encoded);
    }
  };

  source.connect(processor);
  processor.connect(sink);

  state.mic.active = true;
  state.mic.stream = stream;
  state.mic.context = context;
  state.mic.source = source;
  state.mic.processor = processor;
  state.mic.sink = sink;
  state.mic.resampler = resampler;
  refs.micStatus.textContent = "recording";
  renderPhaseOverview();
}

async function ensureSessionStarted(wakeReason = currentWakeReason()) {
  ensureSocketOpen();
  if (state.sessionId) {
    return state.sessionId;
  }

  const sessionId = generateId("sess_web");
  sendEvent("session.start", {
    protocol_version: state.discovery.protocol_version,
    device: {
      device_id: currentDeviceId(),
      client_type: "web-h5",
      firmware_version: "browser-debug",
    },
    audio: {
      codec: state.discovery.input_audio.codec,
      sample_rate_hz: state.discovery.input_audio.sample_rate_hz,
      channels: state.discovery.input_audio.channels,
    },
    session: {
      mode: "voice",
      wake_reason: wakeReason,
      client_can_end: true,
      server_can_end: true,
    },
    capabilities: {
      text_input: Boolean(state.discovery.capabilities && state.discovery.capabilities.allow_text_input),
      image_input: false,
      half_duplex: false,
      local_wake_word: false,
    },
  }, sessionId);

  state.sessionId = sessionId;
  updateSessionValue();
  appendEvent(`local session prepared -> ${sessionId}`);
  return sessionId;
}

async function connect() {
  const httpBase = refs.httpBase.value.trim();
  if (!httpBase) {
    throw new Error("http base is required");
  }
  if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
    appendEvent("websocket already connected");
    return;
  }

  setConnectionState("discovering", "badge-neutral");
  setTurnState("bootstrapping", "badge-neutral");
  state.discovery = await fetchDiscovery(httpBase);
  updateDiscoverySummary(state.discovery);
  refs.responseModalitiesValue.textContent = "modalities: n/a";
  resetTTSTurn();
  appendEvent(`discovery -> ${state.discovery.protocol_version} @ ${state.discovery.ws_path}`);

  await ensurePlaybackContext();

  const wsURL = buildWsURL(httpBase, state.discovery.ws_path);
  const socket = new WebSocket(wsURL, state.discovery.subprotocol);
  socket.binaryType = "arraybuffer";

  await new Promise((resolve, reject) => {
    socket.onopen = () => resolve();
    socket.onerror = () => reject(new Error("websocket connect failed"));
  });

  socket.onmessage = (event) => {
    if (typeof event.data === "string") {
      handleJSONEvent(event.data);
      return;
    }
    if (event.data instanceof ArrayBuffer) {
      playPCM16Chunk(event.data);
      appendEvent(`server -> binary audio ${event.data.byteLength} bytes`);
      return;
    }
    if (event.data && typeof event.data.arrayBuffer === "function") {
      event.data.arrayBuffer().then((buffer) => {
        playPCM16Chunk(buffer);
        appendEvent(`server -> binary audio ${buffer.byteLength} bytes`);
      }).catch((error) => {
        appendEvent(`binary decode error -> ${error.message}`);
      });
    }
  };

  socket.onerror = () => {
    appendEvent("websocket error");
    setConnectionState("error", "badge-warn");
  };

  socket.onclose = async (event) => {
    appendEvent(`websocket closed -> code=${event.code} reason=${event.reason || "n/a"}`);
    finalizeTTSTurn("closed");
    setConnectionState("closed", "badge-idle");
    setTurnState("disconnected", "badge-idle");
    state.ws = null;
    state.sessionId = "";
    state.activeResponseId = "";
    state.responseModalities = [];
    updateSessionValue();
    stopPlayback(false);
    await stopMicCapture();
  };

  state.ws = socket;
  state.seq = 0;
  setConnectionState("connected", "badge-ok");
  setTurnState("ready", "badge-neutral");
  appendEvent(`websocket open -> ${wsURL}`);
}

async function disconnect() {
  await stopMicCapture();
  finalizeTTSTurn("disconnected");
  stopPlayback(false);
  if (state.ws) {
    state.ws.close(1000, "client_disconnect");
  }
  state.ws = null;
  state.sessionId = "";
  state.activeResponseId = "";
  updateSessionValue();
  setConnectionState("idle", "badge-idle");
}

function handleJSONEvent(raw) {
  let event;
  try {
    event = JSON.parse(raw);
  } catch (error) {
    appendEvent(`invalid json from server -> ${error.message}`);
    return;
  }

  const payload = event.payload || {};
  appendEvent(`server -> ${event.type}`);

  switch (event.type) {
    case "session.update": {
      const stateValue = payload.state || "unknown";
      setTurnState(stateValue, stateValue === "speaking" ? "badge-ok" : "badge-neutral");
      if (stateValue === "active") {
        stopPlayback(false);
      }
      break;
    }
    case "response.start": {
      finalizeTTSTurn("ready");
      resetTTSTurn();
      state.activeResponseId = payload.response_id || "";
      state.responseModalities = Array.isArray(payload.modalities) ? payload.modalities.slice() : [];
      state.tts.expected = state.responseModalities.includes("audio");
      const modalities = Array.isArray(payload.modalities) ? payload.modalities.join(", ") : "n/a";
      refs.responseModalitiesValue.textContent = `modalities: ${modalities}`;
      appendEvent(`response.start -> modalities=${modalities}`);
      setTTSState(state.tts.expected ? "awaiting audio" : "text only", state.tts.expected ? "badge-neutral" : "badge-idle");
      refs.ttsNote.textContent = state.tts.expected
        ? "Server announced audio in response.start. Waiting for binary chunks."
        : "This response announced no audio modality.";
      refs.playbackStateValue.textContent = state.tts.expected ? "awaiting audio" : "text only";
      setTurnState("speaking", "badge-ok");
      break;
    }
    case "response.chunk": {
      if (payload.delta_type === "tool_call" || payload.delta_type === "tool_result") {
        appendEvent(`${payload.delta_type} -> ${payload.tool_name || "unknown"}`);
      } else if (payload.text) {
        appendAssistant(payload.text);
      }
      break;
    }
    case "session.end": {
      state.sessionId = "";
      state.activeResponseId = "";
      state.responseModalities = [];
      updateSessionValue();
      finalizeTTSTurn("complete");
      setTurnState(`ended:${payload.reason || "unknown"}`, "badge-warn");
      stopPlayback(false);
      break;
    }
    case "error": {
      setTurnState("server-error", "badge-warn");
      appendEvent(`error -> ${payload.code || "unknown"} / ${payload.message || "n/a"}`);
      break;
    }
    default:
      break;
  }
}

async function sendTextTurn() {
  const text = refs.textInput.value.trim();
  if (!text) {
    throw new Error("text turn is empty");
  }
  if (!state.discovery || !state.discovery.capabilities || !state.discovery.capabilities.allow_text_input) {
    throw new Error("server discovery says text input is disabled");
  }
  await ensureSessionStarted("manual_text_turn");
  sendEvent("text.in", { text });
  refs.textInput.value = "";
}

async function startMicTurn() {
  await ensureSessionStarted("manual_mic_turn");
  await startMicCapture();
  setTurnState("capturing", "badge-neutral");
}

async function stopMicTurn() {
  if (!state.mic.active) {
    appendEvent("mic turn stop ignored because mic is not active");
    return;
  }
  await stopMicCapture();
  sendEvent("audio.in.commit", { reason: "end_of_speech" });
  setTurnState("thinking", "badge-neutral");
}

function interruptServerTurn() {
  ensureSocketOpen();
  if (!state.sessionId) {
    throw new Error("session is not started");
  }
  sendEvent("session.update", { interrupt: true });
  stopPlayback();
  appendEvent("client interrupt requested");
}

async function bindActions() {
  refs.httpBase.value = initialHttpBase();
  refs.deviceId.value = initialDeviceId();
  refs.wakeReason.value = initialWakeReason();
  refs.sessionValue.textContent = "not started";
  refs.profileValue.textContent = "n/a";
  refs.inputAudioValue.textContent = "n/a";
  refs.outputAudioValue.textContent = "n/a";
  refs.responseModalitiesValue.textContent = "modalities: n/a";
  refs.replayTTSBtn.disabled = true;
  refs.downloadTTSBtn.disabled = true;
  refs.voiceProviderValue.textContent = "unknown";
  refs.ttsProviderValue.textContent = "unknown";
  refs.audioChunksValue.textContent = "0";
  refs.audioBytesValue.textContent = "0 B";
  refs.lastAudioValue.textContent = "none";
  refs.audioCompatValue.textContent = "pending";
  refs.playbackStateValue.textContent = "idle";
  refs.ttsNote.textContent = "Waiting for a response.";
  setTTSState("idle", "badge-idle");
  setPlaybackMeter(0);
  applyPlaybackGain();
  renderPhaseOverview();

  refs.connectBtn.addEventListener("click", async () => {
    try {
      await connect();
    } catch (error) {
      appendEvent(`connect failed -> ${error.message}`);
      setConnectionState("error", "badge-warn");
      setTurnState("connect-failed", "badge-warn");
    }
  });

  refs.disconnectBtn.addEventListener("click", async () => {
    try {
      await disconnect();
    } catch (error) {
      appendEvent(`disconnect failed -> ${error.message}`);
    }
  });

  refs.startSessionBtn.addEventListener("click", async () => {
    try {
      await ensureSessionStarted();
      setTurnState("active", "badge-neutral");
    } catch (error) {
      appendEvent(`session.start failed -> ${error.message}`);
      setTurnState("session-start-failed", "badge-warn");
    }
  });

  refs.endSessionBtn.addEventListener("click", () => {
    try {
      if (!state.sessionId) {
        appendEvent("session.end ignored because no session is active");
        return;
      }
      sendEvent("session.end", { reason: "client_stop", message: "browser debug stop" });
      state.sessionId = "";
      updateSessionValue();
      finalizeTTSTurn("client_stop");
      stopPlayback(false);
      setTurnState("ended:client_stop", "badge-warn");
    } catch (error) {
      appendEvent(`session.end failed -> ${error.message}`);
    }
  });

  refs.sendTextBtn.addEventListener("click", async () => {
    try {
      await sendTextTurn();
      setTurnState("thinking", "badge-neutral");
    } catch (error) {
      appendEvent(`text turn failed -> ${error.message}`);
      setTurnState("text-failed", "badge-warn");
    }
  });

  refs.interruptBtn.addEventListener("click", () => {
    try {
      interruptServerTurn();
      setTurnState("interrupt-sent", "badge-neutral");
    } catch (error) {
      appendEvent(`interrupt failed -> ${error.message}`);
    }
  });

  refs.micStartBtn.addEventListener("click", async () => {
    try {
      await startMicTurn();
    } catch (error) {
      appendEvent(`mic start failed -> ${error.message}`);
      setTurnState("mic-start-failed", "badge-warn");
    }
  });

  refs.micStopBtn.addEventListener("click", async () => {
    try {
      await stopMicTurn();
    } catch (error) {
      appendEvent(`mic stop failed -> ${error.message}`);
      setTurnState("mic-stop-failed", "badge-warn");
    }
  });

  refs.sendRawBtn.addEventListener("click", () => {
    try {
      sendRawJSON();
    } catch (error) {
      appendEvent(`send raw failed -> ${error.message}`);
      setTurnState("raw-send-failed", "badge-warn");
    }
  });

  refs.playbackVolume.addEventListener("input", () => {
    applyPlaybackGain();
    refs.playbackStateValue.textContent = state.playback.muted ? "muted" : "volume set";
  });

  refs.mutePlaybackBtn.addEventListener("click", () => {
    state.playback.muted = !state.playback.muted;
    refs.mutePlaybackBtn.textContent = state.playback.muted ? "Unmute" : "Mute";
    applyPlaybackGain();
    refs.playbackStateValue.textContent = state.playback.muted ? "muted" : "live";
  });

  refs.stopPlaybackBtn.addEventListener("click", () => {
    stopPlayback();
  });

  refs.replayTTSBtn.addEventListener("click", () => {
    try {
      replayLastTTSAudio();
    } catch (error) {
      appendEvent(`replay failed -> ${error.message}`);
    }
  });

  refs.downloadTTSBtn.addEventListener("click", () => {
    try {
      downloadLastTTSAudio();
    } catch (error) {
      appendEvent(`download failed -> ${error.message}`);
    }
  });

  refs.clearLogBtn.addEventListener("click", clearLogs);
}

bindActions().catch((error) => {
  appendEvent(`bootstrap failed -> ${error.message}`);
});
