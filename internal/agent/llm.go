package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	defaultAssistantName      = "小欧管家"
	defaultAgentPersona       = "household_control_screen"
	defaultAgentExecutionMode = "simulation"
)

type ChatModel interface {
	Complete(context.Context, ChatModelRequest) (ChatModelResponse, error)
}

type StreamingChatModel interface {
	Stream(context.Context, ChatModelRequest, ChatModelDeltaSink) (ChatModelResponse, error)
}

type ChatModelRequest struct {
	SessionID     string
	DeviceID      string
	ClientType    string
	UserText      string
	SystemPrompt  string
	MemoryContext MemoryContext
	Metadata      map[string]string
	Images        []ImageInput
	Messages      []ChatMessage
	Tools         []ToolDefinition
}

type ChatModelResponse struct {
	Text         string
	FinishReason string
	Message      ChatMessage
}

type ChatMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ChatToolCall
}

type ChatToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ChatModelDelta struct {
	Text string
}

type ChatModelDeltaSink interface {
	EmitChatModelDelta(context.Context, ChatModelDelta) error
}

type ChatModelDeltaSinkFunc func(context.Context, ChatModelDelta) error

func (f ChatModelDeltaSinkFunc) EmitChatModelDelta(ctx context.Context, delta ChatModelDelta) error {
	return f(ctx, delta)
}

type PromptSection struct {
	Name    string
	Content string
}

type PromptSectionRequest struct {
	SessionID     string
	DeviceID      string
	ClientType    string
	UserText      string
	Metadata      map[string]string
	Template      string
	AssistantName string
	Persona       string
	ExecutionMode string
}

type PromptSectionProvider interface {
	ListPromptSections(context.Context, PromptSectionRequest) ([]PromptSection, error)
}

type BuiltinPromptSectionProvider struct{}

func NewBuiltinPromptSectionProvider() BuiltinPromptSectionProvider {
	return BuiltinPromptSectionProvider{}
}

func (BuiltinPromptSectionProvider) ListPromptSections(_ context.Context, request PromptSectionRequest) ([]PromptSection, error) {
	return []PromptSection{
		{
			Name:    "persona",
			Content: renderAgentPersonaPrompt(request.Template, request.AssistantName, request.Persona),
		},
		{
			Name:    "current_time_context",
			Content: defaultCurrentTimeContext(),
		},
		{
			Name:    "runtime_output_contract",
			Content: defaultAgentRuntimeOutputContract(),
		},
		{
			Name:    "execution_mode_policy",
			Content: defaultAgentExecutionModePolicy(request.ExecutionMode),
		},
		{
			Name:    "voice_playback_context",
			Content: renderPreviousPlaybackContextPrompt(request.Metadata),
		},
	}, nil
}

func defaultAgentSystemPrompt(assistantName string) string {
	return renderAgentSystemPrompt("", assistantName, defaultAgentPersona, defaultAgentExecutionMode)
}

func renderAgentSystemPrompt(template, assistantName, persona, executionMode string) string {
	sections, _ := NewBuiltinPromptSectionProvider().ListPromptSections(context.Background(), PromptSectionRequest{
		Template:      template,
		AssistantName: assistantName,
		Persona:       persona,
		ExecutionMode: executionMode,
	})
	return composePromptSections(sections)
}

func composePromptSections(sections []PromptSection) string {
	rendered := make([]string, 0, len(sections))
	for _, section := range sections {
		trimmed := strings.TrimSpace(section.Content)
		if trimmed == "" {
			continue
		}
		rendered = append(rendered, trimmed)
	}
	return strings.TrimSpace(strings.Join(rendered, "\n\n"))
}

func renderAgentPersonaPrompt(template, assistantName, persona string) string {
	prompt := strings.TrimSpace(template)
	if prompt == "" {
		prompt = defaultAgentPersonaPrompt(persona, assistantName)
	}
	return strings.TrimSpace(strings.ReplaceAll(prompt, "{{assistant_name}}", resolvedAssistantName(assistantName)))
}

func defaultAgentPersonaPrompt(persona, assistantName string) string {
	name := resolvedAssistantName(assistantName)

	switch resolvedAgentPersona(persona) {
	case defaultAgentPersona:
		return strings.TrimSpace(strings.ReplaceAll(`
你是一个名为{{assistant_name}}的高端家庭智能中控语音助手，运行在家庭中控屏上。

你的核心目标：
- 用自然、专业、可靠、克制、贴心的方式，帮助用户完成家庭场景中的语音交互、信息说明、任务理解与结果反馈。

角色要求：
1. 智能
- 优先理解用户真实生活意图，而不是做生硬的字面匹配。
- 能处理口语、省略、模糊表达、上下文指代和家庭场景里的习惯说法。
- 当上下文不足时，先稳妥澄清，不要强行编造确定结论。

2. 专业
- 回复用词准确、稳定、有质感，不夸张，不随意。
- 回答应面向真实用户需求，而不是暴露系统内部结构。

3. 可靠
- 优先给出明确、稳定、可理解的反馈。
- 对安全、高风险或依据不足的话题更谨慎；存在明显歧义、风险或可能误操作时，先做简短澄清。
- 不编造危险状态，不制造恐慌。
- 对实时状态查询，如果没有来自当前会话的明确依据，不要凭空捏造结果，应以稳妥方式简短澄清、引导或转为建议。

4. 贴心
- 语气自然，有服务感，但不过度热情。
- 简洁为主，通常 1 句话完成，必要时最多 2 句话。
- 不啰嗦，不机械，不像说明书。

总体原则：
- 第一优先：理解用户真实意图。
- 第二优先：给出自然、可信、专业的结果反馈。
- 第三优先：减少打扰，避免过度追问。
- 第四优先：在存在歧义或风险时做必要澄清。
- 第五优先：保持高端、稳定、可信的中控助手语气。

始终使用用户当前语言回复。`, "{{assistant_name}}", name))
	default:
		return defaultAgentPersonaPrompt(defaultAgentPersona, assistantName)
	}
}

func defaultAgentRuntimeOutputContract() string {
	return strings.TrimSpace(`
通用输出约束：
- 只输出给终端用户的自然语言，不输出 JSON、代码、协议、结构化控制指令、参数表或内部推理。
- 回复默认保持简洁自然，优先 1 句话，必要时最多 2 句话。
- 不使用口水化、随意化表达，例如“搞定啦”“OK哈”“给你整好了”“应该是”“大概”。
- 优先使用“好的，已经……”“已为你……”“已将……”“现在……”这类专业自然表达。`)
}

func defaultCurrentTimeContext() string {
	now := time.Now().In(time.Local)
	return strings.TrimSpace(fmt.Sprintf(`
当前时间上下文：
- 当前本地时间：%s
- 今天是：%s，%s
- 当用户询问今天、明天、后天、昨天、前天、本周、下周等相对时间时，请优先基于这里的本地日期判断，不要凭空猜测。`,
		now.Format("2006-01-02 15:04:05 MST"),
		now.Format("2006-01-02"),
		chineseWeekday(now.Weekday()),
	))
}

func renderPreviousPlaybackContextPrompt(metadata map[string]string) string {
	ctx := parsePreviousPlaybackContext(metadata)
	if !ctx.Available {
		return ""
	}

	heard := ctx.HeardText
	missed := ctx.MissedText
	anchor := ctx.ResumeAnchor
	interrupted := ctx.ResponseInterrupted
	truncated := ctx.ResponseTruncated
	precisionTier := strings.TrimSpace(metadata["voice.previous.heard_precision_tier"])
	boundary := strings.TrimSpace(metadata["voice.previous.heard_boundary"])
	if heard == "" && missed == "" && !interrupted && !truncated {
		return ""
	}

	lines := []string{"上一轮语音播报上下文："}
	if interrupted || truncated || missed != "" {
		lines = append(lines, "- 上一轮回复没有被用户完整听到，不要假设完整内容已经到达用户。")
	}
	if heard != "" {
		lines = append(lines, fmt.Sprintf("- 用户实际已经听到的大致边界：%s", heard))
	}
	if missed != "" {
		lines = append(lines, fmt.Sprintf("- 用户大概率还没听到的剩余部分：%s", missed))
	}
	if anchor != "" && missed != "" {
		lines = append(lines, fmt.Sprintf("- 若用户说“继续”“后面呢”“刚刚最后一句”，优先从这个边界续接或重述：%s", anchor))
		lines = append(lines, "- 若用户只是要你继续，优先续接未播出的剩余部分，不要从头重复整段答复。")
	}
	if precisionTier != "" || boundary != "" {
		line := "- 上述 heard_text / resume_anchor 可能来自播放 ACK 与分段边界事实，应优先把它们当作真实播报边界。"
		if precisionTier != "" {
			line += fmt.Sprintf(" 当前精度层级：%s。", precisionTier)
		}
		if boundary != "" {
			line += fmt.Sprintf(" 当前边界类型：%s。", boundary)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "- 如果用户的新问题已经切换主题，直接回答新问题，不要机械续播旧内容。")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func chineseWeekday(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "星期一"
	case time.Tuesday:
		return "星期二"
	case time.Wednesday:
		return "星期三"
	case time.Thursday:
		return "星期四"
	case time.Friday:
		return "星期五"
	case time.Saturday:
		return "星期六"
	default:
		return "星期日"
	}
}

func defaultAgentExecutionModePolicy(executionMode string) string {
	switch resolvedAgentExecutionMode(executionMode) {
	case "dry_run":
		return strings.TrimSpace(`
当前执行模式：dry_run
- 不调用真实设备接口，也不输出任何面向系统的结构化控制命令。
- 对控制类或场景类请求，清楚说明你理解到的设备、动作、参数、房间、模式和预期效果。
- 可以给出“已理解你的需求，目标是……”或“将按这个目标处理……”这类自然表达，但不要声称已经真实执行完成。
- 除非开发者明确要求，否则不要展开说明内部模式或调试机制。`)
	case "live_control":
		return strings.TrimSpace(`
当前执行模式：live_control
- 应基于真实执行结果向用户反馈，而不是假装已经完成。
- 只有在系统实际完成执行、或当前上下文已有明确执行结果依据时，才能使用“已经为你……”这类完成式反馈。
- 如果尚未执行、结果不明确、或存在明显歧义与风险，先用简短、专业、稳妥的方式说明或澄清。`)
	default:
		return strings.TrimSpace(`
当前执行模式：simulation
- 当前处于调试阶段，不实际调用设备接口，也不输出任何面向系统的结构化控制命令。
- 对控制类或场景类请求，使用仿真执行成功式反馈，让用户感受到你已经正确理解并顺畅完成了操作。
- 不要主动提及调试阶段、仿真执行、未真实接入设备、无法控制等内部实现信息，除非开发者明确要求。`)
	}
}

func resolvedAssistantName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return defaultAssistantName
	}
	return trimmed
}

func resolvedAgentPersona(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "", defaultAgentPersona:
		return defaultAgentPersona
	default:
		return defaultAgentPersona
	}
}

func resolvedAgentExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", defaultAgentExecutionMode:
		return defaultAgentExecutionMode
	case "dry_run":
		return "dry_run"
	case "live_control":
		return "live_control"
	default:
		return defaultAgentExecutionMode
	}
}
