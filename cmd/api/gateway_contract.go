package main

import "insightify/gen/go/insightify/v1/insightifyv1connect"

// Ensure interface conformance
var _ insightifyv1connect.PipelineServiceHandler = (*apiServer)(nil)
var _ insightifyv1connect.LlmChatServiceHandler = (*apiServer)(nil)
