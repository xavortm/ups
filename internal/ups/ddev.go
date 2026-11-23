package ups

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

type ddevList struct {
	Raw []struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		AppRoot  string `json:"approot"`
		HTTPSURL string `json:"httpsurl"`
		HTTPURL  string `json:"httpurl"`
		Primary  string `json:"primary_url"`
		Mailpit  string `json:"mailpit_https_url"`
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
		url := firstNonEmpty(p.Primary, p.HTTPSURL, p.HTTPURL)
		var ports []Port
		if port, ok := urlPort(p.Mailpit); ok {
			ports = append(ports, Port{Port: port, URL: p.Mailpit, Role: "mail"})
		}
		items = append(items, Item{
			Name:   p.Name,
			Kind:   "ddev",
			URL:    url,
			Ports:  ports, // the site's container ports (db, …) are added by mergeDDEV
			Runs:   []string{strings.TrimSpace("ddev " + p.Type)},
			Path:   p.AppRoot,
			Status: p.Status,
		})
	}
	return items, nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func urlPort(raw string) (int, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Port() == "" {
		return 0, false
	}
	p, err := strconv.Atoi(u.Port())
	return p, err == nil
}
