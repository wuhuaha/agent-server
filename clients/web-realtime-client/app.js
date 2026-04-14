const storageKeys = {
  profile: "agent-server.clients.web-realtime-client.profile",
  deviceId: "agent-server.clients.web-realtime-client.device_id",
  wakeReason: "agent-server.clients.web-realtime-client.wake_reason",
};

const legacyStorageKeys = {
  profile: "agent-server.tools.web-client.profile",
  deviceId: "agent-server.tools.web-client.device_id",
  wakeReason: "agent-server.tools.web-client.wake_reason",
};

const defaultProfile = {
  httpBase: "http://127.0.0.1:8080",
  wsPath: "/v1/realtime/ws",
  subprotocol: "agent-server.realtime.v0",
  protocolVersion: "rtos-ws-v0",
  inputAudio: {
    codec: "pcm16le",
    sampleRateHz: 16000,
    channels: 1,
  },
  outputAudio: {
    codec: "pcm16le",
    sampleRateHz: 16000,
    channels: 1,
  },
  allowTextInput: true,
};

const state = {
  profile: {
    httpBase: defaultProfile.httpBase,
    wsPath: defaultProfile.wsPath,
    subprotocol: defaultProfile.subprotocol,
    protocolVersion: defaultProfile.protocolVersion,
    inputAudio: { ...defaultProfile.inputAudio },
    outputAudio: { ...defaultProfile.outputAudio },
    allowTextInput: defaultProfile.allowTextInput,
  },
  server: {
    voiceProvider: "unknown",
    ttsProvider: "unknown",
  },
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
  wsPath: document.getElementById("ws-path"),
  subprotocol: document.getElementById("subprotocol"),
  protocolVersion: document.getElementById("protocol-version"),
  deviceId: document.getElementById("device-id"),
  wakeReason: document.getElementById("wake-reason"),
  inputCodec: document.getElementById("input-codec"),
  inputSampleRate: document.getElementById("input-sample-rate"),
  inputChannels: document.getElementById("input-channels"),
  outputCodec: document.getElementById("output-codec"),
  outputSampleRate: document.getElementById("output-sample-rate"),
  outputChannels: document.getElementById("output-channels"),
  allowTextInput: document.getElementById("allow-text-input"),
  discoveryJSON: document.getElementById("discovery-json"),
  connectionBadge: document.getElementById("connection-badge"),
  turnState: document.getElementById("turn-state"),
  ttsBadge: document.getElementById("tts-badge"),
  profileSummary: document.getElementById("profile-summary"),
  inputAudioSummary: document.getElementById("input-audio-summary"),
  outputAudioSummary: document.getElementById("output-audio-summary"),
  sessionValue: document.getElementById("session-value"),
  responseModalitiesValue: document.getElementById("response-modalities-value"),
  requirementNote: document.getElementById("requirement-note"),
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
  textInput: document.getElementById("text-input"),
  rawJSONInput: document.getElementById("raw-json-input"),
  assistantOutput: document.getElementById("assistant-output"),
  eventLog: document.getElementById("event-log"),
  micStatus: document.getElementById("mic-status"),
  micMeter: document.getElementById("mic-meter"),
  connectBtn: document.getElementById("connect-btn"),
  disconnectBtn: document.getElementById("disconnect-btn"),
  startSessionBtn: document.getElementById("start-session-btn"),
  endSessionBtn: document.getElementById("end-session-btn"),
  fetchDiscoveryBtn: document.getElementById("fetch-discovery-btn"),
  applyDiscoveryBtn: document.getElementById("apply-discovery-btn"),
  saveProfileBtn: document.getElementById("save-profile-btn"),
  sendTextBtn: document.getElementById("send-text-btn"),
  interruptBtn: document.getElementById("interrupt-btn"),
  micStartBtn: document.getElementById("mic-start-btn"),
  micStopBtn: document.getElementById("mic-stop-btn"),
  sendRawBtn: document.getElementById("send-raw-btn"),
  clearLogBtn: document.getElementById("clear-log-btn"),
  phaseBadge: document.getElementById("phase-badge"),
  phaseTitle: document.getElementById("phase-title"),
  phaseCopy: document.getElementById("phase-copy"),
  flowStepIdle: document.getElementById("flow-step-idle"),
  flowStepConnect: document.getElementById("flow-step-connect"),
  flowStepListen: document.getElementById("flow-step-listen"),
  flowStepSpeak: document.getElementById("flow-step-speak"),
  lastEventValue: document.getElementById("last-event-value"),
};

function cloneProfile(profile) {
  return {
    httpBase: profile.httpBase,
    wsPath: profile.wsPath,
    subprotocol: profile.subprotocol,
    protocolVersion: profile.protocolVersion,
    inputAudio: { ...profile.inputAudio },
    outputAudio: { ...profile.outputAudio },
    allowTextInput: Boolean(profile.allowTextInput),
  };
}

function readStoredValue(key, legacyKey) {
  const current = window.localStorage.getItem(key);
  if (current !== null) {
    return current;
  }
  if (!legacyKey) {
    return null;
  }
  const legacy = window.localStorage.getItem(legacyKey);
  if (legacy !== null) {
    window.localStorage.setItem(key, legacy);
  }
  return legacy;
}

function initialProfile() {
  const fromStorage = readStoredValue(storageKeys.profile, legacyStorageKeys.profile);
  if (!fromStorage) {
    return cloneProfile(defaultProfile);
  }

  try {
    const parsed = JSON.parse(fromStorage);
    return mergeProfile(defaultProfile, parsed);
  } catch (_error) {
    return cloneProfile(defaultProfile);
  }
}

function initialDeviceId() {
  const stored = readStoredValue(storageKeys.deviceId, legacyStorageKeys.deviceId);
  if (stored) {
    return stored;
  }
  const fresh = `web-client-${Math.random().toString(16).slice(2, 8)}`;
  window.localStorage.setItem(storageKeys.deviceId, fresh);
  return fresh;
}

function initialWakeReason() {
  const stored = readStoredValue(storageKeys.wakeReason, legacyStorageKeys.wakeReason);
  if (stored) {
    return stored;
  }
  return "manual_web_client";
}

function mergeProfile(base, incoming) {
  const merged = cloneProfile(base);
  if (!incoming || typeof incoming !== "object") {
    return merged;
  }
  if (typeof incoming.httpBase === "string") {
    merged.httpBase = incoming.httpBase;
  }
  if (typeof incoming.wsPath === "string") {
    merged.wsPath = incoming.wsPath;
  }
  if (typeof incoming.subprotocol === "string") {
    merged.subprotocol = incoming.subprotocol;
  }
  if (typeof incoming.protocolVersion === "string") {
    merged.protocolVersion = incoming.protocolVersion;
  }
  if (incoming.inputAudio && typeof incoming.inputAudio === "object") {
    if (typeof incoming.inputAudio.codec !== "undefined" && incoming.inputAudio.codec !== null) {
      merged.inputAudio.codec = incoming.inputAudio.codec;
    }
    if (typeof incoming.inputAudio.sampleRateHz !== "undefined" && incoming.inputAudio.sampleRateHz !== null) {
      merged.inputAudio.sampleRateHz = Number(incoming.inputAudio.sampleRateHz);
    }
    if (typeof incoming.inputAudio.channels !== "undefined" && incoming.inputAudio.channels !== null) {
      merged.inputAudio.channels = Number(incoming.inputAudio.channels);
    }
  }
  if (incoming.outputAudio && typeof incoming.outputAudio === "object") {
    if (typeof incoming.outputAudio.codec !== "undefined" && incoming.outputAudio.codec !== null) {
      merged.outputAudio.codec = incoming.outputAudio.codec;
    }
    if (typeof incoming.outputAudio.sampleRateHz !== "undefined" && incoming.outputAudio.sampleRateHz !== null) {
      merged.outputAudio.sampleRateHz = Number(incoming.outputAudio.sampleRateHz);
    }
    if (typeof incoming.outputAudio.channels !== "undefined" && incoming.outputAudio.channels !== null) {
      merged.outputAudio.channels = Number(incoming.outputAudio.channels);
    }
  }
  if (typeof incoming.allowTextInput !== "undefined") {
    merged.allowTextInput = Boolean(incoming.allowTextInput);
  }
  return merged;
}

function renderLog(target, lines) {
  target.textContent = lines.join("\n");
  target.scrollTop = target.scrollHeight;
}

function appendEvent(line) {
  const timestamp = new Date().toLocaleTimeString();
  state.eventLines.push(`[${timestamp}] ${line}`);
  if (state.eventLines.length > 280) {
    state.eventLines.splice(0, state.eventLines.length - 280);
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
  if (state.assistantLines.length > 120) {
    state.assistantLines.splice(0, state.assistantLines.length - 120);
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
      copy: "先建立 websocket 连接，再启动会话、发文本或开始一轮监听。",
    },
    connected: {
      badge: "READY",
      title: "连接已就绪",
      copy: "连接与 profile 已准备好，可以直接开始会话、发文本或启动收音。",
    },
    connecting: {
      badge: "WORKING",
      title: "正在建立或处理",
      copy: "当前正在 discovery、起会话、处理中断，或等待本轮回复返回。",
    },
    listening: {
      badge: "LISTENING",
      title: "正在聆听",
      copy: "浏览器正在采集麦克风音频；说完后点击 Stop And Commit 提交本轮。",
    },
    speaking: {
      badge: "SPEAKING",
      title: "正在回复",
      copy: "服务端正在下发文本或音频；如需打断当前轮，可直接点击 Interrupt。",
    },
    error: {
      badge: "ERROR",
      title: "当前阶段出错",
      copy: "先看右侧日志定位问题，必要时重新连接或回到 Settings 调整配置。",
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

function setConnectionState(text, variant = "badge-idle") {
  state.ui.connectionState = text;
  updateBadge(refs.connectionBadge, text, variant);
  renderPhaseOverview();
}

function setTurnState(text, variant = "badge-neutral") {
  state.ui.turnState = text;
  updateBadge(refs.turnState, text, variant);
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
  if (state.tts.lastChunks.length === 0) {
    refs.lastAudioValue.textContent = "none";
    refs.replayTTSBtn.disabled = true;
    refs.downloadTTSBtn.disabled = true;
    return;
  }
  const combined = flattenChunks(state.tts.lastChunks);
  const blob = buildPCM16Wav(combined, state.profile.outputAudio.sampleRateHz, state.profile.outputAudio.channels);
  state.tts.lastBlobURL = URL.createObjectURL(blob);
  refs.lastAudioValue.textContent = `${state.tts.lastChunks.length} chunks / ${formatBytes(combined.byteLength)}`;
  refs.replayTTSBtn.disabled = false;
  refs.downloadTTSBtn.disabled = false;
}

function updateServerMeta(meta = {}) {
  if (typeof meta.voiceProvider === "string" && meta.voiceProvider.trim()) {
    state.server.voiceProvider = meta.voiceProvider.trim();
  }
  if (typeof meta.ttsProvider === "string" && meta.ttsProvider.trim()) {
    state.server.ttsProvider = meta.ttsProvider.trim();
  }
  refs.voiceProviderValue.textContent = state.server.voiceProvider;
  refs.ttsProviderValue.textContent = state.server.ttsProvider;
}

function updateAudioCompatibility(profile) {
  const inputOK = profile.inputAudio.codec === "pcm16le" && profile.inputAudio.channels === 1;
  const outputOK = profile.outputAudio.codec === "pcm16le" && profile.outputAudio.channels === 1;
  refs.audioCompatValue.textContent = inputOK && outputOK ? "pcm16 / mono ready" : "manual check required";
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
    refs.ttsNote.textContent = state.server.ttsProvider === "none"
      ? "Response was text-only because the current server TTS provider is none."
      : "Response announced audio or likely expected audio, but no binary audio chunks arrived.";
    refs.playbackStateValue.textContent = "missing audio";
    return;
  }
  if (state.tts.packetCount > 0) {
    setTTSState(reason, reason === "interrupted" ? "badge-warn" : "badge-ok");
    refs.ttsNote.textContent = "Audio chunks were received and stored as the latest replayable artifact.";
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

function formatAudioSummary(audio) {
  return `${audio.codec} / ${audio.sampleRateHz} Hz / ${audio.channels} ch`;
}

function renderProfile(profile) {
  refs.httpBase.value = profile.httpBase;
  refs.wsPath.value = profile.wsPath;
  refs.subprotocol.value = profile.subprotocol;
  refs.protocolVersion.value = profile.protocolVersion;
  refs.inputCodec.value = profile.inputAudio.codec;
  refs.inputSampleRate.value = String(profile.inputAudio.sampleRateHz);
  refs.inputChannels.value = String(profile.inputAudio.channels);
  refs.outputCodec.value = profile.outputAudio.codec;
  refs.outputSampleRate.value = String(profile.outputAudio.sampleRateHz);
  refs.outputChannels.value = String(profile.outputAudio.channels);
  refs.allowTextInput.checked = profile.allowTextInput;
  refs.profileSummary.textContent = `${profile.protocolVersion} / ${profile.subprotocol}`;
  refs.inputAudioSummary.textContent = formatAudioSummary(profile.inputAudio);
  refs.outputAudioSummary.textContent = formatAudioSummary(profile.outputAudio);
  updateAudioCompatibility(profile);
  updateRequirementNote(profile);
  updateTTSStats();
}

function persistProfile(profile) {
  window.localStorage.setItem(storageKeys.profile, JSON.stringify(profile));
}

function parsePositiveInt(value, fieldName) {
  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${fieldName} must be a positive integer`);
  }
  return parsed;
}

function readProfileFromInputs() {
  const profile = {
    httpBase: refs.httpBase.value.trim(),
    wsPath: refs.wsPath.value.trim(),
    subprotocol: refs.subprotocol.value.trim(),
    protocolVersion: refs.protocolVersion.value.trim(),
    inputAudio: {
      codec: refs.inputCodec.value.trim().toLowerCase(),
      sampleRateHz: parsePositiveInt(refs.inputSampleRate.value, "input sample rate"),
      channels: parsePositiveInt(refs.inputChannels.value, "input channels"),
    },
    outputAudio: {
      codec: refs.outputCodec.value.trim().toLowerCase(),
      sampleRateHz: parsePositiveInt(refs.outputSampleRate.value, "output sample rate"),
      channels: parsePositiveInt(refs.outputChannels.value, "output channels"),
    },
    allowTextInput: refs.allowTextInput.checked,
  };

  if (!profile.httpBase) {
    throw new Error("HTTP base is required");
  }
  if (!profile.wsPath) {
    throw new Error("WS path is required");
  }
  if (!profile.subprotocol) {
    throw new Error("subprotocol is required");
  }
  if (!profile.protocolVersion) {
    throw new Error("protocol version is required");
  }

  return profile;
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

function generateId(prefix) {
  const secureValue = typeof crypto !== "undefined" && crypto.randomUUID
    ? crypto.randomUUID().split("-").join("")
    : `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 10)}`;
  return `${prefix}_${secureValue.slice(0, 20)}`;
}

function currentProfile() {
  state.profile = readProfileFromInputs();
  renderProfile(state.profile);
  return state.profile;
}

function updateRequirementNote(profile) {
  const notes = [];
  if (!(profile.inputAudio.codec === "pcm16le" && profile.inputAudio.channels === 1)) {
    notes.push("Mic capture currently supports only mono pcm16le input.");
  }
  if (!(profile.outputAudio.codec === "pcm16le" && profile.outputAudio.channels === 1)) {
    notes.push("Audio playback currently supports only mono pcm16le output.");
  }
  if (!window.isSecureContext && !/^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/i.test(profile.httpBase)) {
    notes.push("Remote browser microphones typically require HTTPS and WSS.");
  }
  if (state.server.ttsProvider === "none") {
    notes.push("Current server metadata says TTS provider is none, so text-only responses are expected.");
  }
  refs.requirementNote.textContent = notes.length > 0
    ? notes.join(" ")
    : "Profile looks compatible with the current standalone client path. You can connect and send text or microphone turns directly to the native realtime websocket.";
}

function applyDiscoveryJSON() {
  const raw = refs.discoveryJSON.value.trim();
  if (!raw) {
    throw new Error("discovery JSON is empty");
  }

  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new Error(`invalid discovery JSON: ${error.message}`);
  }

  const inputAudio = parsed.input_audio && typeof parsed.input_audio === "object" ? parsed.input_audio : {};
  const outputAudio = parsed.output_audio && typeof parsed.output_audio === "object" ? parsed.output_audio : {};
  const capabilities = parsed.capabilities && typeof parsed.capabilities === "object" ? parsed.capabilities : {};

  const nextProfile = mergeProfile(defaultProfile, {
    httpBase: refs.httpBase.value.trim() || state.profile.httpBase,
    wsPath: parsed.ws_path,
    subprotocol: parsed.subprotocol,
    protocolVersion: parsed.protocol_version,
    inputAudio: {
      codec: inputAudio.codec,
      sampleRateHz: inputAudio.sample_rate_hz,
      channels: inputAudio.channels,
    },
    outputAudio: {
      codec: outputAudio.codec,
      sampleRateHz: outputAudio.sample_rate_hz,
      channels: outputAudio.channels,
    },
    allowTextInput: capabilities.allow_text_input,
  });

  state.profile = nextProfile;
  updateServerMeta({
    voiceProvider: parsed.voice_provider,
    ttsProvider: parsed.tts_provider,
  });
  renderProfile(nextProfile);
  persistProfile(nextProfile);
  appendEvent("profile updated from discovery JSON");
}

async function fetchDiscoveryProfile() {
  const httpBase = refs.httpBase.value.trim() || state.profile.httpBase;
  if (!httpBase) {
    throw new Error("HTTP base is required before fetching discovery");
  }
  const response = await fetch(new URL("/v1/realtime", httpBase), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`discovery fetch failed with status ${response.status}`);
  }
  const payload = await response.json();
  refs.discoveryJSON.value = JSON.stringify(payload, null, 2);
  appendEvent("discovery fetched from server");
}

function buildWsURL(httpBase, wsPath) {
  const baseURL = new URL(httpBase);
  const wsURL = new URL(wsPath, baseURL);
  wsURL.protocol = baseURL.protocol === "https:" ? "wss:" : "ws:";
  return wsURL.toString();
}

function ensureSocketOpen() {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
    throw new Error("websocket is not connected");
  }
}

function sendEnvelope(envelope) {
  ensureSocketOpen();
  state.ws.send(JSON.stringify(envelope));
  appendEvent(`client -> ${envelope.type}`);
}

function sendEvent(type, payload, sessionId = state.sessionId) {
  const envelope = {
    type,
    seq: nextSeq(),
    ts: new Date().toISOString(),
    payload: payload || {},
  };
  if (sessionId) {
    envelope.session_id = sessionId;
  }
  sendEnvelope(envelope);
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
  if (frameCount <= 0) {
    return;
  }

  const channel = new Float32Array(frameCount);
  for (let index = 0; index < frameCount; index += 1) {
    channel[index] = view.getInt16(index * 2, true) / 32768;
  }

  const buffer = context.createBuffer(1, frameCount, state.profile.outputAudio.sampleRateHz);
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
  const profile = state.profile;
  if (!(profile.outputAudio.codec === "pcm16le" && profile.outputAudio.channels === 1)) {
    appendEvent("server -> binary audio dropped because output profile is not pcm16le/mono");
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
    for (let index = 0; index < input.length; index += 1) {
      this.pending.push(input[index]);
    }
    if (this.pending.length === 0) {
      return new ArrayBuffer(0);
    }

    const output = this.fromRate >= this.toRate ? this.downsample() : this.upsample();
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
  for (let index = 0; index < samples.length; index += 1) {
    const clipped = Math.max(-1, Math.min(1, samples[index]));
    const value = clipped < 0 ? clipped * 32768 : clipped * 32767;
    view.setInt16(index * 2, value, true);
  }
  return output;
}

function setMicMeter(level) {
  refs.micMeter.style.width = `${Math.max(0, Math.min(100, level * 100))}%`;
}

async function stopMicCapture() {
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
  const profile = currentProfile();
  if (state.mic.active) {
    return;
  }
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    throw new Error("browser microphone capture is not available");
  }
  if (!(profile.inputAudio.codec === "pcm16le" && profile.inputAudio.channels === 1)) {
    throw new Error("standalone mic capture currently requires mono pcm16le input");
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

  const resampler = new PCMFloatResampler(context.sampleRate, profile.inputAudio.sampleRateHz);
  processor.onaudioprocess = (event) => {
    if (!state.mic.active || !state.ws || state.ws.readyState !== WebSocket.OPEN) {
      return;
    }

    const input = event.inputBuffer.getChannelData(0);
    const copied = new Float32Array(input.length);
    copied.set(input);

    let peak = 0;
    for (let index = 0; index < copied.length; index += 1) {
      peak = Math.max(peak, Math.abs(copied[index]));
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
  currentProfile();
  ensureSocketOpen();
  if (state.sessionId) {
    return state.sessionId;
  }

  const sessionId = generateId("sess_webclient");
  sendEvent("session.start", {
    protocol_version: state.profile.protocolVersion,
    device: {
      device_id: currentDeviceId(),
      client_type: "web-h5-client",
      firmware_version: "web-realtime-client",
    },
    audio: {
      codec: state.profile.inputAudio.codec,
      sample_rate_hz: state.profile.inputAudio.sampleRateHz,
      channels: state.profile.inputAudio.channels,
    },
    session: {
      mode: "voice",
      wake_reason: wakeReason,
      client_can_end: true,
      server_can_end: true,
    },
    capabilities: {
      text_input: state.profile.allowTextInput,
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
  const profile = currentProfile();
  if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
    appendEvent("websocket already connected");
    return;
  }

  await ensurePlaybackContext();

  setConnectionState("connecting", "badge-neutral");
  setTurnState("bootstrapping", "badge-neutral");

  const wsURL = buildWsURL(profile.httpBase, profile.wsPath);
  const socket = new WebSocket(wsURL, profile.subprotocol);
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
    stopPlayback();
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
  stopPlayback();
  if (state.ws) {
    state.ws.close(1000, "client_disconnect");
  }
  state.ws = null;
  state.sessionId = "";
  updateSessionValue();
  setConnectionState("idle", "badge-idle");
  setTurnState("ready", "badge-neutral");
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
      const nextState = payload.state || "unknown";
      setTurnState(nextState, nextState === "speaking" ? "badge-ok" : "badge-neutral");
      if (nextState === "active") {
        stopPlayback(false);
      }
      break;
    }
    case "response.start": {
      const modalities = Array.isArray(payload.modalities) ? payload.modalities.join(", ") : "n/a";
      finalizeTTSTurn("ready");
      resetTTSTurn();
      state.activeResponseId = payload.response_id || "";
      state.responseModalities = Array.isArray(payload.modalities) ? payload.modalities.slice() : [];
      state.tts.expected = state.responseModalities.includes("audio");
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
      updateSessionValue();
      finalizeTTSTurn("complete");
      stopPlayback(false);
      setTurnState(`ended:${payload.reason || "unknown"}`, "badge-warn");
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
  const profile = currentProfile();
  if (!profile.allowTextInput) {
    throw new Error("text input is disabled in the current client profile");
  }

  const text = refs.textInput.value.trim();
  if (!text) {
    throw new Error("text turn is empty");
  }

  await ensureSessionStarted("manual_text_turn");
  sendEvent("text.in", { text });
  refs.textInput.value = "";
  setTurnState("thinking", "badge-neutral");
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

  sendEnvelope(envelope);
}

function restoreFromQueryParams() {
  const params = new URLSearchParams(window.location.search);
  const queryProfile = {};
  if (params.get("httpBase")) {
    queryProfile.httpBase = params.get("httpBase");
  }
  if (params.get("wsPath")) {
    queryProfile.wsPath = params.get("wsPath");
  }
  if (params.get("subprotocol")) {
    queryProfile.subprotocol = params.get("subprotocol");
  }
  if (params.get("protocolVersion")) {
    queryProfile.protocolVersion = params.get("protocolVersion");
  }
  const merged = mergeProfile(state.profile, queryProfile);
  state.profile = merged;
}

function bindActions() {
  state.profile = initialProfile();
  restoreFromQueryParams();
  refs.wakeReason.value = initialWakeReason();
  refs.deviceId.value = initialDeviceId();
  refs.sessionValue.textContent = "not started";
  refs.responseModalitiesValue.textContent = "modalities: n/a";
  refs.replayTTSBtn.disabled = true;
  refs.downloadTTSBtn.disabled = true;
  updateServerMeta();
  updateTTSStats();
  setTTSState("idle", "badge-idle");
  refs.playbackStateValue.textContent = "idle";
  refs.lastAudioValue.textContent = "none";
  refs.audioCompatValue.textContent = "pending";
  refs.ttsNote.textContent = "Waiting for a response.";
  setPlaybackMeter(0);
  renderProfile(state.profile);
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
      sendEvent("session.end", { reason: "client_stop", message: "web realtime client stop" });
      state.sessionId = "";
      updateSessionValue();
      finalizeTTSTurn("client_stop");
      stopPlayback(false);
      setTurnState("ended:client_stop", "badge-warn");
    } catch (error) {
      appendEvent(`session.end failed -> ${error.message}`);
    }
  });

  if (refs.fetchDiscoveryBtn) {
    refs.fetchDiscoveryBtn.addEventListener("click", async () => {
      try {
        await fetchDiscoveryProfile();
        setTurnState("discovery-fetched", "badge-neutral");
      } catch (error) {
        appendEvent(`fetch discovery failed -> ${error.message}`);
        setTurnState("discovery-fetch-failed", "badge-warn");
      }
    });
  }

  if (refs.applyDiscoveryBtn) {
    refs.applyDiscoveryBtn.addEventListener("click", () => {
      try {
        applyDiscoveryJSON();
      } catch (error) {
        appendEvent(`apply discovery failed -> ${error.message}`);
        setTurnState("profile-error", "badge-warn");
      }
    });
  }

  if (refs.saveProfileBtn) {
    refs.saveProfileBtn.addEventListener("click", () => {
      try {
        const profile = currentProfile();
        persistProfile(profile);
        appendEvent("profile saved to local storage");
      } catch (error) {
        appendEvent(`save profile failed -> ${error.message}`);
      }
    });
  }

  refs.sendTextBtn.addEventListener("click", async () => {
    try {
      await sendTextTurn();
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

bindActions();
