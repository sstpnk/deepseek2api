package shared

import (
	"context"
	"net/http"

	"ds2api/internal/auth"
)

type emptyOutputProxyAvoider interface {
	AvoidProxyForResponse(context.Context, *http.Response, *auth.RequestAuth, string) context.Context
}

type accountEmptyOutputRecorder interface {
	RecordAccountEmptyOutput(*auth.RequestAuth, string)
}

type accountVisibleSuccessRecorder interface {
	RecordAccountVisibleSuccess(*auth.RequestAuth, string)
}

func AvoidProxyForEmptyOutputRetry(ds any, ctx context.Context, resp *http.Response, a *auth.RequestAuth, reason string) context.Context {
	if avoider, ok := ds.(emptyOutputProxyAvoider); ok {
		return avoider.AvoidProxyForResponse(ctx, resp, a, reason)
	}
	return ctx
}

func RecordAccountEmptyOutput(ds any, a *auth.RequestAuth, reason string) {
	if recorder, ok := ds.(accountEmptyOutputRecorder); ok {
		recorder.RecordAccountEmptyOutput(a, reason)
	}
}

func RecordAccountVisibleSuccess(ds any, a *auth.RequestAuth, reason string) {
	if recorder, ok := ds.(accountVisibleSuccessRecorder); ok {
		recorder.RecordAccountVisibleSuccess(a, reason)
	}
}
