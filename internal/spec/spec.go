package spec

import "lyftacli/internal/api"

const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

func Capabilities(version string) map[string]any {
	return map[string]any{
		"name": "lyfta", "version": version, "stability": "experimental",
		"base_url": api.DefaultBaseURL,
		"authentication": map[string]any{
			"type": "bearer", "environment": []string{"LYFTA_API_TOKEN", "LYFTA_API_KEY"},
			"token_file_option": "--token-file",
			"transport":         "HTTPS; HTTP is allowed only for loopback development servers",
		},
		"rate_limits": map[string]any{"per_minute": 60, "per_day": 5000},
		"output": map[string]any{
			"success": "compact JSON on stdout", "failure": "JSON on stderr and a non-zero exit code",
			"pretty_option": "--pretty", "ansi": false,
		},
		"global_options": map[string]any{
			"position": "before or after subcommands",
			"options": map[string]any{
				"--base-url":   map[string]any{"type": "string", "environment": "LYFTA_BASE_URL"},
				"--token-file": map[string]any{"type": "path"},
				"--timeout":    map[string]any{"type": "number", "minimumExclusive": 0, "default": int(api.DefaultTimeout.Seconds())},
				"--max-retries": map[string]any{
					"type": "integer", "minimum": 0, "default": api.DefaultMaxRetries,
					"applies_to": "idempotent HTTP methods only",
				},
				"--pretty": map[string]any{"type": "boolean", "default": false},
			},
		},
		"exit_codes": map[string]any{
			"0": "success", "2": "usage, validation, or input error",
			"3": "configuration or transport error", "4": "Lyfta API error", "130": "interrupted",
		},
		"error_types": []string{
			"usage_error", "input_error", "validation_error", "configuration_error",
			"transport_error", "api_error", "interrupted",
		},
		"commands": commands(),
		"schemas": map[string]any{
			"collection_create": collectionSchema(),
			"template_create":   templateSchema(),
		},
	}
}

func commands() []map[string]any {
	return []map[string]any{
		{"command": "capabilities", "side_effect": "none", "authentication_required": false},
		{
			"command": "mappings", "description": "Print documented exercise metadata ID mappings",
			"side_effect": "none", "authentication_required": false,
		},
		readCommand("workouts list", "/api/v1/workouts", pageArguments(20, 100)),
		readCommand("workouts summary", "/api/v1/workouts/summary", pageArguments(100, 1000)),
		readCommand("exercises list", "/api/v1/exercises", pageArguments(20, 100)),
		{
			"command": "exercises library", "method": "GET", "path": "/api/v1/exercises/library",
			"arguments": map[string]any{
				"--search":    map[string]any{"type": "string", "required": false},
				"--limit":     map[string]any{"type": "integer", "minimum": 1, "default": 20},
				"--offset":    map[string]any{"type": "integer", "minimum": 0, "default": 0},
				"--all":       map[string]any{"type": "boolean", "default": false},
				"--max-pages": map[string]any{"type": "integer", "minimum": 1, "default": 100},
			},
			"pagination": "offset", "side_effect": "remote_read", "idempotent": true,
		},
		{
			"command": "exercises progress", "method": "GET", "path": "/api/v1/exercises/progress",
			"arguments": map[string]any{
				"--exercise-id": map[string]any{"type": "string", "required": true},
				"--duration":    map[string]any{"type": "integer", "minimum": 1, "required": true},
				"--client-id":   map[string]any{"type": "string", "required": false},
			},
			"side_effect": "remote_read", "idempotent": true,
		},
		{
			"command": "clients list", "method": "GET", "path": "/api/v1/clients",
			"arguments":   map[string]any{"--active": map[string]any{"type": "boolean", "default": false}},
			"side_effect": "remote_read", "idempotent": true, "plan_required": "Coach or Scale",
		},
		{
			"command": "collections create", "method": "POST", "path": "/api/v1/collections",
			"input": "flags or a complete JSON request via --payload FILE|-", "request_schema": "collection_create",
			"supports_dry_run": true, "side_effect": "creates a remote collection",
			"idempotent": false, "automatic_retries": false,
		},
		{
			"command": "templates create", "method": "POST", "path": "/api/v1/templates",
			"input": "flags, --workout FILE|-, or a complete JSON request via --payload FILE|-", "request_schema": "template_create",
			"supports_dry_run": true, "side_effect": "creates a remote workout template",
			"idempotent": false, "automatic_retries": false,
		},
		{
			"command": "request", "description": "Forward-compatible same-origin API escape hatch",
			"arguments": map[string]any{
				"method":    map[string]any{"enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
				"path":      map[string]any{"type": "string", "required": true},
				"--query":   map[string]any{"type": "string", "repeatable": true},
				"--data":    map[string]any{"type": "path", "accepts_stdin": true},
				"--dry-run": map[string]any{"type": "boolean", "default": false},
			},
			"side_effect": "depends on method and endpoint", "idempotent": "depends on method and endpoint",
			"supports_dry_run": true,
		},
	}
}

func readCommand(command, path string, arguments map[string]any) map[string]any {
	return map[string]any{
		"command": command, "method": "GET", "path": path, "arguments": arguments,
		"pagination": "page", "side_effect": "remote_read", "idempotent": true,
	}
}

func pageArguments(defaultLimit, maxLimit int) map[string]any {
	return map[string]any{
		"--limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": maxLimit, "default": defaultLimit},
		"--page":      map[string]any{"type": "integer", "minimum": 1, "default": 1},
		"--all":       map[string]any{"type": "boolean", "default": false},
		"--max-pages": map[string]any{"type": "integer", "minimum": 1, "default": 100},
		"--client-id": map[string]any{"type": "string", "required": false},
	}
}

func collectionSchema() map[string]any {
	return map[string]any{
		"$schema": schemaDialect, "$id": "urn:lyftacli:schema:collection-create",
		"type": "object", "required": []string{"collection"},
		"properties": map[string]any{
			"client_id": map[string]any{"type": "string", "minLength": 1},
			"collection": map[string]any{
				"type": "object", "required": []string{"title"},
				"properties": map[string]any{
					"title":       map[string]any{"type": "string", "minLength": 1},
					"description": map[string]any{"type": "string"}, "goal": map[string]any{"type": "string"},
					"image": map[string]any{"type": "string", "contentEncoding": "base64"},
				},
			},
		},
	}
}

func templateSchema() map[string]any {
	set := map[string]any{
		"type": "object", "properties": map[string]any{
			"set_type_id": map[string]any{"type": "integer"}, "reps": map[string]any{"type": "string"},
			"from_reps": map[string]any{"type": "string"}, "to_reps": map[string]any{"type": "string"},
			"weight": map[string]any{"type": "string"}, "rir": map[string]any{"type": "string"},
			"duration": map[string]any{"type": "string"}, "distance": map[string]any{"type": "string"},
		},
	}
	exercise := map[string]any{
		"type": "object", "required": []string{"exercise_id", "excercise_name", "exercise_type", "exercise_image"},
		"properties": map[string]any{
			"exercise_id": map[string]any{"type": "integer"}, "excercise_name": map[string]any{"type": "string"},
			"exercise_type": map[string]any{"type": "string"}, "exercise_image": map[string]any{"type": "string"},
			"exercise_note":        map[string]any{"type": "string"},
			"exercise_rest_time":   map[string]any{"type": "integer", "minimum": 0},
			"exercise_superset_id": map[string]any{"type": "integer"}, "is_rep_range_active": map[string]any{"type": "boolean"},
			"sets": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/template_set"}},
		},
	}
	return map[string]any{
		"$schema": schemaDialect, "$id": "urn:lyftacli:schema:template-create",
		"type": "object", "required": []string{"collectionId", "workout"},
		"properties": map[string]any{
			"clientId":     map[string]any{"type": "string", "minLength": 1},
			"collectionId": map[string]any{"type": "integer", "minimum": 1},
			"workout": map[string]any{
				"type": "object", "properties": map[string]any{
					"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
					"note": map[string]any{"type": "string"}, "color": map[string]any{"type": "string"},
					"picture":   map[string]any{"type": "string"},
					"exercises": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/template_exercise"}},
				},
			},
		},
		"$defs": map[string]any{"template_exercise": exercise, "template_set": set},
	}
}
