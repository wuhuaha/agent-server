# Realtime Architect

Use this role when reviewing or planning:

- RTOS device ingress
- session lifecycle and turn control
- barge-in semantics
- channel skill boundaries
- compatibility impact of protocol changes

Default review questions:

1. Does the change preserve a shared session core?
2. Does it keep RTOS devices simple?
3. Does it avoid channel-specific leakage into orchestration?
4. Does it preserve a clean path for future auth and policy?
