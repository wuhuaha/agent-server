package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	builtinSkillHouseholdControl       = "household_control"
	householdControlSimulationToolName = "home.control.simulate"
)

type householdControlToolInput struct {
	RoomName         string `json:"room_name"`
	DeviceType       string `json:"device_type"`
	Action           string `json:"action"`
	Mode             string `json:"mode"`
	Target           string `json:"target"`
	Parameter        string `json:"parameter"`
	Value            string `json:"value"`
	SceneName        string `json:"scene_name"`
	Query            string `json:"query"`
	UtteranceSummary string `json:"utterance_summary"`
}

func normalizedBuiltinSkills(skills []string) map[string]struct{} {
	if len(skills) == 0 {
		return nil
	}
	normalized := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		name := canonicalBuiltinSkillName(skill)
		if name == "" {
			continue
		}
		normalized[name] = struct{}{}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func canonicalBuiltinSkillName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case builtinSkillHouseholdControl, "household", "home", "home_control":
		return builtinSkillHouseholdControl
	default:
		return ""
	}
}

func householdControlSkillPrompt() string {
	return fmt.Sprintf(strings.TrimSpace(`
已启用 runtime skill: household_control

- 当用户意图属于家庭设备控制、场景切换或设备状态查询，并且目标已经足够明确时，先调用工具 %s，再根据工具结果组织最终回复。
- 这个 skill 负责家庭控制语义，不应由 transport、voice responder 或 websocket gateway 直接写死规则。
- 可以根据自然口语、模糊表达、参数省略和上下文，把请求归一化为 room_name、device_type、action、mode、target、query 等字段。
- 对门锁、燃气、安防等敏感域，如果目标设备、房间、动作或风险边界仍不清楚，先做一句简短澄清，不要直接假设已经完成。
- 工具返回的是结构化结果，不要把工具名、JSON 或内部字段直接念给用户；最终回复必须仍是自然语言。`), householdControlSimulationToolName)
}

func householdControlToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        householdControlSimulationToolName,
		Description: "Normalize one smart-home control, scene, or state-query request into a structured runtime result for the shared household-control skill.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"room_name": map[string]any{
					"type":        "string",
					"description": "Optional room or zone, such as 客厅 or 主卧.",
				},
				"device_type": map[string]any{
					"type":        "string",
					"description": "Normalized device category, such as light, curtain, aircon, tv, speaker, fan, humidifier, dehumidifier, air_purifier, robot, water_heater, lock, security, gas, or scene.",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Normalized user intent such as on, off, open, close, set, query, increase, decrease, activate.",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Optional device mode such as cooling, heating, sleep, movie, eco, auto.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Optional target value such as 26度, 60%, 暖白, 中档.",
				},
				"parameter": map[string]any{
					"type":        "string",
					"description": "Optional controlled parameter such as temperature, brightness, color_temperature, speed, humidity, volume.",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Optional parameter value when target needs a separate field.",
				},
				"scene_name": map[string]any{
					"type":        "string",
					"description": "Optional normalized scene name such as sleep, movie, away, wakeup.",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Optional query target when the user is asking for state instead of control.",
				},
				"utterance_summary": map[string]any{
					"type":        "string",
					"description": "Optional concise summary of what the user meant.",
				},
			},
			"additionalProperties": false,
		},
	}
}

type HouseholdControlSkill struct{}

func (HouseholdControlSkill) Name() string {
	return builtinSkillHouseholdControl
}

func (HouseholdControlSkill) ListTools(context.Context, ToolCatalogRequest) ([]ToolDefinition, error) {
	return []ToolDefinition{householdControlToolDefinition()}, nil
}

func (HouseholdControlSkill) ListPromptFragments(context.Context, SkillPromptRequest) ([]string, error) {
	return []string{householdControlSkillPrompt()}, nil
}

func (HouseholdControlSkill) InvokeTool(_ context.Context, call ToolCall) (ToolResult, bool, error) {
	if strings.TrimSpace(call.ToolName) != householdControlSimulationToolName {
		return ToolResult{}, false, nil
	}
	input, err := parseHouseholdControlToolInput(call.ToolInput)
	if err != nil {
		return ToolResult{
			CallID:     call.CallID,
			ToolName:   call.ToolName,
			ToolStatus: "failed",
			ToolOutput: encodeToolOutput(map[string]any{"error": err.Error()}),
		}, true, nil
	}

	result := householdControlToolResult(input)
	return ToolResult{
		CallID:     call.CallID,
		ToolName:   call.ToolName,
		ToolStatus: "completed",
		ToolOutput: encodeToolOutput(result),
	}, true, nil
}

func parseHouseholdControlToolInput(raw string) (householdControlToolInput, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return householdControlToolInput{}, nil
	}
	var input householdControlToolInput
	if err := json.Unmarshal([]byte(trimmed), &input); err != nil {
		return householdControlToolInput{}, fmt.Errorf("invalid tool input json: %w", err)
	}
	input.RoomName = strings.TrimSpace(input.RoomName)
	input.DeviceType = normalizedDeviceType(input.DeviceType)
	input.Action = normalizedHouseholdAction(input.Action)
	input.Mode = strings.TrimSpace(input.Mode)
	input.Target = strings.TrimSpace(input.Target)
	input.Parameter = strings.TrimSpace(input.Parameter)
	input.Value = strings.TrimSpace(input.Value)
	input.SceneName = strings.TrimSpace(input.SceneName)
	input.Query = strings.TrimSpace(input.Query)
	input.UtteranceSummary = strings.TrimSpace(input.UtteranceSummary)
	return input, nil
}

func householdControlToolResult(input householdControlToolInput) map[string]any {
	deviceType := input.DeviceType
	if deviceType == "" && input.SceneName != "" {
		deviceType = "scene"
	}
	result := map[string]any{
		"skill":                  builtinSkillHouseholdControl,
		"room_name":              input.RoomName,
		"device_type":            deviceType,
		"action":                 input.Action,
		"mode":                   input.Mode,
		"target":                 coalesceNonEmpty(input.Target, input.Value),
		"parameter":              input.Parameter,
		"scene_name":             input.SceneName,
		"query":                  input.Query,
		"utterance_summary":      input.UtteranceSummary,
		"sensitive_domain":       isSensitiveHouseholdDomain(deviceType),
		"requires_clarification": false,
		"goal":                   householdControlGoal(input),
	}

	if isSensitiveHouseholdDomain(deviceType) {
		result["requires_clarification"] = true
		result["clarification_hint"] = "这是敏感设备，请先确认具体设备、位置和动作。"
		return result
	}

	if strings.TrimSpace(result["goal"].(string)) == "" {
		result["requires_clarification"] = true
		result["clarification_hint"] = "当前家居请求还不够明确，请先确认设备、动作或查询目标。"
	}
	return result
}

func householdControlGoal(input householdControlToolInput) string {
	if input.Query != "" {
		scope := householdScope(input.RoomName, input.DeviceType)
		if scope == "" {
			return "查询当前家庭设备状态"
		}
		return fmt.Sprintf("查询%s%s", scope, input.Query)
	}

	deviceType := input.DeviceType
	if deviceType == "" && input.SceneName != "" {
		deviceType = "scene"
	}
	if deviceType == "scene" {
		scene := input.SceneName
		if scene == "" {
			scene = input.Action
		}
		if scene == "" {
			return "切换家庭场景"
		}
		return "切换到" + scene + "场景"
	}

	action := input.Action
	if action == "" {
		action = "set"
	}
	scope := householdScope(input.RoomName, deviceType)
	switch action {
	case "on":
		return "打开" + scope
	case "off":
		return "关闭" + scope
	case "open":
		return "打开" + scope
	case "close":
		return "关闭" + scope
	case "increase":
		if input.Parameter != "" {
			return fmt.Sprintf("调高%s的%s", scope, input.Parameter)
		}
		return "提升" + scope
	case "decrease":
		if input.Parameter != "" {
			return fmt.Sprintf("调低%s的%s", scope, input.Parameter)
		}
		return "降低" + scope
	default:
		target := coalesceNonEmpty(input.Target, input.Value)
		if input.Parameter != "" && target != "" {
			return fmt.Sprintf("将%s的%s设置为%s", scope, input.Parameter, target)
		}
		if input.Mode != "" && target != "" {
			return fmt.Sprintf("将%s设置为%s %s", scope, input.Mode, target)
		}
		if input.Mode != "" {
			return fmt.Sprintf("将%s切换到%s模式", scope, input.Mode)
		}
		if target != "" {
			return fmt.Sprintf("调整%s到%s", scope, target)
		}
		if scope != "" {
			return "调整" + scope
		}
		return ""
	}
}

func householdScope(roomName, deviceType string) string {
	room := strings.TrimSpace(roomName)
	device := normalizedDeviceType(deviceType)
	switch {
	case room != "" && device != "":
		return room + householdDeviceLabel(device)
	case room != "":
		return room + "设备"
	case device != "":
		return householdDeviceLabel(device)
	default:
		return ""
	}
}

func householdDeviceLabel(deviceType string) string {
	switch normalizedDeviceType(deviceType) {
	case "light":
		return "灯光"
	case "curtain":
		return "窗帘"
	case "aircon":
		return "空调"
	case "tv":
		return "电视"
	case "speaker":
		return "音响"
	case "fan":
		return "风扇"
	case "humidifier":
		return "加湿器"
	case "dehumidifier":
		return "除湿机"
	case "air_purifier":
		return "空气净化器"
	case "fresh_air":
		return "新风"
	case "floor_heating":
		return "地暖"
	case "water_heater":
		return "热水器"
	case "robot":
		return "扫地机器人"
	case "lock":
		return "门锁"
	case "security":
		return "安防设备"
	case "gas":
		return "燃气设备"
	default:
		return strings.TrimSpace(deviceType)
	}
}

func normalizedDeviceType(deviceType string) string {
	switch strings.ToLower(strings.TrimSpace(deviceType)) {
	case "light", "lights", "lamp":
		return "light"
	case "curtain", "curtains":
		return "curtain"
	case "aircon", "air_conditioner", "ac":
		return "aircon"
	case "tv", "television":
		return "tv"
	case "speaker", "audio", "soundbar":
		return "speaker"
	case "fan":
		return "fan"
	case "humidifier":
		return "humidifier"
	case "dehumidifier":
		return "dehumidifier"
	case "air_purifier", "purifier":
		return "air_purifier"
	case "fresh_air", "freshair":
		return "fresh_air"
	case "floor_heating", "heating_floor":
		return "floor_heating"
	case "water_heater":
		return "water_heater"
	case "robot", "robot_vacuum", "vacuum":
		return "robot"
	case "scene":
		return "scene"
	case "lock", "door_lock":
		return "lock"
	case "security", "alarm":
		return "security"
	case "gas":
		return "gas"
	default:
		return strings.TrimSpace(deviceType)
	}
}

func normalizedHouseholdAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "turn_on", "enable", "start":
		return "on"
	case "turn_off", "disable", "stop":
		return "off"
	case "activate", "run":
		return "activate"
	case "increase", "up":
		return "increase"
	case "decrease", "down":
		return "decrease"
	default:
		return strings.ToLower(strings.TrimSpace(action))
	}
}

func isSensitiveHouseholdDomain(deviceType string) bool {
	switch normalizedDeviceType(deviceType) {
	case "lock", "security", "gas":
		return true
	default:
		return false
	}
}

func coalesceNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
