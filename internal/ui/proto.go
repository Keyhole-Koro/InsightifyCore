package ui

import (
	insightifyv1 "insightify/gen/go/insightify/v1"
)

func ToProtoNode(node Node) *insightifyv1.UiNode {
	out := &insightifyv1.UiNode{
		Id:   node.ID,
		Type: toProtoNodeType(node.Type),
		Meta: &insightifyv1.UiNodeMeta{
			Title:       node.Meta.Title,
			Description: node.Meta.Description,
			Tags:        append([]string(nil), node.Meta.Tags...),
		},
	}
	if node.LLMChat != nil {
		out.LlmChat = &insightifyv1.UiLlmChatState{
			Model:        node.LLMChat.Model,
			IsResponding: node.LLMChat.IsResponding,
			SendLocked:   node.LLMChat.SendLocked,
			SendLockHint: node.LLMChat.SendLockHint,
			Messages:     toProtoMessages(node.LLMChat.Messages),
		}
	}
	if node.Markdown != nil {
		out.Markdown = &insightifyv1.UiMarkdownState{
			Markdown: node.Markdown.Markdown,
		}
	}
	if node.Image != nil {
		out.Image = &insightifyv1.UiImageState{
			Src: node.Image.Src,
			Alt: node.Image.Alt,
		}
	}
	if node.Table != nil {
		out.Table = &insightifyv1.UiTableState{
			Columns: append([]string(nil), node.Table.Columns...),
			Rows:    toProtoTableRows(node.Table.Rows),
		}
	}
	return out
}
