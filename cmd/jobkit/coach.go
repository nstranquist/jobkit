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
	case "study":
		return cmdCoachStudy(c)
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
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach source|deck|run|stats|serve|study|path")
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

func cmdCoachStudy(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	action := ""
	if len(c.args) > 2 {
		action = c.args[2]
	}
	moduleID := strings.TrimSpace(c.str("module"))
	practiceID := strings.TrimSpace(c.str("practice"))
	if moduleID == "" && action != "" && action != "list" && action != "show" && action != "attempt" && action != "next" && action != "status" && action != "claims" && action != "launch" {
		moduleID = action
		action = "show"
	}
	answer, err := studyAnswer(c)
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	switch action {
	case "claims":
		cur, err := coach.LoadCurriculum()
		if err != nil {
			return envelope.New(envelope.CodeInternal, err.Error())
		}
		rows := coach.ClaimTrace(cur)
		if c.bool("json") {
			envelope.EmitData(map[string]any{"claims": rows})
			return nil
		}
		return printClaimTrace(rows)
	case "show":
		if moduleID == "" && len(c.args) > 3 {
			moduleID = c.args[3]
		}
		if moduleID == "" {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach study show <module>")
		}
	case "attempt":
		if moduleID == "" && len(c.args) > 3 {
			moduleID = c.args[3]
		}
		if strings.TrimSpace(answer) == "" {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach study attempt <module> --answer TEXT|--answers FILE")
		}
	case "list", "next", "status", "launch", "":
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach study [list|show|attempt|next|status|claims]").WithHint("jobkit coach study --module docs-puller --answer \"...\" teaches, scores, and reports the next step")
	}
	if action == "attempt" && moduleID == "" {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit coach study attempt <module> --answer TEXT|--answers FILE")
	}
	opts := coach.LaunchOptions{ModuleID: moduleID, PracticeID: practiceID, Answer: answer, Now: time.Now().UTC()}
	if action == "next" {
		opts = coach.LaunchOptions{}
	}
	report, err := coach.Launch(store, opts)
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	if action == "show" && report.Module == nil {
		return envelope.New(envelope.CodeNotFound, "study module is missing")
	}
	if c.bool("json") {
		envelope.EmitData(report)
		return nil
	}
	printStudyReport(report)
	return nil
}

func studyAnswer(c *cli) (string, error) {
	if path := strings.TrimSpace(c.str("answers")); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			return "", fmt.Errorf("study answers file is empty")
		}
		if trimmed[0] == '{' || trimmed[0] == '[' {
			var wrapped struct {
				Text   string `json:"text"`
				Answer string `json:"answer"`
			}
			if err := decodeCoachJSON(trimmed, &wrapped); err == nil {
				if text := firstNonEmpty(wrapped.Text, wrapped.Answer); text != "" {
					return text, nil
				}
			}
			answers, err := loadCoachAnswers(path)
			if err != nil {
				return "", err
			}
			if len(answers) == 0 {
				return "", fmt.Errorf("study answers file has no text")
			}
			return answers[0].Text, nil
		}
		return string(trimmed), nil
	}
	return c.str("answer"), nil
}

func printStudyReport(report *coach.StudyReport) {
	if len(report.Modules) > 0 {
		fmt.Println("PIN MODULES")
		for _, module := range report.Modules {
			mark := ""
			if module.Complete {
				mark = " done"
			}
			fmt.Printf("  %d. %s  %d/%d%s\n", module.Order, module.ID, module.Passed, module.Practices, mark)
		}
		fmt.Println()
	}
	if report.Module != nil {
		fmt.Printf("MODULE %s\n", report.Module.ID)
		fmt.Printf("Purpose: %s\n", report.Module.Purpose)
		if len(report.Module.Architecture) > 0 {
			fmt.Println("Architecture:")
			for _, line := range report.Module.Architecture {
				fmt.Printf("  - %s\n", line)
			}
		}
		if len(report.Module.Decisions) > 0 {
			fmt.Println("Decisions:")
			for _, line := range report.Module.Decisions {
				fmt.Printf("  - %s\n", line)
			}
		}
		fmt.Printf("Run / demo:\n  %s\n", report.Module.RunDemo)
		if len(report.Module.TalkingPoints) > 0 {
			fmt.Println("Talking points:")
			for _, line := range report.Module.TalkingPoints {
				fmt.Printf("  - %s\n", line)
			}
		}
		if len(report.Module.Practices) > 0 {
			fmt.Println("Practices:")
			for _, practice := range report.Module.Practices {
				fmt.Printf("  - %s (%s) %s\n", practice.ID, practice.Kind, practice.Prompt)
			}
		}
		fmt.Println()
	}
	if report.Attempt != nil {
		status := "FAIL"
		if report.Attempt.Passed {
			status = "PASS"
		}
		fmt.Printf("SCORE %d/100  %s  (%s)\n", report.Attempt.Score, status, report.Attempt.Verdict)
		if len(report.Attempt.CoveredConcepts) > 0 {
			fmt.Printf("covered: %s\n", strings.Join(report.Attempt.CoveredConcepts, ", "))
		}
		if len(report.Attempt.MissingConcepts) > 0 {
			fmt.Printf("missing: %s\n", strings.Join(report.Attempt.MissingConcepts, ", "))
		}
		if len(report.Attempt.ClaimViolations) > 0 {
			fmt.Printf("claim violations: %d\n", len(report.Attempt.ClaimViolations))
			for _, v := range report.Attempt.ClaimViolations {
				fmt.Printf("  - %s\n", v.Token)
			}
		}
		fmt.Println()
	}
	if report.Next != nil && report.Next.Prompt != "" {
		fmt.Printf("next: %s / %s — %s\n", report.Next.ModuleID, report.Next.PracticeID, report.Next.Prompt)
		return
	}
	fmt.Println("next: all pin practices complete")
}

func printClaimTrace(rows []coach.ClaimTraceRow) error {
	fmt.Printf("%-16s %-12s %-28s %-18s %s\n", "MODULE", "TOKEN", "CLAIM", "AUTHORITY", "LOCATOR")
	for _, row := range rows {
		fmt.Printf("%-16s %-12s %-28s %-18s %s\n", row.ModuleID, row.Token, row.ClaimID, row.Authority, row.Locator)
	}
	return nil
}

func cmdCoachServe(c *cli) error {
	store, err := coachStore()
	if err != nil {
		return err
	}
	if _, err := store.LoadSource(); err != nil && !os.IsNotExist(err) {
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
