package rotation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type KeyAPI interface {
	CreateTemporaryKey(context.Context, string, string, string) (json.RawMessage, error)
	RotateKey(context.Context, string, int, string) (json.RawMessage, error)
	SearchLogs(context.Context) (json.RawMessage, error)
	RevokeKey(context.Context, string) (json.RawMessage, error)
}

type DrillInput struct {
	ProjectID  string     `json:"project_id"`
	Name       string     `json:"name"`
	GraceHours int        `json:"grace_hours"`
	Build      BuildEvent `json:"build"`
}

type BuildEvent struct {
	BuildID    string `json:"build_id"`
	Deployment string `json:"deployment"`
}

type DrillResult struct {
	TemporaryKeyID string           `json:"temporary_key_id"`
	NewKey         string           `json:"new_key"`
	Diagnostics    json.RawMessage  `json:"diagnostics"`
	Release        ReleaseOperation `json:"release"`
}

type ReleaseOperation struct {
	BuildID    string `json:"build_id"`
	Deployment string `json:"deployment"`
	Decision   string `json:"decision"`
}

type Drill struct {
	api KeyAPI
}

func NewDrill(api KeyAPI) *Drill { return &Drill{api: api} }

func (d *Drill) Run(ctx context.Context, input DrillInput) (DrillResult, error) {
	if input.Name == "" || input.GraceHours < 1 {
		return DrillResult{}, errors.New("name and a positive grace_hours are required")
	}
	runID, err := randomID()
	if err != nil {
		return DrillResult{}, err
	}
	created, err := d.api.CreateTemporaryKey(ctx, input.ProjectID, input.Name, runID+"-create")
	if err != nil {
		return DrillResult{}, err
	}
	keyID, _, err := keyMaterial(created)
	if err != nil {
		return DrillResult{}, fmt.Errorf("read temporary key: %w", err)
	}

	rotated, err := d.api.RotateKey(ctx, keyID, input.GraceHours, runID+"-rotate")
	if err != nil {
		return DrillResult{}, err
	}
	_, newKey, err := keyMaterial(rotated)
	if err != nil {
		return DrillResult{}, fmt.Errorf("read rotated key: %w", err)
	}
	diagnostics, err := d.api.SearchLogs(ctx)
	if err != nil {
		return DrillResult{}, err
	}
	if _, err := d.api.RevokeKey(ctx, keyID); err != nil {
		return DrillResult{}, err
	}

	return DrillResult{
		TemporaryKeyID: keyID,
		NewKey:         newKey,
		Diagnostics:    diagnostics,
		Release: ReleaseOperation{
			BuildID:    input.Build.BuildID,
			Deployment: input.Build.Deployment,
			Decision:   "temporary drill complete; schedule the real key after deployment diagnostics are clear",
		},
	}, nil
}

func keyMaterial(raw json.RawMessage) (string, string, error) {
	var value struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", "", err
	}
	if value.ID == "" {
		return "", "", errors.New("response omitted key id")
	}
	return value.ID, value.Key, nil
}

func randomID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("create idempotency key: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
