package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ProviderConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Providers     map[string]Command `json:"providers,omitempty"`
	Transcriber   *Command           `json:"transcriber,omitempty"`
}

type Command struct {
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type FeedbackRequest struct {
	SchemaVersion int               `json:"schema_version"`
	SourceDigest  string            `json:"source_digest"`
	Deck          *Deck             `json:"deck"`
	Session       *Session          `json:"session"`
	Evidence      []ProjectEvidence `json:"evidence,omitempty"`
}

type ProjectEvidence struct {
	ProjectID string     `json:"project_id"`
	Name      string     `json:"name"`
	URL       string     `json:"url,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

type feedbackResponse struct {
	SchemaVersion int      `json:"schema_version"`
	Advisory      bool     `json:"advisory"`
	Summary       string   `json:"summary"`
	FollowUps     []string `json:"follow_ups,omitempty"`
	Model         string   `json:"model,omitempty"`
	TokensIn      int      `json:"tokens_in,omitempty"`
	TokensOut     int      `json:"tokens_out,omitempty"`
	CostUSD       float64  `json:"cost_usd,omitempty"`
}

func LoadProviderConfig(path string) (*ProviderConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ProviderConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode coach provider configuration: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode coach provider configuration: trailing JSON data")
	}
	if cfg.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("coach provider schema_version=%d; supported version is %d", cfg.SchemaVersion, SchemaVersion)
	}
	for name, command := range cfg.Providers {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("coach provider name is required")
		}
		if err := validateCommand(command); err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
	}
	if cfg.Transcriber != nil {
		if err := validateCommand(*cfg.Transcriber); err != nil {
			return nil, fmt.Errorf("transcriber: %w", err)
		}
		if !commandHasFilePlaceholder(*cfg.Transcriber) {
			return nil, fmt.Errorf("transcriber argv must contain {file}")
		}
	}
	return &cfg, nil
}

func RunFeedback(ctx context.Context, cfg *ProviderConfig, provider string, bundle *SourceBundle, deck *Deck, session *Session) (*ProviderFeedback, error) {
	if cfg == nil {
		return nil, fmt.Errorf("coach provider configuration is missing")
	}
	command, ok := cfg.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("coach provider %q is not configured", provider)
	}
	request := FeedbackRequest{
		SchemaVersion: SchemaVersion,
		SourceDigest:  bundle.Digest(),
		Deck:          deck,
		Session:       session,
	}
	for _, project := range bundle.Projects {
		if contains(deck.ProjectIDs, project.ID) {
			request.Evidence = append(request.Evidence, ProjectEvidence{
				ProjectID: project.ID, Name: project.Name, URL: project.URL, Evidence: project.Evidence,
			})
		}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	out, err := execute(ctx, command, payload, "")
	if err != nil {
		return nil, err
	}
	var response feedbackResponse
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode coach provider response: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode coach provider response: trailing JSON data")
	}
	if response.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("coach provider response schema_version=%d; supported version is %d", response.SchemaVersion, SchemaVersion)
	}
	if !response.Advisory {
		return nil, fmt.Errorf("coach provider response must set advisory=true")
	}
	if strings.TrimSpace(response.Summary) == "" {
		return nil, fmt.Errorf("coach provider response summary is required")
	}
	response.Summary = strings.TrimSpace(response.Summary)
	response.Model = strings.TrimSpace(response.Model)
	if len(response.Summary) > 4000 || len(response.Model) > 200 || len(response.FollowUps) > 6 || response.TokensIn < 0 || response.TokensOut < 0 || response.CostUSD < 0 {
		return nil, fmt.Errorf("coach provider response exceeds the advisory feedback limits")
	}
	for i := range response.FollowUps {
		response.FollowUps[i] = strings.TrimSpace(response.FollowUps[i])
		if response.FollowUps[i] == "" || len(response.FollowUps[i]) > 500 {
			return nil, fmt.Errorf("coach provider follow-up %d must contain 1 to 500 bytes", i+1)
		}
	}
	return &ProviderFeedback{
		Provider: provider, Model: response.Model, Advisory: true,
		Summary: response.Summary, FollowUps: response.FollowUps,
		TokensIn: response.TokensIn, TokensOut: response.TokensOut, CostUSD: response.CostUSD,
	}, nil
}

func RunTranscriber(ctx context.Context, cfg *ProviderConfig, audioPath string) (string, error) {
	if cfg.Transcriber == nil {
		return "", fmt.Errorf("coach transcriber is not configured")
	}
	if !commandHasFilePlaceholder(*cfg.Transcriber) {
		return "", fmt.Errorf("coach transcriber argv must contain {file}")
	}
	out, err := execute(ctx, *cfg.Transcriber, nil, audioPath)
	if err != nil {
		return "", err
	}
	var response struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(out, &response) == nil && strings.TrimSpace(response.Text) != "" {
		return strings.TrimSpace(response.Text), nil
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("coach transcriber returned no text")
	}
	return text, nil
}

func execute(parent context.Context, command Command, stdin []byte, audioPath string) ([]byte, error) {
	if err := validateCommand(command); err != nil {
		return nil, err
	}
	timeout := time.Duration(command.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	argv := append([]string(nil), command.Argv...)
	for i := range argv {
		argv[i] = strings.ReplaceAll(argv[i], "{file}", audioPath)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(stdin)
	stdout := newLimitedBuffer(4 * 1024 * 1024)
	stderr := newLimitedBuffer(64 * 1024)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if stdout.exceeded {
			return nil, fmt.Errorf("coach adapter output exceeded 4 MiB")
		}
		if stderr.exceeded {
			return nil, fmt.Errorf("coach adapter error output exceeded 64 KiB")
		}
		message := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil {
			return nil, fmt.Errorf("coach adapter timed out after %s", timeout)
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("coach adapter failed: %s", message)
	}
	if stdout.exceeded {
		return nil, fmt.Errorf("coach adapter output exceeded 4 MiB")
	}
	return stdout.Bytes(), nil
}

func validateCommand(command Command) error {
	if len(command.Argv) == 0 || strings.TrimSpace(command.Argv[0]) == "" {
		return fmt.Errorf("argv needs an executable")
	}
	if command.TimeoutSeconds < 0 || command.TimeoutSeconds > 300 {
		return fmt.Errorf("timeout_seconds must be between 0 and 300")
	}
	for _, arg := range command.Argv {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("argv cannot contain a NUL byte")
		}
	}
	return nil
}

func commandHasFilePlaceholder(command Command) bool {
	for _, arg := range command.Argv {
		if strings.Contains(arg, "{file}") {
			return true
		}
	}
	return false
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.exceeded {
		return len(p), fmt.Errorf("output limit exceeded")
	}
	remaining := b.limit - b.Len()
	if len(p) <= remaining {
		return b.Buffer.Write(p)
	}
	if remaining > 0 {
		_, _ = b.Buffer.Write(p[:remaining])
	}
	b.exceeded = true
	return len(p), fmt.Errorf("output limit exceeded")
}
