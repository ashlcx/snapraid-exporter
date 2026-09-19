package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]string{
		"/snapraid/v1/array": "testdata/array.json",
		"/snapraid/v1/disks": "testdata/disks.json",
		"/snapraid/v1/tasks": "testdata/tasks.json",
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Error(err)
		}
		w.Write(b)
	}))
}

func TestCollect(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	out, err := collect(context.Background(), &client{base: srv.URL, http: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"snapraid_up 1\n",
		`snapraid_array_health{health="passed"} 1`,
		`snapraid_array_health{health="failing"} 0`,
		`snapraid_blocks{state="bad"} 0`,
		`snapraid_disk_health{disk="d1",role="data",health="passed"} 1`,
		`snapraid_disk_health{disk="parity",role="parity",health="passed"} 1`,
		`snapraid_device_smart_failing{disk="d1",role="data",device="/dev/sdg",serial="SERIAL0001",model="ST8000VN004-3CP101"} 0`,
		`snapraid_device_smart_attribute_raw{disk="d1",role="data",device="/dev/sdg",serial="SERIAL0001",model="ST8000VN004-3CP101",attribute="Reallocated_Sector_Ct"} 0`,
		`snapraid_last_run_age_seconds{operation="sync"}`,
		`snapraid_task_active{command="sync"} 1`,
		`snapraid_task_last_exit_code{command="sync"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, `snapraid_task_last_success{command="diff"}`) {
		t.Error("diff must not have a pass/fail metric")
	}
	assertGrouped(t, out)
}

// The exposition format requires all samples of a family to be contiguous.
func assertGrouped(t *testing.T, out string) {
	t.Helper()
	seen := map[string]bool{}
	prev := ""
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name := line[:strings.IndexAny(line, "{ ")]
		if name != prev && seen[name] {
			t.Errorf("samples of %s are not contiguous", name)
		}
		seen[name], prev = true, name
	}
}

func TestCollectDaemonDown(t *testing.T) {
	srv := fixtureServer(t)
	srv.Close()
	out, err := collect(context.Background(), &client{base: srv.URL, http: &http.Client{Timeout: time.Second}})
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(out, "snapraid_up 0") || strings.Contains(out, "snapraid_array_health") {
		t.Errorf("down output wrong:\n%s", out)
	}
}

func TestAgeSeconds(t *testing.T) {
	if a, ok := ageSeconds("2026-09-20T01:00:00", "2026-09-20T00:00:00"); !ok || a != 3600 {
		t.Errorf("age=%v ok=%v", a, ok)
	}
	if _, ok := ageSeconds("2026-09-20T01:00:00", ""); ok {
		t.Error("empty timestamp must not yield an age")
	}
}
