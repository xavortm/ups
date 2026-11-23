package ups

import (
	"context"
	"encoding/json"
	"strings"
)

type ddevList struct {
	Raw []struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		AppRoot  string `json:"approot"`
		HTTPSURL string `json:"httpsurl"`
		HTTPURL  string `json:"httpurl"`
		Type     string `json:"type"`
	} `json:"raw"`
}

func collectDDEV(ctx context.Context, _ Options) ([]Item, error) {
	out, err := run(ctx, "ddev", "list", "-j")
	if err != nil {
		return nil, err
	}
	return parseDDEV(out)
}

// parseDDEV keeps only running projects; `ddev list` also includes stopped ones.
func parseDDEV(out []byte) ([]Item, error) {
	var l ddevList
	if err := json.Unmarshal(out, &l); err != nil {
		return nil, err
	}
	var items []Item
	for _, p := range l.Raw {
		if p.Status != "running" {
			continue
		}
		url := p.HTTPSURL
		if url == "" {
			url = p.HTTPURL
		}
		items = append(items, Item{
			Name:   p.Name,
			Kind:   "ddev",
			URLs:   []string{url},
			Runs:   []string{strings.TrimSpace("ddev " + p.Type)},
			Path:   p.AppRoot,
			Status: p.Status,
		})
	}
	return items, nil
}
