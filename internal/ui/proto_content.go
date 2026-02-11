package ui

import insightifyv1 "insightify/gen/go/insightify/v1"

func toProtoMessages(messages []ChatMessage) []*insightifyv1.UiChatMessage {
	out := make([]*insightifyv1.UiChatMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, &insightifyv1.UiChatMessage{
			Id:      m.ID,
			Role:    toProtoMessageRole(m.Role),
			Content: m.Content,
		})
	}
	return out
}

func toProtoTableRows(rows [][]string) []*insightifyv1.UiTableRow {
	out := make([]*insightifyv1.UiTableRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &insightifyv1.UiTableRow{
			Cells: append([]string(nil), row...),
		})
	}
	return out
}
