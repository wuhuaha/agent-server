# 运行时 `entity catalog / alias / canonical grounding` MVP（2026-04-17）

## 文档性质

- 本文记录当前仓库已经落地的第一版运行时实体归一化能力。
- 它不是最终的个性化 catalog / dynamic bias 服务设计，而是研究阶段的高 ROI MVP。
- 目标是把此前停留在研究里的 `alias -> canonical entity -> slot completeness` 路径，接进当前 `SemanticSlotParser` 与 preview 仲裁主链。

## 这一步解决什么问题

在只接入 `SemanticSlotParser` 的情况下，运行时已经能知道：

- 这更像 `smart_home` 还是 `desktop_assistant`
- 当前是 `clarify_needed`、`wait_more` 还是 `act_candidate`
- 当前可能缺少 `target` / `location` / `target_app`

但它还不能稳定回答另外一类更关键的问题：

- `客厅灯` 到底是不是一个可执行的 canonical target
- `VS Code` 是否可以被归并到统一 app identity
- `灯` / `空调` / `窗帘` 这类泛称是否已经唯一，还是仍应立刻澄清

因此，这一层的价值不是“再做一次意图识别”，而是：

- 把 LLM 给出的结构化语义结果，继续压到 runtime-owned 的可执行对象上
- 让 preview 仲裁知道“缺槽”和“已听到但仍歧义”之间的区别

## 当前落地边界

当前能力明确放在：

- `internal/voice.SemanticSlotParser`
- `internal/voice.EntityCatalogGrounder`
- `InputPreview.Arbitration`

之间。

也就是说：

1. LLM slot parser 先做 `domain / intent / slot completeness / actionability` 的结构化判断。
2. runtime-owned `EntityCatalogGrounder` 再根据当前 preview 文本做 alias 匹配与 canonical grounding。
3. grounding 结果只以摘要形式并回 `TurnArbitration`，而不是让 adapter 或 gateway 直接拥有领域对象。

## 明确不做的事情

这一版刻意不做：

- 不把 entity catalog 下沉到 gateway / websocket adapter
- 不让 LLM 直接输出最终 canonical id 作为主判
- 不因为 catalog 未命中就否定 parser 结果
- 不引入重型个性化配置系统或外部目录服务
- 不把 dynamic bias 反向塞回 transport 层协议

## 当前 seed catalog 范围

当前内置的是一个小型 demo seed catalog，用于验证主链行为，覆盖两个 namespace：

### `smart_home`

- room：`客厅` / `书房` / `卧室`
- device_group / device：
  - `客厅灯`
  - `客厅筒灯`
  - `书房灯`
  - `客厅空调`
  - `卧室空调`
  - `客厅窗帘`
  - `书房窗帘`

### `desktop_assistant`

- app：
  - `Visual Studio Code`
  - `浏览器`
  - `终端`

每个实体至少具备：

- `entity_id`
- `namespace`
- `entity_type`
- `canonical_name`
- `aliases[]`
- 可选 `common_misrecognitions[]`
- 对家居实体的 `room_id` / `device_group`

## 当前 grounding 规则

## 1. 只使用正向证据做修正

这是当前最重要的约束。

如果 catalog 没命中，不代表用户没说对，只代表当前 seed catalog 还不完整。

因此当前 grounder 只在两种情况下改写 slot parse 结果：

- **命中了唯一 canonical entity**
- **命中了明确多候选歧义**

如果完全没命中：

- 保持 slot parser 原结果
- 不因为 catalog 缺词而把 `complete` 拉回 `missing`

## 2. 先按 namespace / entity type 粗筛

- `smart_home` 只看 `room / device / device_group`
- `desktop_assistant` 只看 `app`

这样可避免一个 seed catalog 同时服务多域时的误吸附。

## 3. 使用最具体 alias 优先

匹配时不是“只要包含就都算”，而是：

- 先为每个实体找出当前文本命中的最长 alias
- 再只保留当前候选里最具体的那一档 alias

这样可以避免：

- `客厅筒灯` 命中时，被更泛的 `灯` 干扰
- `客厅窗帘` 命中时，被泛称 `窗帘` 抢走主判

## 4. 如果唯一房间已知，则先按房间过滤设备候选

例如：

- 文本里已唯一命中 `书房`
- 同时设备候选里命中了多个 `灯`

则优先保留 `room_id=书房` 的设备候选。

这让 `书房灯` 这类短命令，即便 target 本身 alias 偏泛，也能被稳定收敛。

## 5. 让“缺槽”和“歧义槽”严格区分

当前 grounder 的核心收益之一，是把以下两种情况拉开：

- **missing**：还没形成可用对象
- **ambiguous**：听到了对象，但对象不唯一

例如：

- `打开客厅灯`
  - 可唯一 grounding
  - `target` 从 `missing` 清除
  - promotion 到 `act_candidate`
- `把灯调亮一点`
  - 已命中多个灯类实体
  - 不再只是“缺 target”
  - 而是 `ambiguous target -> clarify_needed`

## 当前产出的摘要字段

当前 grounding 会进入 `InputPreview.Arbitration` 的摘要字段包括：

- `slot_grounded`
- `slot_canonical_target`
- `slot_canonical_location`
- `slot_missing`
- `slot_ambiguous`
- `slot_status`
- `slot_actionability`

同时，这些摘要也进入 preview prewarm metadata，便于后续 runtime 复用。

## 当前可以稳定改善的典型例子

### 例 1：智能家居唯一目标

输入：`打开客厅灯`

当前可得到：

- `slot_grounded=true`
- `slot_canonical_target=客厅灯`
- `slot_canonical_location=客厅`
- `slot_status=complete`
- `slot_actionability=act_candidate`

### 例 2：泛称歧义目标

输入：`把灯调亮一点`

当前可得到：

- `slot_status=ambiguous`
- `slot_actionability=clarify_needed`
- `slot_ambiguous=[target]`

### 例 3：桌面助理 alias 归一

输入：`打开 VS Code`

当前可得到：

- `slot_grounded=true`
- `slot_canonical_target=Visual Studio Code`
- `slot_status=complete`
- `slot_actionability=act_candidate`

## 当前限制

这仍然只是 MVP，限制包括：

- catalog 还是内置 seed，不是 session-scoped dynamic catalog
- 还没有 user/profile/room memory 注入
- 还没有把 alias / recent entity / active context 回灌给 ASR hotword
- 还没有 value normalization（如温度、百分比、时间）
- 还没有 entity risk model（高风险设备 / 高风险动作）
- 还没有更完整的 desktop domain（window/file/contact/browser-tab）

## 下一步最值得做的扩展

按 ROI 排序，下一步最值得继续做的是：

1. **把 session recent context 接进 catalog ranking**
   - 最近提到的房间 / 设备 / app 应提升优先级
2. **把 room / device_group / app alias 反哺给 ASR bias**
   - preview 轻 bias，final 强 correction
3. **补 canonical value normalization**
   - 亮度 / 温度 / 音量 / 模式 / 时间
4. **补风险与确认门槛**
   - 门锁、支付、删除等高风险对象
5. **把 seed catalog 进化为 runtime-managed catalog source**
   - 但仍保持 `internal/voice` runtime-owned，不把它下沉到 adapter

## 当前结论

这一步的真正价值不是“又多了一份字典”，而是：

- 把 `SemanticSlotParser` 从“知道缺不缺槽”推进到“能不能把槽位映射到真实对象”
- 让 preview arbitration 能更早地区分：
  - 继续等更多语音
  - 现在就该澄清
  - 已经足够进入 act candidate

而且这仍然保持了当前项目最重要的边界：

- 语义解析可以借助 LLM
- canonical grounding 由 runtime-owned catalog 完成
- 最终 accept 仍由 shared voice runtime orchestration 掌控
