const storageKeys = {
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
  saveProfileBtn: document.getElementById("save-profile-btn"),
  fetchDiscoveryBtn: document.getElementById("fetch-discovery-btn"),
  applyDiscoveryBtn: document.getElementById("apply-discovery-btn"),
  openDebugLink: document.getElementById("open-debug-link"),
  settingsStatus: document.getElementById("settings-status"),
  discoveryStatus: document.getElementById("discovery-status"),
  profileSummary: document.getElementById("profile-summary"),
  sessionPresetValue: document.getElementById("session-preset-value"),
  inputAudioSummary: document.getElementById("input-audio-summary"),
  outputAudioSummary: document.getElementById("output-audio-summary"),
  voiceProviderValue: document.getElementById("voice-provider-value"),
  ttsProviderValue: document.getElementById("tts-provider-value"),
  requirementNote: document.getElementById("requirement-note"),
};

const serverMeta = {
  llmProvider: "unknown",
  voiceProvider: "unknown",
  ttsProvider: "unknown",
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

function initialProfile() {
  const raw = window.localStorage.getItem(storageKeys.profile);
  if (!raw) {
    return cloneProfile(defaultProfile);
  }
  try {
    return mergeProfile(defaultProfile, JSON.parse(raw));
  } catch (_error) {
    return cloneProfile(defaultProfile);
  }
}

function initialDeviceId() {
  const stored = window.localStorage.getItem(storageKeys.deviceId);
  if (stored) {
    return stored;
  }
  const fresh = `web-tool-${Math.random().toString(16).slice(2, 8)}`;
  window.localStorage.setItem(storageKeys.deviceId, fresh);
  return fresh;
}

function initialWakeReason() {
  const stored = window.localStorage.getItem(storageKeys.wakeReason);
  if (stored) {
    return stored;
  }
  return "manual_web_tool";
}

function setBadge(element, text, variant) {
  element.textContent = text;
  element.className = `badge ${variant}`;
}

function formatAudio(audio) {
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
  refs.inputAudioSummary.textContent = formatAudio(profile.inputAudio);
  refs.outputAudioSummary.textContent = formatAudio(profile.outputAudio);
  refs.sessionPresetValue.textContent = `${refs.deviceId.value.trim() || initialDeviceId()} / ${refs.wakeReason.value.trim() || initialWakeReason()}`;
  refs.voiceProviderValue.textContent = serverMeta.voiceProvider;
  refs.ttsProviderValue.textContent = serverMeta.ttsProvider;
  updateRequirementNote(profile);
}

function readPositiveInt(value, fieldName) {
  const parsed = Number.parseInt(String(value), 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${fieldName} must be a positive integer`);
  }
  return parsed;
}

function readProfile() {
  return {
    httpBase: refs.httpBase.value.trim(),
    wsPath: refs.wsPath.value.trim(),
    subprotocol: refs.subprotocol.value.trim(),
    protocolVersion: refs.protocolVersion.value.trim(),
    inputAudio: {
      codec: refs.inputCodec.value.trim().toLowerCase(),
      sampleRateHz: readPositiveInt(refs.inputSampleRate.value, "input sample rate"),
      channels: readPositiveInt(refs.inputChannels.value, "input channels"),
    },
    outputAudio: {
      codec: refs.outputCodec.value.trim().toLowerCase(),
      sampleRateHz: readPositiveInt(refs.outputSampleRate.value, "output sample rate"),
      channels: readPositiveInt(refs.outputChannels.value, "output channels"),
    },
    allowTextInput: refs.allowTextInput.checked,
  };
}

function persistCurrentSettings() {
  const profile = readProfile();
  if (!profile.httpBase) {
    throw new Error("HTTP base is required");
  }
  if (!profile.wsPath || !profile.subprotocol || !profile.protocolVersion) {
    throw new Error("profile fields are incomplete");
  }
  window.localStorage.setItem(storageKeys.profile, JSON.stringify(profile));
  window.localStorage.setItem(storageKeys.deviceId, refs.deviceId.value.trim() || initialDeviceId());
  window.localStorage.setItem(storageKeys.wakeReason, refs.wakeReason.value.trim() || initialWakeReason());
  renderProfile(profile);
  setBadge(refs.settingsStatus, "saved", "badge-ok");
}

function updateRequirementNote(profile) {
  const notes = [];
  if (!(profile.inputAudio.codec === "pcm16le" && profile.inputAudio.channels === 1)) {
    notes.push("当前浏览器麦克风适配只支持 mono pcm16le 输入。");
  }
  if (!(profile.outputAudio.codec === "pcm16le" && profile.outputAudio.channels === 1)) {
    notes.push("当前浏览器播放器只支持 mono pcm16le 输出。");
  }
  if (!window.isSecureContext && !/^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/i.test(profile.httpBase)) {
    notes.push("如果不是本机地址，远程浏览器开麦通常需要 HTTPS/WSS。");
  }
  if (serverMeta.ttsProvider === "none") {
    notes.push("当前服务端 discovery 显示 TTS provider 为 none，调试页文本回包正常但不会有音频。");
  }
  if (serverMeta.llmProvider === "bootstrap") {
    notes.push("当前服务端 discovery 显示 LLM provider 为 bootstrap，TTS 更可能播报占位回声文本而不是真正的模型回复。");
  }
  refs.requirementNote.textContent = notes.length > 0
    ? notes.join(" ")
    : "当前 profile 看起来兼容浏览器直接调试路径。保存后进入 Debug 页即可直接做 connect、text turn、mic turn 和 TTS 验证。";
}

function applyDiscovery(payload) {
  const inputAudio = payload.input_audio && typeof payload.input_audio === "object" ? payload.input_audio : {};
  const outputAudio = payload.output_audio && typeof payload.output_audio === "object" ? payload.output_audio : {};
  const capabilities = payload.capabilities && typeof payload.capabilities === "object" ? payload.capabilities : {};

  const nextProfile = mergeProfile(defaultProfile, {
    httpBase: refs.httpBase.value.trim() || defaultProfile.httpBase,
    wsPath: payload.ws_path,
    subprotocol: payload.subprotocol,
    protocolVersion: payload.protocol_version,
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

  serverMeta.llmProvider = payload.llm_provider || "unknown";
  serverMeta.voiceProvider = payload.voice_provider || "unknown";
  serverMeta.ttsProvider = payload.tts_provider || "unknown";
  refs.discoveryJSON.value = JSON.stringify(payload, null, 2);
  renderProfile(nextProfile);
  window.localStorage.setItem(storageKeys.profile, JSON.stringify(nextProfile));
  setBadge(refs.discoveryStatus, "synced", "badge-ok");
  setBadge(refs.settingsStatus, "saved", "badge-ok");
}

async function fetchDiscovery() {
  const httpBase = refs.httpBase.value.trim();
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
  applyDiscovery(payload);
}

function applyDiscoveryFromTextarea() {
  const raw = refs.discoveryJSON.value.trim();
  if (!raw) {
    throw new Error("discovery JSON is empty");
  }
  let payload;
  try {
    payload = JSON.parse(raw);
  } catch (error) {
    throw new Error(`invalid discovery JSON: ${error.message}`);
  }
  applyDiscovery(payload);
}

function bindActions() {
  const profile = initialProfile();
  refs.deviceId.value = initialDeviceId();
  refs.wakeReason.value = initialWakeReason();
  renderProfile(profile);
  setBadge(refs.settingsStatus, "saved", "badge-ok");
  setBadge(refs.discoveryStatus, "manual", "badge-idle");

  refs.saveProfileBtn.addEventListener("click", () => {
    try {
      persistCurrentSettings();
    } catch (error) {
      setBadge(refs.settingsStatus, "error", "badge-warn");
      refs.requirementNote.textContent = error.message;
    }
  });

  refs.fetchDiscoveryBtn.addEventListener("click", async () => {
    try {
      await fetchDiscovery();
    } catch (error) {
      setBadge(refs.discoveryStatus, "failed", "badge-warn");
      refs.requirementNote.textContent = error.message;
    }
  });

  refs.applyDiscoveryBtn.addEventListener("click", () => {
    try {
      applyDiscoveryFromTextarea();
    } catch (error) {
      setBadge(refs.discoveryStatus, "invalid", "badge-warn");
      refs.requirementNote.textContent = error.message;
    }
  });

  refs.openDebugLink.addEventListener("click", (event) => {
    try {
      persistCurrentSettings();
    } catch (error) {
      event.preventDefault();
      setBadge(refs.settingsStatus, "error", "badge-warn");
      refs.requirementNote.textContent = error.message;
    }
  });

  refs.deviceId.addEventListener("input", () => renderProfile(readProfile()));
  refs.wakeReason.addEventListener("input", () => renderProfile(readProfile()));
}

bindActions();
