package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/samuelfarkas/lyftacli/internal/api"
	"github.com/samuelfarkas/lyftacli/internal/errs"
	"github.com/samuelfarkas/lyftacli/internal/spec"
	"github.com/samuelfarkas/lyftacli/internal/version"
)

type runtimeConfig struct {
	baseURL    string
	tokenFile  string
	timeout    float64
	maxRetries int
	pretty     bool
}

type App struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	getenv func(string) string
	config runtimeConfig
	client *api.Client
}

func New(in io.Reader, out, errOut io.Writer) *App {
	return &App{in: in, out: out, errOut: errOut, getenv: os.Getenv}
}

func (a *App) Execute(ctx context.Context, arguments []string) int {
	root := a.rootCommand()
	root.SetArgs(arguments)
	root.SetIn(a.in)
	root.SetOut(a.out)
	root.SetErr(a.errOut)
	command, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return a.writeError(&errs.Error{Kind: errs.KindInterrupted, Message: "Interrupted"}, nil)
	}
	var lyftaErr *errs.Error
	if errors.As(err, &lyftaErr) {
		return a.writeError(lyftaErr, nil)
	}
	usage := ""
	hint := "Run 'lyfta --help' for details."
	if command != nil {
		usage = strings.TrimSpace(command.UseLine())
		hint = fmt.Sprintf("Run '%s --help' for details.", command.CommandPath())
	}
	return a.writeError(&errs.Error{Kind: errs.KindUsage, Message: err.Error()}, map[string]any{"usage": usage, "hint": hint})
}

func (a *App) rootCommand() *cobra.Command {
	a.config = runtimeConfig{
		baseURL: api.DefaultBaseURL, timeout: api.DefaultTimeout.Seconds(), maxRetries: api.DefaultMaxRetries,
	}
	if value := a.getenv("LYFTA_BASE_URL"); value != "" {
		a.config.baseURL = value
	}
	root := &cobra.Command{
		Use:           "lyfta",
		Short:         "Agent-friendly CLI for the Lyfta Developer and Coach APIs.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version.Value,
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("a subcommand is required")
		},
		Example: strings.Join([]string{
			"  lyfta workouts summary --limit 10",
			"  lyfta exercises library --search squat",
			"  lyfta capabilities",
		}, "\n"),
		DisableSuggestions: true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.PersistentFlags().StringVar(&a.config.baseURL, "base-url", a.config.baseURL, "API origin (env: LYFTA_BASE_URL)")
	root.PersistentFlags().StringVar(&a.config.tokenFile, "token-file", "", "Read the API token from this file instead of the environment")
	root.PersistentFlags().Float64Var(&a.config.timeout, "timeout", a.config.timeout, "Request timeout in seconds")
	root.PersistentFlags().IntVar(&a.config.maxRetries, "max-retries", a.config.maxRetries, "Retries for idempotent requests")
	root.PersistentFlags().BoolVar(&a.config.pretty, "pretty", false, "Pretty-print JSON output")

	root.AddCommand(
		a.staticCommand("capabilities", "Print machine-readable commands and request schemas", func() any { return spec.Capabilities(version.Value) }),
		a.staticCommand("mappings", "Print documented equipment, body-part, and muscle ID mappings", func() any { return spec.Mappings() }),
		a.workoutsCommand(), a.exercisesCommand(), a.clientsCommand(),
		a.collectionsCommand(), a.templatesCommand(), a.requestCommand(),
	)
	return root
}

func (a *App) staticCommand(use, short string, value func() any) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return a.writeJSON(a.out, value()) },
	}
}

func (a *App) getClient() (*api.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	if a.config.timeout <= 0 || math.IsNaN(a.config.timeout) || math.IsInf(a.config.timeout, 0) {
		return nil, errs.New(errs.KindConfiguration, "Timeout must be greater than zero")
	}
	if a.config.maxRetries < 0 {
		return nil, errs.New(errs.KindConfiguration, "Max retries cannot be negative")
	}
	token, err := a.resolveToken()
	if err != nil {
		return nil, err
	}
	client, err := api.New(api.Config{
		Token: token, BaseURL: a.config.baseURL,
		Timeout: time.Duration(a.config.timeout * float64(time.Second)), MaxRetries: a.config.maxRetries,
	})
	if err != nil {
		return nil, err
	}
	a.client = client
	return client, nil
}

func groupCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("a subcommand is required for %s", use)
		},
	}
}

func (a *App) resolveToken() (string, error) {
	if a.config.tokenFile != "" {
		data, err := os.ReadFile(a.config.tokenFile)
		if err != nil {
			return "", errs.Wrap(errs.KindConfiguration, "Could not read token file", err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", errs.New(errs.KindConfiguration, "Token file is empty")
		}
		return token, nil
	}
	token := a.getenv("LYFTA_API_TOKEN")
	if token == "" {
		token = a.getenv("LYFTA_API_KEY")
	}
	if token == "" {
		return "", errs.New(errs.KindConfiguration, "No API token configured. Set LYFTA_API_TOKEN or use --token-file.")
	}
	return token, nil
}

func (a *App) writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if a.config.pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return errs.Wrap(errs.KindInput, "Could not write JSON output", err)
	}
	return nil
}

func (a *App) writeError(problem *errs.Error, extra map[string]any) int {
	detail := map[string]any{"type": problem.Kind, "message": problem.Message}
	if problem.StatusCode != 0 {
		detail["status_code"] = problem.StatusCode
	}
	if problem.Details != nil {
		detail["details"] = problem.Details
	}
	if problem.RequestID != "" {
		detail["request_id"] = problem.RequestID
	}
	for key, value := range extra {
		detail[key] = value
	}
	_ = a.writeJSON(a.errOut, map[string]any{"ok": false, "error": detail})
	return errs.ExitCode(problem.Kind)
}
