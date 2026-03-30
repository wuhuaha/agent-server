# Security Policy

This policy is adapted from the `SECURITY.md` in [affaan-m/everything-claude-code](https://github.com/affaan-m/everything-claude-code), but scoped to `agent-server` so vulnerability reports and remediation expectations match this repository.

## Supported Versions

| Version | Supported |
| ------- | --------- |
| `main` / unreleased | :white_check_mark: |
| `0.1.x` | :white_check_mark: |
| older experimental snapshots | :warning: best effort |

## Reporting a Vulnerability

If you discover a security vulnerability in `agent-server`, report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, contact the repository maintainers through a private channel before disclosure. If this project later adds a dedicated security email address or GitHub private advisory workflow, update this file to point to that channel.

Include:

- A clear description of the vulnerability
- Steps to reproduce
- The affected branch, version, or deployment shape
- Potential impact and any known mitigations
- Logs, traces, or proof-of-concept details when safe to share

Expected handling target:

- **Acknowledgment** within 72 hours
- **Status update** within 7 days
- **Mitigation or remediation plan** as quickly as practical for confirmed issues

If the report is accepted, maintainers should:

- Coordinate disclosure timing with the reporter
- Ship a mitigation or fix in a timely manner
- Credit the reporter unless anonymity is requested

If the report is declined, maintainers should explain why and clarify whether it belongs in another project or dependency.

## Scope

This policy covers:

- The `agent-server` service and its transport gateways
- RTOS device session handling and realtime protocol logic
- Voice, image, and text orchestration paths in this repository
- Channel skills such as Feishu adapters added to this repository
- Local hooks, scripts, Docker assets, and deployment files shipped here
- Any project-local Codex or Claude automation and skill definitions stored in this repo

## Security Resources

- Realtime protocol: [docs/protocols/realtime-session-v0.md](docs/protocols/realtime-session-v0.md)
- Channel adapter contract: [docs/protocols/channel-skill-contract-v0.md](docs/protocols/channel-skill-contract-v0.md)
- Architecture baseline: [docs/architecture/overview.md](docs/architecture/overview.md)
- ECC security guide inspiration: [the-security-guide.md](https://github.com/affaan-m/everything-claude-code/blob/main/the-security-guide.md)
- OWASP MCP Top 10: [owasp.org/www-project-mcp-top-10](https://owasp.org/www-project-mcp-top-10/)
- OWASP Agentic Applications Top 10: [genai.owasp.org](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/)
