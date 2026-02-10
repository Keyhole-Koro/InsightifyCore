package main

import (
	"context"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func (s *apiServer) NeedUserInput(_ context.Context, req *connect.Request[insightifyv1.SubmitRunInputRequest]) (*connect.Response[insightifyv1.SubmitRunInputResponse], error) {
	sessionID, runID, userInput, err := prepareNeedUserInput(req)
	if err != nil {
		return nil, err
	}

	_, err = submitPendingUserInput(sessionID, runID, userInput)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&insightifyv1.SubmitRunInputResponse{RunId: runID}), nil
}
