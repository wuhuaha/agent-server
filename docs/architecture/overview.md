# Architecture Overview

## Primary Layers

### 1. Realtime Session Core

Owns session state, turn management, interruption, response streaming, and shared event semantics.

### 2. Device Adapters

Own ingress and egress for RTOS devices, desktops, and browsers. They translate transport details into core events.

### 3. Channel Skills

Own ingress and egress for external messaging platforms such as Feishu. A channel skill is a transport and message adapter, not a tool runner.

### 4. Voice Runtime

Provides built-in voice capabilities such as turn detection, ASR, TTS, and stream control. It is planned as a default runtime capability.

### 5. Control Plane

Provides health, diagnostics, config, device management, auth, and policy APIs.

## First Practical Shape

- One Go service process.
- One shared event envelope.
- One realtime contract.
- One future plugin path for channels and runtime skills.

## Hard Boundaries

- Session logic must not depend on Feishu, Slack, or any specific device.
- Voice provider logic must stay behind interfaces.
- Device transport code must not own business policy.
- Channel skills must adapt messages into the shared session contract.
