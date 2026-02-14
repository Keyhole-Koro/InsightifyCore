package worker

import (
	workerv1 "insightify/gen/go/worker/v1"
)

func asClientView(v any) *workerv1.ClientView {
	if v == nil {
		return nil
	}
	view, ok := v.(*workerv1.ClientView)
	if !ok {
		return nil
	}
	return view
}
