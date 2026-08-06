package cli

import (
	"fmt"
	"maps"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"lyftacli/internal/errs"
)

func (a *App) workoutsCommand() *cobra.Command {
	command := groupCommand("workouts", "Fetch workout data")
	command.AddCommand(
		a.pageCommand("list", "Fetch detailed workouts", "/api/v1/workouts", "workouts", 20, 100),
		a.pageCommand("summary", "Fetch workout summaries", "/api/v1/workouts/summary", "workouts", 100, 1000),
	)
	return command
}

func (a *App) exercisesCommand() *cobra.Command {
	command := groupCommand("exercises", "Fetch exercise data")
	command.AddCommand(
		a.pageCommand("list", "Fetch exercises performed by a user", "/api/v1/exercises", "exercises", 20, 100),
		a.exerciseLibraryCommand(), a.exerciseProgressCommand(),
	)
	return command
}

func (a *App) pageCommand(use, short, path, itemKey string, defaultLimit, maxLimit int) *cobra.Command {
	var limit, page, maxPages int
	var fetchAll bool
	var clientID string
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 || limit > maxLimit {
				return errs.New(errs.KindValidation, "--limit must be between 1 and "+strconv.Itoa(maxLimit))
			}
			if page < 1 || maxPages < 1 {
				return errs.New(errs.KindValidation, "--page and --max-pages must be at least 1")
			}
			client, err := a.getClient()
			if err != nil {
				return err
			}
			params := url.Values{"limit": {strconv.Itoa(limit)}, "page": {strconv.Itoa(page)}}
			if clientID != "" {
				params.Set("client_id", clientID)
			}
			var result any
			if fetchAll {
				result, err = client.PaginatePages(cmd.Context(), path, itemKey, params, maxPages)
			} else {
				result, err = client.Get(cmd.Context(), path, params)
			}
			if err != nil {
				return err
			}
			return a.writeJSON(a.out, result)
		},
	}
	command.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum records per page")
	command.Flags().IntVar(&page, "page", 1, "Page number")
	command.Flags().BoolVar(&fetchAll, "all", false, "Fetch and combine all pages")
	command.Flags().IntVar(&maxPages, "max-pages", 100, "Safety cap used with --all")
	command.Flags().StringVar(&clientID, "client-id", "", "Coach client public ID")
	return command
}

func (a *App) exerciseLibraryCommand() *cobra.Command {
	var search string
	var limit, offset, maxPages int
	var fetchAll bool
	command := &cobra.Command{
		Use: "library", Short: "Search the full Lyfta exercise catalog", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 || offset < 0 || maxPages < 1 {
				return errs.New(errs.KindValidation, "--limit and --max-pages must be positive; --offset cannot be negative")
			}
			client, err := a.getClient()
			if err != nil {
				return err
			}
			params := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
			if search != "" {
				params.Set("search", search)
			}
			var result any
			if fetchAll {
				result, err = client.PaginateOffsets(cmd.Context(), "/api/v1/exercises/library", params, maxPages)
			} else {
				result, err = client.Get(cmd.Context(), "/api/v1/exercises/library", params)
			}
			if err != nil {
				return err
			}
			return a.writeJSON(a.out, result)
		},
	}
	command.Flags().StringVar(&search, "search", "", "Exercise name search text")
	command.Flags().IntVar(&limit, "limit", 20, "Maximum records per page")
	command.Flags().IntVar(&offset, "offset", 0, "Result offset")
	command.Flags().BoolVar(&fetchAll, "all", false, "Fetch and combine all pages")
	command.Flags().IntVar(&maxPages, "max-pages", 100, "Safety cap used with --all")
	return command
}

func (a *App) exerciseProgressCommand() *cobra.Command {
	var exerciseID, clientID string
	var duration int
	command := &cobra.Command{
		Use: "progress", Short: "Fetch progress for one performed exercise", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if exerciseID == "" || duration < 1 {
				return errs.New(errs.KindValidation, "--exercise-id and a positive --duration are required")
			}
			client, err := a.getClient()
			if err != nil {
				return err
			}
			params := url.Values{"exercise_id": {exerciseID}, "duration": {strconv.Itoa(duration)}}
			if clientID != "" {
				params.Set("client_id", clientID)
			}
			result, err := client.Get(cmd.Context(), "/api/v1/exercises/progress", params)
			if err != nil {
				return err
			}
			return a.writeJSON(a.out, result)
		},
	}
	command.Flags().StringVar(&exerciseID, "exercise-id", "", "Performed exercise ID")
	command.Flags().IntVar(&duration, "duration", 0, "Lookback duration in days")
	command.Flags().StringVar(&clientID, "client-id", "", "Coach client public ID")
	return command
}

func (a *App) clientsCommand() *cobra.Command {
	command := groupCommand("clients", "Access Coach API clients")
	var active bool
	list := &cobra.Command{
		Use: "list", Short: "List linked coaching clients", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.getClient()
			if err != nil {
				return err
			}
			response, err := client.Get(cmd.Context(), "/api/v1/clients", nil)
			if err != nil {
				return err
			}
			if active {
				object, ok := response.(map[string]any)
				if !ok {
					return &errs.Error{Kind: errs.KindValidation, Message: "Unexpected clients response shape", Details: response}
				}
				items, ok := object["data"].([]any)
				if !ok {
					return &errs.Error{Kind: errs.KindValidation, Message: "Unexpected clients response shape", Details: response}
				}
				filtered := make([]any, 0, len(items))
				for _, item := range items {
					entry, ok := item.(map[string]any)
					if ok && fmt.Sprint(entry["status"]) == "1" {
						filtered = append(filtered, item)
					}
				}
				filteredResponse := maps.Clone(object)
				filteredResponse["data"] = filtered
				filteredResponse["count"] = len(filtered)
				response = filteredResponse
			}
			return a.writeJSON(a.out, response)
		},
	}
	list.Flags().BoolVar(&active, "active", false, "Only return status=1")
	command.AddCommand(list)
	return command
}
