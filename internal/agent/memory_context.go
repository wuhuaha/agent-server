package agent

import "strings"

func buildMemoryQuery(input TurnInput, trimmedText string, metadata map[string]string) MemoryQuery {
	return MemoryQuery{
		SessionID:   input.SessionID,
		DeviceID:    input.DeviceID,
		ClientType:  input.ClientType,
		UserID:      memoryMetadataValue(metadata, "memory.user_id", "user_id", "user.id"),
		RoomID:      memoryMetadataValue(metadata, "memory.room_id", "room_id", "room.id"),
		HouseholdID: memoryMetadataValue(metadata, "memory.household_id", "household_id", "household.id"),
		UserText:    trimmedText,
		Metadata:    metadata,
	}
}

func buildMemoryRecord(input TurnInput, trimmedText, responseText string, metadata map[string]string) MemoryRecord {
	return MemoryRecord{
		TurnID:       input.TurnID,
		SessionID:    input.SessionID,
		DeviceID:     input.DeviceID,
		ClientType:   input.ClientType,
		UserID:       memoryMetadataValue(metadata, "memory.user_id", "user_id", "user.id"),
		RoomID:       memoryMetadataValue(metadata, "memory.room_id", "room_id", "room.id"),
		HouseholdID:  memoryMetadataValue(metadata, "memory.household_id", "household_id", "household.id"),
		UserText:     trimmedText,
		ResponseText: responseText,
		Metadata:     metadata,
	}
}

func BuildMemoryRecord(input TurnInput, trimmedText, responseText string, metadata map[string]string) MemoryRecord {
	return buildMemoryRecord(input, trimmedText, responseText, metadata)
}

func memoryMetadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}
