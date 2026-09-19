package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type client struct {
	base       string
	http       *http.Client
	user, pass string
}

func (c *client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// The daemon reports timestamps in its own local time with no zone. All ages
// are computed against the daemon's own clock (pulse.current_at), so the zone
// never matters.
const daemonTime = "2006-01-02T15:04:05"

type pulse struct {
	CurrentAt string `json:"current_at"`
}

type array struct {
	Pulse              pulse   `json:"pulse"`
	DaemonVersion      string  `json:"daemon_version"`
	EngineVersion      string  `json:"engine_version"`
	Health             string  `json:"health"`
	TotalSpaceBytes    float64 `json:"total_space_bytes"`
	FreeSpaceBytes     float64 `json:"free_space_bytes"`
	AnnualFailureRate  float64 `json:"annual_failure_rate"`
	FailureProbability float64 `json:"failure_probability"`
	FilesCount         float64 `json:"files_count"`
	BlocksBad          float64 `json:"blocks_bad"`
	BlocksRehash       float64 `json:"blocks_rehash"`
	BlocksUnsynced     float64 `json:"blocks_unsynced"`
	BlocksUnscrubbed   float64 `json:"blocks_unscrubbed"`
	BlocksCount        float64 `json:"blocks_count"`
	LastSyncAt         string  `json:"last_sync_at"`
	LastScrubAt        string  `json:"last_scrub_at"`
	LastDiffAt         string  `json:"last_diff_at"`
	HoldOff            bool    `json:"hold_off"`
	DiffEqual          float64 `json:"diff_equal"`
	DiffAdded          float64 `json:"diff_added"`
	DiffRemoved        float64 `json:"diff_removed"`
	DiffUpdated        float64 `json:"diff_updated"`
	DiffMoved          float64 `json:"diff_moved"`
	DiffCopied         float64 `json:"diff_copied"`
	DiffRelocated      float64 `json:"diff_relocated"`
	DiffRestored       float64 `json:"diff_restored"`
	FixRecovered       float64 `json:"fix_recovered"`
	FixUnrecoverable   float64 `json:"fix_unrecoverable"`
}

type smartAttr struct {
	Name string  `json:"name"`
	Raw  float64 `json:"raw"`
	Norm float64 `json:"norm"`
}

type device struct {
	Node               string  `json:"node"`
	Health             string  `json:"health"`
	Model              string  `json:"model"`
	Serial             string  `json:"serial"`
	Power              string  `json:"power"`
	SizeBytes          float64 `json:"size_bytes"`
	AnnualFailureRate  float64 `json:"annual_failure_rate"`
	FailureProbability float64 `json:"failure_probability"`
	Smart              struct {
		Attributes   []smartAttr `json:"attributes"`
		PowerOnHours float64     `json:"power_on_hours"`
		Temperature  *float64    `json:"temperature_celsius"`
		Failing      bool        `json:"failing"`
		Prefail      bool        `json:"prefail"`
		ErrorLogged  bool        `json:"error_logged"`
	} `json:"smart"`
}

type disk struct {
	Name            string   `json:"name"`
	Health          string   `json:"health"`
	TotalSpaceBytes float64  `json:"total_space_bytes"`
	FreeSpaceBytes  float64  `json:"free_space_bytes"`
	AccessCount     float64  `json:"access_count"`
	ErrorIO         float64  `json:"error_io"`
	ErrorData       float64  `json:"error_data"`
	Devices         []device `json:"devices"`
}

type disks struct {
	Data   []disk `json:"data_disks"`
	Parity []disk `json:"parity_disks"`
	Extra  []disk `json:"extra_disks"`
}

type task struct {
	Number     int     `json:"number"`
	Command    string  `json:"command"`
	Health     string  `json:"health"`
	Status     string  `json:"status"`
	ExitCode   float64 `json:"exit_code"`
	FinishedAt string  `json:"finished_at"`
	Progress   float64 `json:"progress"`
	ETASeconds float64 `json:"eta_seconds"`
	SpeedMBs   float64 `json:"speed_mbs"`
	ErrorSoft  float64 `json:"error_soft"`
	ErrorIO    float64 `json:"error_io"`
	ErrorData  float64 `json:"error_data"`
}

type tasks struct {
	Pending []task `json:"pending"`
	Active  []task `json:"active"`
	History []task `json:"history"`
}

func ageSeconds(now, then string) (float64, bool) {
	n, err1 := time.Parse(daemonTime, now)
	t, err2 := time.Parse(daemonTime, then)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return n.Sub(t).Seconds(), true
}
