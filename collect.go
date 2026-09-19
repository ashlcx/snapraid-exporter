package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var healthStates = []string{"passed", "pending", "prefail", "degraded", "corrupt", "failing"}

type family struct {
	help    string
	samples []string
}

// writer groups samples by metric name, as the exposition format requires,
// and renders families in first-use order.
type writer struct {
	order []string
	fams  map[string]*family
}

func newWriter() *writer { return &writer{fams: map[string]*family{}} }

func esc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func (w *writer) gauge(name, help string, v float64, labels ...string) {
	f, ok := w.fams[name]
	if !ok {
		f = &family{help: help}
		w.fams[name] = f
		w.order = append(w.order, name)
	}
	var sb strings.Builder
	sb.WriteString(name)
	if len(labels) > 0 {
		sb.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `%s="%s"`, labels[i], esc(labels[i+1]))
		}
		sb.WriteByte('}')
	}
	fmt.Fprintf(&sb, " %g", v)
	f.samples = append(f.samples, sb.String())
}

func (w *writer) String() string {
	var sb strings.Builder
	for _, name := range w.order {
		f := w.fams[name]
		fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s gauge\n", name, f.help, name)
		for _, s := range f.samples {
			sb.WriteString(s + "\n")
		}
	}
	return sb.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// healthState emits a one-hot state set so alerts can match on health!="passed".
func (w *writer) healthState(name, help, current string, labels ...string) {
	for _, s := range healthStates {
		w.gauge(name, help, b2f(s == current), append(append([]string{}, labels...), "health", s)...)
	}
}

// collect fetches the daemon state and renders it. On failure it still returns
// a valid exposition with snapraid_up 0.
func collect(ctx context.Context, c *client) (string, error) {
	var (
		a    array
		d    disks
		t    tasks
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)
	for path, dst := range map[string]any{
		"/snapraid/v1/array": &a,
		"/snapraid/v1/disks": &d,
		"/snapraid/v1/tasks": &t,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.get(ctx, path, dst); err != nil {
				mu.Lock()
				errs = append(errs, err.Error())
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	w := newWriter()
	if len(errs) > 0 {
		sort.Strings(errs)
		w.gauge("snapraid_up", "1 if the last daemon query succeeded.", 0)
		return w.String(), fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	w.gauge("snapraid_up", "1 if the last daemon query succeeded.", 1)
	render(w, a, d, t)
	return w.String(), nil
}

func render(w *writer, a array, d disks, t tasks) {
	now := a.Pulse.CurrentAt
	w.gauge("snapraid_info", "Daemon and engine versions.", 1, "daemon_version", a.DaemonVersion, "engine_version", a.EngineVersion)
	w.healthState("snapraid_array_health", "Array health (1 for the current state).", a.Health)
	w.gauge("snapraid_array_total_bytes", "Total array space.", a.TotalSpaceBytes)
	w.gauge("snapraid_array_free_bytes", "Free array space.", a.FreeSpaceBytes)
	w.gauge("snapraid_array_files", "Files in the array.", a.FilesCount)
	w.gauge("snapraid_array_failure_probability", "Estimated probability (0-1) that a disk fails within a year.", a.FailureProbability)
	w.gauge("snapraid_array_annual_failure_rate", "Estimated annual failure rate of the array.", a.AnnualFailureRate)
	w.gauge("snapraid_array_hold_off", "1 if scheduled maintenance is held off.", b2f(a.HoldOff))
	blocks := map[string]float64{
		"bad": a.BlocksBad, "rehash": a.BlocksRehash, "unsynced": a.BlocksUnsynced,
		"unscrubbed": a.BlocksUnscrubbed, "total": a.BlocksCount,
	}
	for _, name := range sortedKeys(blocks) {
		w.gauge("snapraid_blocks", "Blocks by state.", blocks[name], "state", name)
	}
	diff := map[string]float64{
		"equal": a.DiffEqual, "added": a.DiffAdded, "removed": a.DiffRemoved, "updated": a.DiffUpdated,
		"moved": a.DiffMoved, "copied": a.DiffCopied, "relocated": a.DiffRelocated, "restored": a.DiffRestored,
	}
	for _, change := range sortedKeys(diff) {
		w.gauge("snapraid_diff_files", "Files by change since the last sync, as of the last diff.", diff[change], "change", change)
	}
	w.gauge("snapraid_fix_recovered_blocks", "Blocks recovered by the last fix.", a.FixRecovered)
	w.gauge("snapraid_fix_unrecoverable_blocks", "Blocks the last fix could not recover.", a.FixUnrecoverable)
	lastRun := map[string]string{"sync": a.LastSyncAt, "scrub": a.LastScrubAt, "diff": a.LastDiffAt}
	for _, op := range sortedKeys(lastRun) {
		if age, ok := ageSeconds(now, lastRun[op]); ok {
			w.gauge("snapraid_last_run_age_seconds", "Seconds since the last successful run of the operation.", age, "operation", op)
		}
	}

	roles := map[string][]disk{"data": d.Data, "parity": d.Parity, "extra": d.Extra}
	for _, role := range []string{"data", "parity", "extra"} {
		for _, dk := range roles[role] {
			l := []string{"disk", dk.Name, "role", role}
			w.healthState("snapraid_disk_health", "Disk health (1 for the current state).", dk.Health, l...)
			w.gauge("snapraid_disk_total_bytes", "Filesystem size.", dk.TotalSpaceBytes, l...)
			w.gauge("snapraid_disk_free_bytes", "Filesystem free space.", dk.FreeSpaceBytes, l...)
			w.gauge("snapraid_disk_access_count", "Access count since the daemon started counting.", dk.AccessCount, l...)
			w.gauge("snapraid_disk_errors", "Errors reported on the disk.", dk.ErrorIO, append(l, "type", "io")...)
			w.gauge("snapraid_disk_errors", "Errors reported on the disk.", dk.ErrorData, append(l, "type", "data")...)
			for _, dev := range dk.Devices {
				dl := append(append([]string{}, l...), "device", dev.Node, "serial", dev.Serial, "model", dev.Model)
				w.healthState("snapraid_device_health", "Device health (1 for the current state).", dev.Health, dl...)
				w.gauge("snapraid_device_active", "1 if the device is spun up.", b2f(dev.Power == "active"), dl...)
				w.gauge("snapraid_device_size_bytes", "Device size.", dev.SizeBytes, dl...)
				w.gauge("snapraid_device_failure_probability", "Estimated probability (0-1) the device fails within a year.", dev.FailureProbability, dl...)
				w.gauge("snapraid_device_annual_failure_rate", "Estimated annual failure rate.", dev.AnnualFailureRate, dl...)
				w.gauge("snapraid_device_power_on_hours", "SMART power-on hours.", dev.Smart.PowerOnHours, dl...)
				if dev.Smart.Temperature != nil {
					w.gauge("snapraid_device_temperature_celsius", "SMART temperature.", *dev.Smart.Temperature, dl...)
				}
				w.gauge("snapraid_device_smart_failing", "1 if SMART reports the device as failing.", b2f(dev.Smart.Failing), dl...)
				w.gauge("snapraid_device_smart_prefail", "1 if SMART reports a pre-failure condition.", b2f(dev.Smart.Prefail), dl...)
				w.gauge("snapraid_device_smart_error_logged", "1 if the SMART error log has entries.", b2f(dev.Smart.ErrorLogged), dl...)
				for _, at := range dev.Smart.Attributes {
					al := append(append([]string{}, dl...), "attribute", at.Name)
					w.gauge("snapraid_device_smart_attribute_raw", "Raw SMART attribute value.", at.Raw, al...)
					w.gauge("snapraid_device_smart_attribute_normalized", "Normalized SMART attribute value.", at.Norm, al...)
				}
			}
		}
	}

	for _, tk := range t.Active {
		l := []string{"command", tk.Command}
		w.gauge("snapraid_task_active", "1 while the command is running.", 1, l...)
		w.gauge("snapraid_task_progress_percent", "Progress of the running command.", tk.Progress, l...)
		w.gauge("snapraid_task_eta_seconds", "Estimated seconds remaining.", tk.ETASeconds, l...)
		w.gauge("snapraid_task_speed_megabytes_per_second", "Processing speed of the running command.", tk.SpeedMBs, l...)
	}
	w.gauge("snapraid_tasks_pending", "Queued tasks.", float64(len(t.Pending)))
	// Newest finished task per command.
	last := map[string]task{}
	for _, tk := range t.History {
		if cur, ok := last[tk.Command]; !ok || tk.Number > cur.Number {
			last[tk.Command] = tk
		}
	}
	for _, cmd := range sortedKeys(last) {
		tk := last[cmd]
		l := []string{"command", cmd}
		w.gauge("snapraid_task_last_exit_code", "Exit code of the most recent finished run of the command.", tk.ExitCode, l...)
		// diff exits 2 whenever files changed, so only sync/scrub have a
		// meaningful pass/fail.
		if cmd == "sync" || cmd == "scrub" {
			w.gauge("snapraid_task_last_success", "1 if the most recent sync/scrub finished with exit code 0.", b2f(tk.ExitCode == 0 && tk.Status == "terminated"), l...)
		}
		if age, ok := ageSeconds(now, tk.FinishedAt); ok {
			w.gauge("snapraid_task_last_finished_age_seconds", "Seconds since the command last finished.", age, l...)
		}
	}
}
