package client

import (
	"bytes"
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/json"
	"errors"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

func (c *Client) CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	baseCtx := ctx
	headers := c.authHeaders(a.DeepSeekToken)
	headers["x-ds-pow-response"] = powResp
	captureSession := c.capture.Start("deepseek_completion", dsprotocol.DeepSeekCompletionURL, a.AccountID, payload)
	attempts := 0
	for attempts < maxAttempts {
		clients := c.requestClientsForAuth(baseCtx, a)
		ctx = withActiveProxyID(baseCtx, clients.proxyID)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.streamPost(ctx, clients.stream, clients.fallbackS, dsprotocol.DeepSeekCompletionURL, headers, payload)
		if err != nil {
			attempts++
			if attempts >= maxAttempts {
				break
			}
			if waitErr := sleepWithCtx(ctx, retryDelay(attempts-1, true)); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		if resp.StatusCode == http.StatusOK {
			if captureSession != nil {
				resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
			}
			resp = c.wrapCompletionWithAutoContinue(ctx, a, payload, powResp, resp)
			return resp, nil
		}
		if captureSession != nil {
			resp.Body = captureSession.WrapBody(resp.Body, resp.StatusCode)
		}
		statusCode := resp.StatusCode
		body, readErr := readResponseBody(resp)
		if readErr == nil {
			parsed := map[string]any{}
			if len(body) > 0 {
				if err := json.Unmarshal(body, &parsed); err == nil {
					code, bizCode, msg, bizMsg := extractResponseStatus(parsed)
					if isInvalidPowResponse(code, bizCode, msg, bizMsg) {
						c.invalidatePowHeader(a, dsprotocol.DeepSeekCompletionTargetPath)
					}
				}
			}
		}
		if err := resp.Body.Close(); err != nil {
			config.Logger.Warn("[completion] response body close failed", "error", err)
		}
		attempts++
		if attempts >= maxAttempts {
			break
		}
		if waitErr := sleepWithCtx(ctx, retryDelay(attempts-1, statusCode >= 500)); waitErr != nil {
			return nil, waitErr
		}
	}
	return nil, errors.New("completion failed")
}

func (c *Client) streamPost(ctx context.Context, doer trans.Doer, fallback trans.Doer, url string, headers map[string]string, payload any) (*http.Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	headers = c.jsonHeaders(headers)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err != nil {
		if isTransportError(err) {
			c.markProxyFailure(activeProxyIDFromContext(ctx))
		}
		config.Logger.Warn("[deepseek] fingerprint stream request failed, fallback to std transport", "url", url, "error", err)
		req2, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if reqErr != nil {
			return nil, reqErr
		}
		for k, v := range headers {
			req2.Header.Set(k, v)
		}
		return fallback.Do(req2)
	}
	if resp != nil && resp.StatusCode < 500 {
		c.markProxySuccess(activeProxyIDFromContext(ctx))
	}
	return resp, nil
}
