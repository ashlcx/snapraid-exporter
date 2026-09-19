# snapraid-exporter

Serves SnapRAID health as Prometheus metrics over HTTP (default `:9634/metrics`).
Inspired by [ljmerza/snapraid-collector](https://github.com/ljmerza/snapraid-collector),
which only writes node_exporter textfiles.

It runs the read-only `snapraid smart` and `snapraid status` every
`--collect.interval` (default 15m) and caches the result. Scrapes never invoke
snapraid, and `sync`/`scrub` are never run.

## Install

    go build -o snapraid-exporter . && sudo install snapraid-exporter /usr/local/bin/
    sudo install -m644 contrib/snapraid-exporter.service /etc/systemd/system/
    sudo systemctl enable --now snapraid-exporter

## Flags

| flag | default |
|---|---|
| `--web.listen-address` | `:9634` |
| `--snapraid.bin` | `snapraid` |
| `--snapraid.config` | snapraid default (`/etc/snapraid.conf`) |
| `--collect.interval` | `15m` |
| `--collect.timeout` | `10m` |

## Metrics

`snapraid_smart_disk_{temperature_celsius,power_on_days,error_count,fail_probability,size_terabytes}`
(labels `disk,device,serial`), `snapraid_smart_array_fail_probability`,
`snapraid_disk_{files,fragmented_files,excess_fragments,wasted_bytes,used_bytes,free_bytes}{disk}`,
`snapraid_array_{files,used_bytes,free_bytes,errors}`,
`snapraid_scrub_{oldest,median,newest}_block_age_days`, `snapraid_unscrubbed_ratio`,
`snapraid_sync_in_progress`, `snapraid_zero_subsecond_files`, and
`snapraid_exporter_command_{success,exit_status,duration_seconds,last_run_timestamp_seconds}{command}`.

Parsers are tested against fixtures written from snapraid's documented output; verify against
your snapraid version with `curl :9634/metrics`.

## Container image

`ghcr.io/ashlcx/snapraid-exporter` bundles snapraid and smartctl. `main` publishes `:latest` and
`:sha-<sha>`; pushing a tag `vX.Y.Z` publishes `:X.Y.Z`, `:X.Y` (and `:X` from v1) and creates a
GitHub release. To run it, mount `/etc/snapraid.conf`, the array's disks at the same paths, and
pass the disk devices for SMART (`--privileged` or explicit `--device`s).

    git tag v0.1.0 && git push --tags
