// snapraid-exporter serves SnapRAID health as Prometheus metrics over HTTP.
// It periodically runs the read-only `snapraid smart` and `snapraid status`
// commands and caches the parsed result; scrapes never invoke snapraid.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type cmdResult struct {
	ok       bool
	exit     int
	duration time.Duration
	at       time.Time
}

type state struct {
	mu     sync.RWMutex
	smart  smartReport
	status statusReport
	// Set once a run has parsed successfully, so a failing command never
	// renders as healthy zeros.
	haveSmart, haveStatus bool
	res                   map[string]cmdResult
}

func run(ctx context.Context, bin, conf, sub string) (string, cmdResult) {
	start := time.Now()
	args := []string{}
	if conf != "" {
		args = append(args, "-c", conf)
	}
	args = append(args, sub)
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	r := cmdResult{ok: err == nil, duration: time.Since(start), at: time.Now()}
	if err != nil {
		log.Printf("snapraid %s failed: %v\n%s", sub, err, strings.TrimSpace(buf.String()))
	}
	if ee, isExit := err.(*exec.ExitError); isExit {
		r.exit = ee.ExitCode()
	} else if err != nil {
		r.exit = -1
	}
	return buf.String(), r
}

func (s *state) refresh(bin, conf string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	smartOut, smartRes := run(ctx, bin, conf, "smart")
	statusOut, statusRes := run(ctx, bin, conf, "status")
	s.mu.Lock()
	defer s.mu.Unlock()
	if smartRes.ok {
		s.smart, s.haveSmart = parseSmart(smartOut), true
	}
	if statusRes.ok {
		s.status, s.haveStatus = parseStatus(statusOut), true
	}
	s.res["smart"], s.res["status"] = smartRes, statusRes
	log.Printf("refreshed: smart ok=%v (%s), status ok=%v (%s)", smartRes.ok, smartRes.duration.Round(time.Millisecond), statusRes.ok, statusRes.duration.Round(time.Millisecond))
}

func esc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

type writer struct {
	sb    strings.Builder
	typed map[string]bool
}

func (w *writer) gauge(name, help string, v float64, labels ...string) {
	if !w.typed[name] {
		w.typed[name] = true
		fmt.Fprintf(&w.sb, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	}
	w.sb.WriteString(name)
	if len(labels) > 0 {
		w.sb.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				w.sb.WriteByte(',')
			}
			fmt.Fprintf(&w.sb, `%s="%s"`, labels[i], esc(labels[i+1]))
		}
		w.sb.WriteByte('}')
	}
	fmt.Fprintf(&w.sb, " %g\n", v)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (s *state) render() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w := &writer{typed: map[string]bool{}}
	for _, cmd := range []string{"smart", "status"} {
		r, ok := s.res[cmd]
		if !ok {
			continue
		}
		l := []string{"command", cmd}
		w.gauge("snapraid_exporter_command_success", "1 if the last snapraid invocation exited 0.", b2f(r.ok), l...)
		w.gauge("snapraid_exporter_command_exit_status", "Exit status of the last snapraid invocation (-1 if it failed to start).", float64(r.exit), l...)
		w.gauge("snapraid_exporter_command_duration_seconds", "Duration of the last snapraid invocation.", r.duration.Seconds(), l...)
		w.gauge("snapraid_exporter_command_last_run_timestamp_seconds", "Unix time of the last snapraid invocation.", float64(r.at.Unix()), l...)
	}
	smartDisks := s.smart.Disks
	if !s.haveSmart {
		smartDisks = nil
	}
	for _, d := range smartDisks {
		l := []string{"disk", d.Name, "device", d.Device, "serial", d.Serial}
		if d.HasTemp {
			w.gauge("snapraid_smart_disk_temperature_celsius", "Disk temperature reported by SMART.", d.TempC, l...)
		}
		if d.HasDays {
			w.gauge("snapraid_smart_disk_power_on_days", "Disk power-on time in days.", d.PowerOnDays, l...)
		}
		if d.HasErrors {
			w.gauge("snapraid_smart_disk_error_count", "SMART error count.", d.ErrorCount, l...)
		}
		if d.HasFailProb {
			w.gauge("snapraid_smart_disk_fail_probability", "Estimated probability (0-1) the disk fails within a year.", d.FailProb, l...)
		}
		if d.HasSize {
			w.gauge("snapraid_smart_disk_size_terabytes", "Disk size in TB.", d.SizeTB, l...)
		}
	}
	if s.haveSmart && s.smart.HasArrayProb {
		w.gauge("snapraid_smart_array_fail_probability", "Estimated probability (0-1) that at least one disk fails within a year.", s.smart.ArrayFailProb)
	}
	if !s.haveStatus {
		return w.sb.String()
	}
	st := s.status
	for _, d := range st.Disks {
		l := []string{"disk", d.Name}
		w.gauge("snapraid_disk_files", "Files on the disk.", d.Files, l...)
		w.gauge("snapraid_disk_fragmented_files", "Fragmented files on the disk.", d.Fragmented, l...)
		w.gauge("snapraid_disk_excess_fragments", "Excess fragments on the disk.", d.ExcessFragments, l...)
		w.gauge("snapraid_disk_wasted_bytes", "Wasted space on the disk.", d.WastedGB*1e9, l...)
		w.gauge("snapraid_disk_used_bytes", "Used space on the disk.", d.UsedGB*1e9, l...)
		w.gauge("snapraid_disk_free_bytes", "Free space on the disk.", d.FreeGB*1e9, l...)
	}
	if t := st.Total; t != nil {
		w.gauge("snapraid_array_files", "Files in the array.", t.Files)
		w.gauge("snapraid_array_used_bytes", "Used space in the array.", t.UsedGB*1e9)
		w.gauge("snapraid_array_free_bytes", "Free space in the array.", t.FreeGB*1e9)
	}
	if st.HasScrubAge {
		w.gauge("snapraid_scrub_oldest_block_age_days", "Age in days of the oldest block's last scrub.", st.ScrubOldestDays)
		w.gauge("snapraid_scrub_median_block_age_days", "Median age in days of blocks' last scrub.", st.ScrubMedianDays)
		w.gauge("snapraid_scrub_newest_block_age_days", "Age in days of the newest block's last scrub.", st.ScrubNewestDays)
	}
	{
		w.gauge("snapraid_unscrubbed_ratio", "Fraction (0-1) of the array not yet scrubbed.", st.UnscrubbedPercent/100)
		w.gauge("snapraid_array_errors", "Errors reported by snapraid status.", st.Errors)
		w.gauge("snapraid_sync_in_progress", "1 if snapraid reports an interrupted sync.", b2f(st.SyncInProgress))
		w.gauge("snapraid_zero_subsecond_files", "Files with a zero sub-second timestamp.", st.ZeroSubsecondFiles)
	}
	return w.sb.String()
}

func main() {
	listen := flag.String("web.listen-address", ":9634", "address to listen on")
	bin := flag.String("snapraid.bin", "snapraid", "path to the snapraid binary")
	conf := flag.String("snapraid.config", "", "snapraid config file (default: snapraid's own default)")
	interval := flag.Duration("collect.interval", 15*time.Minute, "how often to run snapraid")
	timeout := flag.Duration("collect.timeout", 10*time.Minute, "timeout for one collection round")
	flag.Parse()

	s := &state{res: map[string]cmdResult{}}
	go func() {
		for {
			s.refresh(*bin, *conf, *timeout)
			time.Sleep(*interval)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(rw, s.render())
	})
	mux.HandleFunc("/", func(rw http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(rw, `<a href="/metrics">metrics</a>`)
	})
	log.Printf("listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
