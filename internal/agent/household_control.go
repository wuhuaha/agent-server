package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var temperaturePattern = regexp.MustCompile(`([1-3]?\d)\s*度`)

type householdIntent struct {
	RoomName   string
	DeviceType string
	Action     string
	Mode       string
	Target     string
	Sensitive  bool
	Clarify    string
}

func deterministicHouseholdTurn(input TurnInput, executionMode string) (TurnOutput, bool) {
	intent, ok := parseHouseholdIntent(input)
	if !ok {
		return TurnOutput{}, false
	}

	text := renderHouseholdIntentReply(intent, executionMode)
	if strings.TrimSpace(text) == "" {
		return TurnOutput{}, false
	}
	return TurnOutput{Text: text}, true
}

func parseHouseholdIntent(input TurnInput) (householdIntent, bool) {
	text := strings.TrimSpace(input.UserText)
	if text == "" {
		return householdIntent{}, false
	}
	intent := householdIntent{
		RoomName: inferredRoomName(text, input.Metadata),
	}

	if clarify, ok := parseSensitiveIntent(text, intent.RoomName); ok {
		intent.DeviceType = "sensitive"
		intent.Sensitive = true
		intent.Clarify = clarify
		return intent, true
	}
	if parsed, ok := parseLightIntent(text, intent.RoomName); ok {
		return parsed, true
	}
	if parsed, ok := parseCurtainIntent(text, intent.RoomName); ok {
		return parsed, true
	}
	if parsed, ok := parseAirconIntent(text, intent.RoomName); ok {
		return parsed, true
	}
	if parsed, ok := parseSceneIntent(text, intent.RoomName); ok {
		return parsed, true
	}
	return householdIntent{}, false
}

func parseSensitiveIntent(text, roomName string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(normalized, "门锁"), strings.Contains(normalized, "门禁"):
		return "这是门锁相关操作，请先确认你要处理的是哪一扇门。", true
	case strings.Contains(normalized, "燃气"):
		return "这是燃气相关操作，请先确认要处理的设备和当前位置。", true
	case strings.Contains(normalized, "安防"), strings.Contains(normalized, "报警"):
		return "这是安防相关操作，请先确认你要调整的是哪一组安防设备。", true
	default:
		return "", false
	}
}

func parseLightIntent(text, roomName string) (householdIntent, bool) {
	if !containsAny(text, "灯", "灯光", "主灯", "吊灯", "台灯") && !containsAny(text, "有点暗", "亮一点", "暗一点", "调亮", "调暗") {
		return householdIntent{}, false
	}
	intent := householdIntent{RoomName: roomName, DeviceType: "light"}
	switch {
	case containsAny(text, "关闭", "关掉", "把灯关", "熄灯"):
		intent.Action = "off"
	case containsAny(text, "调亮", "亮一点", "有点暗"):
		intent.Action = "brighten"
	case containsAny(text, "调暗", "暗一点", "太亮", "刺眼"):
		intent.Action = "dim"
	default:
		intent.Action = "on"
	}
	return intent, true
}

func parseCurtainIntent(text, roomName string) (householdIntent, bool) {
	if !containsAny(text, "窗帘") {
		return householdIntent{}, false
	}
	intent := householdIntent{RoomName: roomName, DeviceType: "curtain"}
	switch {
	case containsAny(text, "打开", "拉开"):
		intent.Action = "open"
	default:
		intent.Action = "close"
	}
	return intent, true
}

func parseAirconIntent(text, roomName string) (householdIntent, bool) {
	if !containsAny(text, "空调", "太热", "太冷") && !temperaturePattern.MatchString(text) {
		return householdIntent{}, false
	}
	intent := householdIntent{RoomName: roomName, DeviceType: "aircon"}
	switch {
	case containsAny(text, "关闭空调", "关空调"):
		intent.Action = "off"
	case containsAny(text, "打开空调", "开空调"):
		intent.Action = "on"
	default:
		intent.Action = "set"
	}
	switch {
	case containsAny(text, "制冷", "太热"):
		intent.Mode = "cooling"
	case containsAny(text, "制热", "太冷"):
		intent.Mode = "heating"
	}
	if matches := temperaturePattern.FindStringSubmatch(text); len(matches) == 2 {
		if temp, err := strconv.Atoi(matches[1]); err == nil {
			intent.Target = strconv.Itoa(temp)
		}
	}
	return intent, true
}

func parseSceneIntent(text, roomName string) (householdIntent, bool) {
	switch {
	case containsAny(text, "睡觉了", "睡眠模式", "晚安"):
		return householdIntent{RoomName: roomName, DeviceType: "scene", Action: "sleep"}, true
	case containsAny(text, "观影", "影院模式", "看电影"):
		return householdIntent{RoomName: roomName, DeviceType: "scene", Action: "movie"}, true
	default:
		return householdIntent{}, false
	}
}

func renderHouseholdIntentReply(intent householdIntent, executionMode string) string {
	if intent.Sensitive {
		return intent.Clarify
	}

	if resolvedAgentExecutionMode(executionMode) == "dry_run" {
		return "已理解你的需求：" + householdIntentGoal(intent) + "。"
	}
	if resolvedAgentExecutionMode(executionMode) == "live_control" {
		return "已收到你的需求，目标是" + householdIntentGoal(intent) + "。"
	}

	switch intent.DeviceType {
	case "light":
		switch intent.Action {
		case "off":
			return fmt.Sprintf("好的，已经把%s灯关掉了。", roomPrefix(intent.RoomName))
		case "brighten":
			return fmt.Sprintf("已为你把%s灯光调亮一些。", roomPrefix(intent.RoomName))
		case "dim":
			return fmt.Sprintf("已为你把%s灯光调暗一些。", roomPrefix(intent.RoomName))
		default:
			return fmt.Sprintf("好的，已经把%s灯打开了。", roomPrefix(intent.RoomName))
		}
	case "curtain":
		if intent.Action == "open" {
			return fmt.Sprintf("好的，%s窗帘已经打开。", roomPrefix(intent.RoomName))
		}
		return fmt.Sprintf("好的，%s窗帘已经关闭。", roomPrefix(intent.RoomName))
	case "aircon":
		switch intent.Action {
		case "off":
			return fmt.Sprintf("好的，已经把%s空调关闭。", roomPrefix(intent.RoomName))
		case "on":
			return fmt.Sprintf("好的，已经把%s空调打开。", roomPrefix(intent.RoomName))
		default:
			modeText := ""
			switch intent.Mode {
			case "cooling":
				modeText = "制冷 "
			case "heating":
				modeText = "制热 "
			}
			if intent.Target != "" {
				return fmt.Sprintf("好的，已经把%s空调调到%s%s度。", roomPrefix(intent.RoomName), modeText, intent.Target)
			}
			if modeText != "" {
				return fmt.Sprintf("好的，已经把%s空调切到%s模式。", roomPrefix(intent.RoomName), strings.TrimSpace(modeText))
			}
			return fmt.Sprintf("好的，已经按你的要求调整%s空调。", roomPrefix(intent.RoomName))
		}
	case "scene":
		switch intent.Action {
		case "sleep":
			return "已为你切到睡眠场景，灯光会更柔和，窗帘和空调也已同步到更适合休息的状态。"
		case "movie":
			return "已为你切到观影氛围，灯光会更柔和一些。"
		}
	}
	return ""
}

func householdIntentGoal(intent householdIntent) string {
	switch intent.DeviceType {
	case "light":
		switch intent.Action {
		case "off":
			return fmt.Sprintf("把%s灯关闭", roomPrefix(intent.RoomName))
		case "brighten":
			return fmt.Sprintf("把%s灯光调亮一些", roomPrefix(intent.RoomName))
		case "dim":
			return fmt.Sprintf("把%s灯光调暗一些", roomPrefix(intent.RoomName))
		default:
			return fmt.Sprintf("把%s灯打开", roomPrefix(intent.RoomName))
		}
	case "curtain":
		if intent.Action == "open" {
			return fmt.Sprintf("把%s窗帘打开", roomPrefix(intent.RoomName))
		}
		return fmt.Sprintf("把%s窗帘关闭", roomPrefix(intent.RoomName))
	case "aircon":
		if intent.Action == "off" {
			return fmt.Sprintf("把%s空调关闭", roomPrefix(intent.RoomName))
		}
		if intent.Action == "on" {
			return fmt.Sprintf("把%s空调打开", roomPrefix(intent.RoomName))
		}
		goal := fmt.Sprintf("调整%s空调", roomPrefix(intent.RoomName))
		if intent.Mode != "" {
			goal += "到" + map[string]string{"cooling": "制冷", "heating": "制热"}[intent.Mode] + "模式"
		}
		if intent.Target != "" {
			goal += "并设为" + intent.Target + "度"
		}
		return goal
	case "scene":
		if intent.Action == "sleep" {
			return "切到睡眠场景"
		}
		if intent.Action == "movie" {
			return "切到观影场景"
		}
	}
	return "处理当前家居控制需求"
}

func inferredRoomName(text string, metadata map[string]string) string {
	rooms := []string{"客厅", "主卧", "次卧", "卧室", "餐厅", "书房", "厨房", "卫生间", "阳台", "儿童房", "老人房", "玄关"}
	for _, room := range rooms {
		if strings.Contains(text, room) {
			return room
		}
	}
	for _, key := range []string{"room_name", "room", "memory.room_name"} {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"memory.room_id", "room_id"} {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func roomPrefix(roomName string) string {
	trimmed := strings.TrimSpace(roomName)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
