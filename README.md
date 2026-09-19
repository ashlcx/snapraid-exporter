# snapraid-exporter

Prometheus exporter for SnapRAID that reads the REST API of
[snapraid-daemon](https://github.com/amadvance/snapraid-daemon). It needs no
privileges and no snapraid binary: the daemon serves cached state, and a scrape
never probes or spins up disks.

Tested against snapraid-daemon v1.14 (`/snapraid/v1/{array,disks,tasks}`).
Newer daemons ship a built-in `/metrics`; this exporter targets releases that
don't have it yet.

## Run

    docker run --rm -p 9634:9634 ghcr.io/ashlcx/snapraid-exporter \
      --daemon.url=http://<host>:7627

The daemon's `net_acl` must allow the exporter's address. If the daemon uses
`net_auth_credential`, set `SNAPRAIDD_USER` and `SNAPRAIDD_PASSWORD`.

| flag | default |
|---|---|
| `--web.listen-address` | `:9634` |
| `--daemon.url` | `http://127.0.0.1:7627` |
| `--daemon.timeout` | `10s` |

## Metrics

| metric | labels |
|---|---|
| `snapraid_up`, `snapraid_info` | `daemon_version`, `engine_version` |
| `snapraid_array_health`, `snapraid_disk_health`, `snapraid_device_health` (one-hot) | `health` |
| `snapraid_array_{total,free}_bytes`, `_files`, `_failure_probability`, `_annual_failure_rate`, `_hold_off` | |
| `snapraid_blocks` | `state` = bad, rehash, unsynced, unscrubbed, total |
| `snapraid_diff_files` | `change` |
| `snapraid_last_run_age_seconds` | `operation` = sync, scrub, diff |
| `snapraid_disk_{total_bytes,free_bytes,access_count,errors}` | `disk`, `role`, (`type`) |
| `snapraid_device_*` (temperature, power-on hours, failure probability, SMART flags) | `disk`, `role`, `device`, `serial`, `model` |
| `snapraid_device_smart_attribute_{raw,normalized}` | + `attribute` |
| `snapraid_task_{active,progress_percent,eta_seconds,speed_megabytes_per_second}`, `snapraid_tasks_pending` | `command` |
| `snapraid_task_last_{exit_code,finished_age_seconds}`, `snapraid_task_last_success` (sync/scrub only) | `command` |

Ages are computed against the daemon's own clock, so time zones don't matter.

## Container image and releases

`ghcr.io/ashlcx/snapraid-exporter` (distroless, non-root). `main` publishes `:latest` and
`:sha-<sha>`; pushing a tag `vX.Y.Z` publishes `:X.Y.Z`, `:X.Y` (and `:X` from v1) and creates a
GitHub release.

    git tag v0.2.0 && git push --tags
