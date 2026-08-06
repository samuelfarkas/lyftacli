package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"lyftacli/internal/errs"
)

func (a *App) collectionsCommand() *cobra.Command {
	parent := groupCommand("collections", "Create programs/collections")
	var payloadFile, title, description, goal, imageFile, clientID string
	var dryRun bool
	create := &cobra.Command{
		Use: "create", Short: "Create a collection for yourself or a coaching client", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var payload any
			var err error
			if payloadFile != "" {
				mixed := mixedFlags(cmd, "title", "description", "goal", "image-file", "client-id")
				if mixed != "" {
					return errs.New(errs.KindValidation, "--payload cannot be combined with: "+mixed)
				}
				payload, err = a.readJSON(payloadFile)
				if err != nil {
					return err
				}
			} else {
				if strings.TrimSpace(title) == "" {
					return errs.New(errs.KindValidation, "--title is required when --payload is not used")
				}
				collection := map[string]any{"title": strings.TrimSpace(title)}
				putString(collection, "description", description)
				putString(collection, "goal", goal)
				if imageFile != "" {
					data, readErr := os.ReadFile(imageFile)
					if readErr != nil {
						return errs.Wrap(errs.KindInput, "Could not read image file", readErr)
					}
					collection["image"] = base64.StdEncoding.EncodeToString(data)
				}
				object := map[string]any{"collection": collection}
				putString(object, "client_id", clientID)
				payload = object
			}
			object, err := validateCollection(payload)
			if err != nil {
				return err
			}
			return a.performWrite(cmd.Context(), "/api/v1/collections", object, dryRun)
		},
	}
	create.Flags().StringVar(&payloadFile, "payload", "", "Full JSON payload file, or - for stdin")
	create.Flags().StringVar(&title, "title", "", "Collection title")
	create.Flags().StringVar(&description, "description", "", "Collection description")
	create.Flags().StringVar(&goal, "goal", "", "Collection goal")
	create.Flags().StringVar(&imageFile, "image-file", "", "Image to base64 encode")
	create.Flags().StringVar(&clientID, "client-id", "", "Coach client public ID")
	create.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and preview without creating")
	parent.AddCommand(create)
	return parent
}

func (a *App) templatesCommand() *cobra.Command {
	parent := groupCommand("templates", "Create workout templates")
	var payloadFile, clientID, workoutFile, title, description, note, color, picture, pictureFile string
	var collectionID int
	var exercises []string
	var dryRun bool
	create := &cobra.Command{
		Use: "create", Short: "Create a workout template in an existing collection", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var payload any
			var err error
			if payloadFile != "" {
				mixed := mixedFlags(cmd, "collection-id", "client-id", "workout", "title", "description", "note", "color", "picture", "picture-file", "exercise-json")
				if mixed != "" {
					return errs.New(errs.KindValidation, "--payload cannot be combined with: "+mixed)
				}
				payload, err = a.readJSON(payloadFile)
				if err != nil {
					return err
				}
			} else {
				if collectionID < 1 {
					return errs.New(errs.KindValidation, "--collection-id is required when --payload is not used")
				}
				var workout map[string]any
				if workoutFile != "" {
					mixed := mixedFlags(cmd, "title", "description", "note", "color", "picture", "picture-file", "exercise-json")
					if mixed != "" {
						return errs.New(errs.KindValidation, "--workout cannot be combined with workout field options")
					}
					value, readErr := a.readJSON(workoutFile)
					if readErr != nil {
						return readErr
					}
					var ok bool
					workout, ok = value.(map[string]any)
					if !ok {
						return errs.New(errs.KindValidation, "Workout must be a JSON object")
					}
				} else {
					parsedExercises := make([]any, 0, len(exercises))
					for _, source := range exercises {
						var exercise map[string]any
						if decodeErr := decodeJSON(strings.NewReader(source), &exercise); decodeErr != nil {
							return errs.New(errs.KindValidation, "Invalid --exercise-json: "+decodeErr.Error())
						}
						parsedExercises = append(parsedExercises, exercise)
					}
					workout = map[string]any{"exercises": parsedExercises}
					putString(workout, "title", title)
					putString(workout, "description", description)
					putString(workout, "note", note)
					putString(workout, "color", color)
					putString(workout, "picture", picture)
					if pictureFile != "" {
						uri, readErr := imageDataURI(pictureFile)
						if readErr != nil {
							return readErr
						}
						workout["picture"] = uri
					}
				}
				object := map[string]any{"collectionId": collectionID, "workout": workout}
				putString(object, "clientId", clientID)
				payload = object
			}
			object, err := validateTemplate(payload)
			if err != nil {
				return err
			}
			return a.performWrite(cmd.Context(), "/api/v1/templates", object, dryRun)
		},
	}
	create.Flags().StringVar(&payloadFile, "payload", "", "Full JSON payload file, or - for stdin")
	create.Flags().IntVar(&collectionID, "collection-id", 0, "Existing collection ID")
	create.Flags().StringVar(&clientID, "client-id", "", "Coach client public ID")
	create.Flags().StringVar(&workoutFile, "workout", "", "Workout object JSON file, or - for stdin")
	create.Flags().StringVar(&title, "title", "", "Workout title")
	create.Flags().StringVar(&description, "description", "", "Workout description")
	create.Flags().StringVar(&note, "note", "", "Workout note")
	create.Flags().StringVar(&color, "color", "", "Workout color")
	create.Flags().StringVar(&picture, "picture", "", "Existing Lyfta CDN URL or data URI")
	create.Flags().StringVar(&pictureFile, "picture-file", "", "Image to encode as a data URI")
	create.Flags().StringArrayVar(&exercises, "exercise-json", nil, "Exercise object; repeat to preserve workout order")
	create.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and preview without creating")
	parent.AddCommand(create)
	return parent
}

func (a *App) requestCommand() *cobra.Command {
	var queryValues []string
	var dataFile string
	var dryRun bool
	command := &cobra.Command{
		Use: "request METHOD PATH", Short: "Call another endpoint on the configured Lyfta origin", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := args[0]
			switch method {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				return errs.New(errs.KindValidation, "METHOD must be one of GET, POST, PUT, PATCH, DELETE")
			}
			query, err := parseQuery(queryValues)
			if err != nil {
				return err
			}
			var body any
			if dataFile != "" {
				body, err = a.readJSON(dataFile)
				if err != nil {
					return err
				}
			}
			if method == http.MethodGet && body != nil {
				return errs.New(errs.KindValidation, "GET requests cannot include --data")
			}
			if dryRun {
				return a.writeJSON(a.out, map[string]any{"dry_run": true, "method": method, "path": args[1], "query": query, "body": body})
			}
			client, err := a.getClient()
			if err != nil {
				return err
			}
			result, err := client.Request(cmd.Context(), method, args[1], query, body)
			if err != nil {
				return err
			}
			return a.writeJSON(a.out, result)
		},
	}
	command.Flags().StringArrayVar(&queryValues, "query", nil, "Query parameter as KEY=VALUE; repeatable")
	command.Flags().StringVar(&dataFile, "data", "", "JSON request body file, or - for stdin")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without sending")
	return command
}

func (a *App) performWrite(ctx context.Context, path string, payload map[string]any, dryRun bool) error {
	if dryRun {
		return a.writeJSON(a.out, map[string]any{"dry_run": true, "method": "POST", "path": path, "body": payload})
	}
	client, err := a.getClient()
	if err != nil {
		return err
	}
	result, err := client.Post(ctx, path, payload)
	if err != nil {
		return err
	}
	return a.writeJSON(a.out, result)
}

func (a *App) readJSON(source string) (any, error) {
	if source == "-" {
		var value any
		if err := decodeJSON(a.in, &value); err != nil {
			return nil, errs.New(errs.KindInput, err.Error())
		}
		return value, nil
	}
	file, err := os.Open(source)
	if err != nil {
		return nil, errs.Wrap(errs.KindInput, "Could not read JSON file", err)
	}
	defer file.Close()
	var value any
	if err := decodeJSON(file, &value); err != nil {
		return nil, errs.New(errs.KindInput, err.Error())
	}
	return value, nil
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	err := decoder.Decode(&struct{}{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errs.New(errs.KindInput, "JSON input must contain exactly one value")
}

func mixedFlags(command *cobra.Command, names ...string) string {
	used := make([]string, 0)
	for _, name := range names {
		if command.Flags().Changed(name) {
			used = append(used, "--"+name)
		}
	}
	return strings.Join(used, ", ")
}

func putString(target map[string]any, key, value string) {
	if value != "" {
		target[key] = value
	}
}

func parseQuery(values []string) (url.Values, error) {
	result := url.Values{}
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok {
			return nil, errs.New(errs.KindValidation, "Query parameter must use KEY=VALUE: "+value)
		}
		if key == "" {
			return nil, errs.New(errs.KindValidation, "Query parameter key cannot be empty")
		}
		result.Add(key, item)
	}
	return result, nil
}

func imageDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errs.Wrap(errs.KindInput, "Could not read picture file", err)
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func validateCollection(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errs.New(errs.KindValidation, "Collection payload must be a JSON object")
	}
	collection, ok := object["collection"].(map[string]any)
	if !ok {
		return nil, errs.New(errs.KindValidation, "collection must be a JSON object")
	}
	title, ok := collection["title"].(string)
	if !ok || strings.TrimSpace(title) == "" {
		return nil, errs.New(errs.KindValidation, "collection.title must be a non-empty string")
	}
	if clientID, exists := object["client_id"]; exists {
		if _, ok := clientID.(string); !ok {
			return nil, errs.New(errs.KindValidation, "client_id must be a string")
		}
	}
	return object, nil
}

func validateTemplate(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errs.New(errs.KindValidation, "Template payload must be a JSON object")
	}
	collectionID, ok := integerValue(object["collectionId"])
	if !ok || collectionID < 1 {
		return nil, errs.New(errs.KindValidation, "collectionId must be a positive integer")
	}
	workout, ok := object["workout"].(map[string]any)
	if !ok {
		return nil, errs.New(errs.KindValidation, "workout must be a JSON object")
	}
	if exercises, exists := workout["exercises"]; exists {
		if _, ok := exercises.([]any); !ok {
			return nil, errs.New(errs.KindValidation, "workout.exercises must be an array")
		}
	}
	if clientID, exists := object["clientId"]; exists {
		if _, ok := clientID.(string); !ok {
			return nil, errs.New(errs.KindValidation, "clientId must be a string")
		}
	}
	return object, nil
}

func integerValue(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		return parsed, err == nil
	default:
		return 0, false
	}
}
