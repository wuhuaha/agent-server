# Household Demo 显式 Profile 入口（2026-04-17）

## 背景

仓库最近已经把 runtime 默认值收敛为 generic：

- `agent.skills = ""`
- `agent.persona = general_assistant`
- `agent.execution_mode = dry_run`
- `voice.entity_catalog_profile = off`

这符合“通用 AI agent server，垂直能力通过 runtime skill/profile 显式接入”的主方向。

但如果没有一个明确的 demo 入口，团队在本地联调、systemd 部署或口头传递时，仍容易把 household 相关值重新抄回通用默认文件里，导致 demo 配置重新污染 shared runtime。

## 目标

本说明把 household demo 能力整理为显式 overlay，而不是默认值：

1. 仓库默认配置继续保持 generic。
2. household demo 通过单独的 profile/env 文件显式开启。
3. `household_control_screen` persona 与 `seed_companion` grounding profile 继续保留，但只作为可选增强项。
4. 本次仅调整文档、示例配置与运行样例，不改动 runtime 代码边界。

## 显式入口

新增 profile 文件：

- `profiles/household-demo.env.example`

它的结构刻意分成“必选项 + 可选增强项”：

- 必选：
  - `AGENT_SERVER_AGENT_SKILLS=household_control`
- 可选：
  - `AGENT_SERVER_AGENT_PERSONA=household_control_screen`
  - `AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家`
  - `AGENT_SERVER_AGENT_EXECUTION_MODE=simulation`
  - `AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=seed_companion`

这样做的含义是：

- household 只是一个 runtime skill，不再代表仓库默认身份
- persona 是展示层/话术层增强，而不是 household skill 的前置条件
- `seed_companion` 只是当前研究阶段的内建 seed grounding profile，不应默认污染所有部署

## 本地启动方式

最小 household demo overlay：

```bash
cp .env.example .env.household
cat profiles/household-demo.env.example >> .env.household
set -a
source ./.env.household
set +a
make run
```

如果你希望恢复更接近此前 household demo 的演示口径，再显式打开可选项：

```bash
cat >> .env.household <<'EOF'
AGENT_SERVER_AGENT_PERSONA=household_control_screen
AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家
AGENT_SERVER_AGENT_EXECUTION_MODE=simulation
AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=seed_companion
EOF
```

## systemd 持久化方式

安装脚本仍然先复制 generic 基线：

- `deploy/systemd/agent-server-agentd.env.example`

如需把机器上的长驻服务切到 household demo，不要改仓库默认值，而是在机器本地显式合并 overlay：

```bash
sudo install -m 0644 deploy/systemd/agent-server-agentd.env.example /etc/agent-server/agentd.env
sudo tee -a /etc/agent-server/agentd.env >/dev/null <<'EOF'
AGENT_SERVER_AGENT_SKILLS=household_control
# Optional:
# AGENT_SERVER_AGENT_PERSONA=household_control_screen
# AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家
# AGENT_SERVER_AGENT_EXECUTION_MODE=simulation
# AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=seed_companion
EOF
sudo systemctl restart agent-server-agentd.service
```

## 为什么不用“直接改默认值”

因为那会重新把仓库推回 vertical demo 默认态，带来几个问题：

1. 新接入端会误以为 household 是核心架构的一部分。
2. generic 语音 runtime 会在未声明场景时带入 seed bias。
3. 后续新增其他 vertical 时，配置层会越来越像“硬编码 demo 选择器”。
4. 文档与本地习惯容易再次偏离，导致默认值漂移。

因此，正确做法不是“让 demo 回到默认值”，而是“让 demo 有明确、可复制、可切换的显式入口”。

## 与当前架构边界的一致性

这次收敛后的职责关系是：

- `internal/agent` / `internal/voice` 继续只暴露 generic runtime 能力与可选 vertical extension
- `.env.example` 与 `deploy/systemd/agent-server-agentd.env.example` 继续作为 generic baseline
- `profiles/household-demo.env.example` 作为显式 overlay，承载当前 household demo 的 opt-in 开关

也就是说，demo profile 是“运行时配置层的显式选择”，不是“shared runtime 的默认身份”。
