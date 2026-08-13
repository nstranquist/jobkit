package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/coach"
	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

func cmdCoach(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	switch sub {
	case "source":
		return cmdCoachSource(c)
	case "deck":
		return cmdCoachDeck(c)
	case "run":
		return cmdCoachRun(c)
	case "stats":
		return cmdCoachStats(c)
	case "serve":
		return cmdCoachServe(c)
	case "path":
		store, err := coachStore()
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": store.Root})
		} else {
			fmt.Println(store.Root)
		}
		return nil
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach source|deck|run|stats|serve|path")
	}
}

func coachStore() (*coach.Store, error) {
	root, err := home.Dir()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	store, err := coach.NewStore(filepath.Join(root, "coach"))
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return store, nil
}

func cmdCoachSource(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	action := ""
	if len(c.args) > 2 {
		action = c.args[2]
	}
	switch action {
	case "path":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": store.SourcePath()})
		} else {
			fmt.Println(store.SourcePath())
		}
		return nil
	case "show":
		bundle, err := store.LoadSource()
		if err != nil {
			if os.IsNotExist(err) {
				return envelope.New(envelope.CodeNotFound, "coach source is missing").WithHint("run `jobkit coach source import <public-bundle.json>`")
			}
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": store.SourcePath(), "digest": bundle.Digest(), "source": bundle})
		} else {
			fmt.Printf("%s\n%s\n%d projects, %d stories, %d claims\n", store.SourcePath(), bundle.Digest(), len(bundle.Projects), len(bundle.Stories), len(bundle.Claims))
		}
		return nil
	case "import":
		if len(c.args) < 4 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach source import <public-bundle.json>")
		}
		bundle, err := coach.LoadSource(c.args[3])
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if err := store.SaveSource(bundle); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": store.SourcePath(), "digest": bundle.Digest(), "projects": len(bundle.Projects), "stories": len(bundle.Stories), "claims": len(bundle.Claims)})
		} else {
			fmt.Printf("imported public coach source to %s\n", store.SourcePath())
			fmt.Printf("%d projects, %d stories, %d claims\n", len(bundle.Projects), len(bundle.Stories), len(bundle.Claims))
		}
		return nil
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach source import <file>|show|path")
	}
}

func cmdCoachDeck(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	bundle, err := store.LoadSource()
	if err != nil {
		if os.IsNotExist(err) {
			return envelope.New(envelope.CodeNotFound, "coach source is missing").WithHint("run `jobkit coach source import <public-bundle.json>`")
		}
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	jobRef := strings.TrimSpace(c.str("job"))
	if jobRef == "" && len(c.args) > 2 {
		jobRef = c.args[2]
	}
	if jobRef == "" {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach deck --job <inbox-id|jd-file|url> [--mode mixed] [--minutes 20]")
	}
	text, company, role, _, _, err := planSource(jobRef)
	if err != nil {
		return err
	}
	posting := jd.Parse(text)
	if posting.Company == "" {
		posting.Company = company
	}
	if posting.Title == "" {
		posting.Title = role
	}
	mode, err := coach.ParseMode(c.str("mode"))
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("use project, behavioral, system-design, claim-defense, or mixed")
	}
	minutes, err := c.int("minutes", 20)
	if err != nil {
		return err
	}
	deck, err := coach.BuildDeck(bundle, posting, text, coach.DeckOptions{
		Mode: mode, Minutes: minutes, ProjectIDs: splitCSV(c.str("projects")), Now: time.Now().UTC(),
	})
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	if err := store.SaveDeck(deck); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if out := c.str("out"); out != "" {
		if out == "auto" {
			out = filepath.Join(store.DecksDir(), deck.ID+".json")
		}
		raw, marshalErr := json.MarshalIndent(deck, "", "  ")
		if marshalErr != nil {
			return envelope.New(envelope.CodeInternal, marshalErr.Error())
		}
		if err := privatefs.WriteFile(out, append(raw, '\n')); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	if c.bool("json") {
		envelope.EmitData(deck)
	} else {
		fmt.Printf("created %s for %s\n", deck.ID, deck.Role)
		fmt.Printf("%d questions · %d minutes · %s\n", len(deck.Questions), deck.Minutes, deck.Mode)
		fmt.Printf("next: jobkit coach run %s\n", deck.ID)
	}
	return nil
}

func cmdCoachRun(c *cli) error {
	if len(c.args) < 3 {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach run <deck-id> [--answers answers.json] [--provider name]")
	}
	store, err := coachStore()
	if err != nil {
		return err
	}
	deck, err := store.LoadDeck(c.args[2])
	if err != nil {
		if os.IsNotExist(err) {
			return envelope.Newf(envelope.CodeNotFound, "coach deck %q does not exist", c.args[2])
		}
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	bundle, err := store.LoadSource()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	started := time.Now().UTC()
	var answers []coach.Answer
	if path := c.str("answers"); path != "" {
		answers, err = loadCoachAnswers(path)
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
	} else {
		if c.bool("json") {
			return envelope.New(envelope.CodeInvalidArgs, "--json requires --answers for coach run").WithHint("interactive prompts use text mode")
		}
		answers, err = readCoachAnswers(deck)
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	if err := coach.ValidateSessionInput(deck, bundle, answers); err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("rebuild the deck after importing a new source; submit one answer for every deck question")
	}
	session := coach.Evaluate(deck, bundle, answers, started, time.Now().UTC())
	useful, err := parseOptionalBool(c.str("useful"))
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	session.Useful = useful
	provider := strings.TrimSpace(c.str("provider"))
	if provider != "" && provider != "none" {
		cfg, cfgErr := loadCoachProviderConfig(store, c.str("provider-config"))
		if cfgErr != nil {
			session.ProviderError = cfgErr.Error()
		} else if feedback, feedbackErr := coach.RunFeedback(context.Background(), cfg, provider, bundle, deck, session); feedbackErr != nil {
			session.ProviderError = feedbackErr.Error()
		} else {
			session.Feedback = feedback
		}
	}
	if err := store.AppendSession(session); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if c.bool("json") {
		envelope.EmitData(session)
		return nil
	}
	fmt.Printf("score: %d/100\n", session.Score)
	fmt.Printf("claim violations: %d\n", session.ClaimViolations)
	fmt.Printf("next review: %s\n", session.NextReviewAt.Local().Format("2006-01-02"))
	for _, result := range session.Results {
		fmt.Printf("  %s  %d/100", result.QuestionID, result.Score.Total)
		if len(result.MissingConcepts) > 0 {
			fmt.Printf("  missing: %s", strings.Join(result.MissingConcepts, ", "))
		}
		fmt.Println()
	}
	if session.Feedback != nil {
		fmt.Printf("advisory feedback (%s): %s\n", session.Feedback.Provider, session.Feedback.Summary)
	}
	if session.ProviderError != "" {
		fmt.Fprintf(os.Stderr, "warning: advisory provider failed: %s\n", session.ProviderError)
	}
	return nil
}

func readCoachAnswers(deck *coach.Deck) ([]coach.Answer, error) {
	reader := bufio.NewReader(os.Stdin)
	answers := make([]coach.Answer, 0, len(deck.Questions))
	fmt.Println("Enter each answer. Put `.done` on its own line to finish the answer.")
	for i, question := range deck.Questions {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(deck.Questions), question.Prompt)
		fmt.Printf("target: %d seconds\n> ", question.TimeSeconds)
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")
			if line == ".done" {
				break
			}
			if line != "" {
				lines = append(lines, line)
			}
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				break
			}
			fmt.Print("> ")
		}
		answers = append(answers, coach.Answer{QuestionID: question.ID, Text: strings.Join(lines, "\n")})
	}
	return answers, nil
}

func loadCoachAnswers(path string) ([]coach.Answer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("decode coach answers: file is empty")
	}
	if trimmed[0] == '[' {
		var answers []coach.Answer
		if err := decodeCoachJSON(trimmed, &answers); err != nil {
			return nil, fmt.Errorf("decode coach answers: %w", err)
		}
		return answers, nil
	}
	var wrapped struct {
		Answers []coach.Answer `json:"answers"`
	}
	if err := decodeCoachJSON(trimmed, &wrapped); err != nil {
		return nil, fmt.Errorf("decode coach answers: %w", err)
	}
	if wrapped.Answers == nil {
		return nil, fmt.Errorf("decode coach answers: answers field is required")
	}
	return wrapped.Answers, nil
}

func decodeCoachJSON(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func cmdCoachStats(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	report, err := store.Stats(time.Now().UTC(), c.str("project"))
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if c.bool("json") {
		envelope.EmitData(report)
		return nil
	}
	fmt.Printf("sessions: %d\naverage: %.1f/100\nclaim violations: %d\ndue reviews: %d\n", report.Sessions, report.AverageScore, report.ClaimViolations, report.DueReviews)
	projects := make([]string, 0, len(report.ByProject))
	for project := range report.ByProject {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	for _, project := range projects {
		band := report.ByProject[project]
		fmt.Printf("  %s  %.1f/100 across %d answers\n", project, band.Average, band.Answers)
	}
	return nil
}

func cmdCoachServe(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	if _, err := store.LoadSource(); err != nil {
		if os.IsNotExist(err) {
			return envelope.New(envelope.CodeNotFound, "coach source is missing").WithHint("run `jobkit coach source import <public-bundle.json>`")
		}
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	addr := strings.TrimSpace(c.str("addr"))
	if addr == "" {
		addr = "127.0.0.1:7331"
	}
	var cfg *coach.ProviderConfig
	configPath := c.str("provider-config")
	if configPath != "" {
		cfg, err = loadCoachProviderConfig(store, configPath)
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
	} else if _, statErr := os.Stat(store.ProvidersPath()); statErr == nil {
		cfg, err = loadCoachProviderConfig(store, "")
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server, err := coach.NewServer(store, cfg)
	if err != nil {
		return envelope.New(envelope.CodeInternal, err.Error())
	}
	fmt.Printf("JobKit Coach: %s\n", server.AccessURL(addr))
	fmt.Println("Press Ctrl-C to stop. The server accepts loopback addresses only and requires its ephemeral access token.")
	if err := server.Serve(ctx, addr); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return nil
}

func loadCoachProviderConfig(store *coach.Store, override string) (*coach.ProviderConfig, error) {
	path := strings.TrimSpace(override)
	if path == "" {
		path = store.ProvidersPath()
	}
	cfg, err := coach.LoadProviderConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load coach provider configuration %s: %w", path, err)
	}
	return cfg, nil
}

func parseOptionalBool(raw string) (*bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "1":
		value := true
		return &value, nil
	case "false", "no", "0":
		value := false
		return &value, nil
	default:
		return nil, fmt.Errorf("--useful must be yes or no")
	}
}
