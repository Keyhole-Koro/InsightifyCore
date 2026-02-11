package ui

import insightifyv1 "insightify/gen/go/insightify/v1"

func toProtoNodeType(nodeType NodeType) insightifyv1.UiNodeType {
	switch nodeType {
	case NodeTypeLLMChat:
		return insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT
	case NodeTypeMarkdown:
		return insightifyv1.UiNodeType_UI_NODE_TYPE_MARKDOWN
	case NodeTypeImage:
		return insightifyv1.UiNodeType_UI_NODE_TYPE_IMAGE
	case NodeTypeTable:
		return insightifyv1.UiNodeType_UI_NODE_TYPE_TABLE
	default:
		return insightifyv1.UiNodeType_UI_NODE_TYPE_UNSPECIFIED
	}
}

func toProtoMessageRole(role MessageRole) insightifyv1.UiChatMessage_Role {
	switch role {
	case RoleUser:
		return insightifyv1.UiChatMessage_ROLE_USER
	case RoleAssistant:
		return insightifyv1.UiChatMessage_ROLE_ASSISTANT
	default:
		return insightifyv1.UiChatMessage_ROLE_UNSPECIFIED
	}
}
