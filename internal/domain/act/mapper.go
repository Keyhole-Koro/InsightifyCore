package act

import (
	"strings"

	"google.golang.org/protobuf/proto"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// NormalizeActState applies minimal normalization for act payload from proto.
func NormalizeActState(in *insightifyv1.UiActState) *insightifyv1.UiActState {
	if in == nil {
		return nil
	}
	out := proto.Clone(in).(*insightifyv1.UiActState)
	out.ActId = strings.TrimSpace(out.GetActId())
	out.Mode = strings.TrimSpace(out.GetMode())
	out.Goal = strings.TrimSpace(out.GetGoal())
	out.SelectedWorker = strings.TrimSpace(out.GetSelectedWorker())
	return out
}
