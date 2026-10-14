package rotation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type fakeAPI struct {
	calls []string
}

func (f *fakeAPI) CreateTemporaryKey(context.Context, string, string, string) (json.RawMessage, error) {
	f.calls = append(f.calls, "create")
	return json.RawMessage(`{"id":"temp-42","key":"initial-secret"}`), nil
}
func (f *fakeAPI) RotateKey(_ context.Context, id string, grace int, _ string) (json.RawMessage, error) {
	f.calls = append(f.calls, "rotate:"+id)
	return json.RawMessage(`{"id":"temp-42","key":"rotated-secret"}`), nil
}
func (f *fakeAPI) SearchLogs(context.Context) (json.RawMessage, error) {
	f.calls = append(f.calls, "search")
	return json.RawMessage(`{"deployments":["api-green","worker-blue"]}`), nil
}
func (f *fakeAPI) RevokeKey(_ context.Context, id string) (json.RawMessage, error) {
	f.calls = append(f.calls, "revoke:"+id)
	return json.RawMessage(`{"revoked":true}`), nil
}

func TestDrillBusinessSequence(t *testing.T) {
	tests := []struct {
		name      string
		input     DrillInput
		wantCalls []string
		wantKey   string
		wantErr   bool
	}{
		{
			name: "rotates only the temporary key and reads diagnostics before cleanup",
			input: DrillInput{
				ProjectID: "analytics", Name: "etl-rotation-drill", GraceHours: 24,
				Build: BuildEvent{BuildID: "build-417", Deployment: "worker-blue"},
			},
			wantCalls: []string{"create", "rotate:temp-42", "search", "revoke:temp-42"},
			wantKey:   "rotated-secret",
		},
		{
			name:    "rejects a run without overlap",
			input:   DrillInput{Name: "etl-rotation-drill", GraceHours: 0},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			got, err := NewDrill(api).Run(context.Background(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(api.calls, tt.wantCalls) {
				t.Fatalf("call order = %v, want %v", api.calls, tt.wantCalls)
			}
			if got.NewKey != tt.wantKey {
				t.Fatalf("new key = %q, want %q", got.NewKey, tt.wantKey)
			}
			if !tt.wantErr && got.Release.BuildID != "build-417" {
				t.Fatalf("release build = %q, want build-417", got.Release.BuildID)
			}
		})
	}
}
