const storageKeys = {
  deviceId: "agent-server.web_h5.device_id",
  wakeReason: "agent-server.web_h5.wake_reason",
};

const refs = {
  httpBase: document.getElementById("http-base"),
  deviceId: document.getElementById("device-id"),
  wakeReason: document.getElementById("wake-reason"),
  refreshDiscoveryBtn: document.getElementById("refresh-discovery-btn"),
  saveSettingsBtn: document.getElementById("save-settings-btn"),
  openDebugLink: document.getElementById("open-debug-link"),
  settingsStatus: document.getElementById("settings-status"),
  discoveryStatus: document.getElementById("discovery-status"),
  profileValue: document.getElementById("profile-value"),
  sessionPresetValue: document.getElementById("session-preset-value"),
  inputAudioValue: document.getElementById("input-audio-value"),
  outputAudioValue: document.getElementById("output-audio-value"),
  voiceProviderValue: document.getElementById("voice-provider-value"),
  ttsProviderValue: document.getElementById("tts-provider-value"),
  requirementNote: document.getElementById("requirement-note"),
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

function setBadge(element, text, variant) {
  element.textContent = text;
  element.className = `badge ${variant}`;
}

function renderPreset() {
  refs.httpBase.value = initialHttpBase();
  refs.deviceId.value = initialDeviceId();
  refs.wakeReason.value = initialWakeReason();
  refs.sessionPresetValue.textContent = `${refs.deviceId.value} / ${refs.wakeReason.value}`;
}

function persistPreset() {
  window.localStorage.setItem(storageKeys.deviceId, refs.deviceId.value.trim() || initialDeviceId());
  window.localStorage.setItem(storageKeys.wakeReason, refs.wakeReason.value.trim() || initialWakeReason());
  renderPreset();
  setBadge(refs.settingsStatus, "saved", "badge-ok");
}

async function refreshDiscovery() {
  const response = await fetch(new URL("/v1/realtime", initialHttpBase()), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`discovery failed with status ${response.status}`);
  }

  const payload = await response.json();
  refs.profileValue.textContent = `${payload.protocol_version} / ${payload.subprotocol}`;
  refs.inputAudioValue.textContent =
    `${payload.input_audio.codec} / ${payload.input_audio.sample_rate_hz} Hz / ${payload.input_audio.channels} ch`;
  refs.outputAudioValue.textContent =
    `${payload.output_audio.codec} / ${payload.output_audio.sample_rate_hz} Hz / ${payload.output_audio.channels} ch`;
  refs.voiceProviderValue.textContent = payload.voice_provider || "unknown";
  refs.ttsProviderValue.textContent = payload.tts_provider || "unknown";

  const notes = [];
  if (payload.llm_provider === "bootstrap") {
    notes.push("当前服务端 discovery 显示 LLM provider 为 bootstrap，TTS 更可能播报占位回声文本而不是真正的模型回复。");
  }
  if (payload.tts_provider === "none") {
    notes.push("当前服务端未启用 TTS，因此调试页的文本回包不会伴随二进制音频。");
  }
  if (!payload.output_audio || payload.output_audio.codec !== "pcm16le" || payload.output_audio.channels !== 1) {
    notes.push("当前浏览器调试页只直接支持 mono pcm16le 输出播放。");
  }
  if (!window.isSecureContext && !/^https?:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/i.test(initialHttpBase())) {
    notes.push("远程浏览器开麦通常需要 HTTPS/WSS。");
  }
  refs.requirementNote.textContent = notes.length > 0
    ? notes.join(" ")
    : "当前 discovery 与浏览器调试页兼容。直接进入 Debug 页即可开始联调。";
  setBadge(refs.discoveryStatus, "ready", "badge-ok");
}

function bindActions() {
  renderPreset();
  setBadge(refs.settingsStatus, "saved", "badge-ok");
  setBadge(refs.discoveryStatus, "loading", "badge-neutral");

  refs.saveSettingsBtn.addEventListener("click", () => {
    persistPreset();
  });

  refs.refreshDiscoveryBtn.addEventListener("click", async () => {
    try {
      await refreshDiscovery();
    } catch (error) {
      setBadge(refs.discoveryStatus, "failed", "badge-warn");
      refs.requirementNote.textContent = error.message;
    }
  });

  refs.openDebugLink.addEventListener("click", () => {
    persistPreset();
  });

  refs.deviceId.addEventListener("input", () => {
    refs.sessionPresetValue.textContent = `${refs.deviceId.value.trim() || initialDeviceId()} / ${refs.wakeReason.value.trim() || initialWakeReason()}`;
  });
  refs.wakeReason.addEventListener("input", () => {
    refs.sessionPresetValue.textContent = `${refs.deviceId.value.trim() || initialDeviceId()} / ${refs.wakeReason.value.trim() || initialWakeReason()}`;
  });

  refreshDiscovery().catch((error) => {
    setBadge(refs.discoveryStatus, "failed", "badge-warn");
    refs.requirementNote.textContent = error.message;
  });
}

bindActions();
