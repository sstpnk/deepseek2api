package history

import (
	"context"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type currentInputTestStore struct {
	enabled  bool
	minChars int
}

func (s currentInputTestStore) CurrentInputFileEnabled() bool { return s.enabled }
func (s currentInputTestStore) CurrentInputFileMinChars() int { return s.minChars }

type currentInputTestUploader struct {
	uploads []dsclient.UploadFileRequest
}

func (u *currentInputTestUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	u.uploads = append(u.uploads, req)
	return &dsclient.UploadFileResult{ID: "file-test"}, nil
}

func TestApplyCurrentInputFileSkipsOrdinaryShortInputByThreshold(t *testing.T) {
	uploader := &currentInputTestUploader{}
	req := promptcompat.StandardRequest{
		Surface:       "openai_chat",
		ResolvedModel: "deepseek-v4-flash",
		Messages: []any{
			map[string]any{"role": "user", "content": "short"},
		},
		LatestUserInputText: "short",
	}

	out, err := (Service{
		Store: currentInputTestStore{enabled: true, minChars: 12},
		DS:    uploader,
	}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, req)
	if err != nil {
		t.Fatalf("ApplyCurrentInputFile error: %v", err)
	}
	if out.CurrentInputFileApplied {
		t.Fatalf("expected short ordinary input to skip current input file")
	}
	if len(uploader.uploads) != 0 {
		t.Fatalf("expected no upload, got %d", len(uploader.uploads))
	}
}

func TestApplyCurrentInputFileRoleplayIgnoresThreshold(t *testing.T) {
	uploader := &currentInputTestUploader{}
	req := promptcompat.StandardRequest{
		Surface:       "openai_chat",
		ResolvedModel: "deepseek-v4-pro-rp",
		Messages: []any{
			map[string]any{"role": "user", "content": "short"},
		},
		LatestUserInputText: "short",
	}

	out, err := (Service{
		Store: currentInputTestStore{enabled: true, minChars: 12},
		DS:    uploader,
	}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, req)
	if err != nil {
		t.Fatalf("ApplyCurrentInputFile error: %v", err)
	}
	if !out.CurrentInputFileApplied {
		t.Fatalf("expected roleplay input to apply current input file")
	}
	if len(uploader.uploads) != 1 {
		t.Fatalf("expected one upload, got %d", len(uploader.uploads))
	}
}
