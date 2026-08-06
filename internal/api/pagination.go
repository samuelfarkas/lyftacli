package api

import (
	"context"
	"maps"
	"net/url"
	"strconv"

	"lyftacli/internal/errs"
)

func (c *Client) PaginatePages(ctx context.Context, path, itemKey string, params url.Values, maxPages int) (map[string]any, error) {
	if maxPages < 1 {
		return nil, errs.New(errs.KindValidation, "max-pages must be at least 1")
	}
	page := valueInt(params.Get("page"), 1)
	startPage := page
	combined := make([]any, 0)
	pagesFetched := 0
	last := map[string]any{}

	for pagesFetched < maxPages {
		current := cloneValues(params)
		current.Set("page", strconv.Itoa(page))
		response, err := c.Get(ctx, path, current)
		if err != nil {
			return nil, err
		}
		object, ok := response.(map[string]any)
		if !ok {
			return nil, &errs.Error{Kind: errs.KindAPI, Message: "Expected a JSON object while paginating", StatusCode: 502, Details: response}
		}
		items, ok := object[itemKey].([]any)
		if !ok {
			return nil, &errs.Error{Kind: errs.KindAPI, Message: "Expected '" + itemKey + "' to be a list", StatusCode: 502, Details: response}
		}
		combined = append(combined, items...)
		last = object
		pagesFetched++

		totalPages, hasTotal := jsonNumberInt(object["total_pages"])
		currentPage, ok := jsonNumberInt(object["current_page"])
		if !ok {
			currentPage = page
		}
		if len(items) == 0 || (hasTotal && currentPage >= totalPages) {
			break
		}
		page = currentPage + 1
	}

	result := maps.Clone(last)
	result[itemKey] = combined
	result["count"] = len(combined)
	result["current_page"] = startPage
	result["pages_fetched"] = pagesFetched
	totalPages, hasTotal := jsonNumberInt(last["total_pages"])
	currentPage, ok := jsonNumberInt(last["current_page"])
	if !ok {
		currentPage = page
	}
	result["truncated"] = pagesFetched == maxPages && (!hasTotal || currentPage < totalPages)
	return result, nil
}

func (c *Client) PaginateOffsets(ctx context.Context, path string, params url.Values, maxPages int) (map[string]any, error) {
	if maxPages < 1 {
		return nil, errs.New(errs.KindValidation, "max-pages must be at least 1")
	}
	offset := valueInt(params.Get("offset"), 0)
	startOffset := offset
	combined := make([]any, 0)
	pagesFetched := 0
	last := map[string]any{}
	pagination := map[string]any{}

	for pagesFetched < maxPages {
		current := cloneValues(params)
		current.Set("offset", strconv.Itoa(offset))
		response, err := c.Get(ctx, path, current)
		if err != nil {
			return nil, err
		}
		object, ok := response.(map[string]any)
		if !ok {
			return nil, shapeError(response)
		}
		data, ok := object["data"].(map[string]any)
		if !ok {
			return nil, shapeError(response)
		}
		items, itemsOK := data["results"].([]any)
		pageInfo, pageOK := data["pagination"].(map[string]any)
		if !itemsOK || !pageOK {
			return nil, shapeError(response)
		}
		combined = append(combined, items...)
		last = object
		pagination = pageInfo
		pagesFetched++
		hasMore, _ := pageInfo["hasMore"].(bool)
		if !hasMore || len(items) == 0 {
			break
		}
		offset += len(items)
	}

	result := maps.Clone(last)
	data, _ := result["data"].(map[string]any)
	data = maps.Clone(data)
	pageInfo := maps.Clone(pagination)
	pageInfo["offset"] = startOffset
	pageInfo["returned"] = len(combined)
	pageInfo["pages_fetched"] = pagesFetched
	hasMore, _ := pagination["hasMore"].(bool)
	pageInfo["truncated"] = pagesFetched == maxPages && hasMore
	data["results"] = combined
	data["pagination"] = pageInfo
	result["data"] = data
	return result, nil
}

func shapeError(details any) *errs.Error {
	return &errs.Error{Kind: errs.KindAPI, Message: "Unexpected exercise library response shape", StatusCode: 502, Details: details}
}

func cloneValues(values url.Values) url.Values {
	copy := make(url.Values, len(values))
	for key, items := range values {
		copy[key] = append([]string(nil), items...)
	}
	return copy
}

func valueInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
