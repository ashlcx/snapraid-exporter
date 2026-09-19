package main

import (
	"strings"
	"testing"
)

const smartFixture = `SnapRAID SMART report:

   Temp  Power   Error   FP Size
      C OnDays   Count        TB  Serial       Device    Disk
 -----------------------------------------------------------------------
     34    282       0   2%  4.0  WD-ABC123    /dev/sdb  d1
     31    900       3  15% 12.0  ZR5-XYZ 9    /dev/sdc  d2
      -      -       -   -   0.5  -            /dev/sdd  d3

The FP column is the estimated probability (in percentage) that the disk
is going to fail in the next year.

Probability that at least one disk is going to fail in the next year is 17%.
`

func TestParseSmart(t *testing.T) {
	r := parseSmart(smartFixture)
	if len(r.Disks) != 3 {
		t.Fatalf("disks=%d", len(r.Disks))
	}
	d := r.Disks[1]
	if d.Name != "d2" || d.Device != "/dev/sdc" || d.Serial != "ZR5-XYZ 9" || d.ErrorCount != 3 || d.FailProb != 0.15 || d.SizeTB != 12 {
		t.Errorf("bad d2: %+v", d)
	}
	if u := r.Disks[2]; u.HasTemp || u.HasFailProb || !u.HasSize {
		t.Errorf("bad unknown disk: %+v", u)
	}
	if !r.HasArrayProb || r.ArrayFailProb != 0.17 {
		t.Errorf("array prob: %+v", r)
	}
}

const statusFixture = `Self test...
Loading state from /mnt/disk1/.snapraid.content...
SnapRAID status report:

   Files Fragmented Excess  Wasted  Used    Free  Use Name
            Files  Fragments  GB      GB      GB
      12       2       5     0.0    100     200  33% d1
      40       0       0     1.5    300     100  75% d2
 --------------------------------------------------------------------------
      52       2       5     1.5    400     300  57%

 3%|o                                             *
   |o                                             o
 0%|_____________________________________________
   38                  days ago of the last scrub/sync                   0

The oldest block was scrubbed 38 days ago, the median 12, the newest 0.

WARNING! The 5% of the array is not scrubbed.
You have 2 files with zero sub-second timestamp.
No rehash is in progress or needed.
DANGER! In the array there are 4 errors!
`

func TestParseStatus(t *testing.T) {
	r := parseStatus(statusFixture)
	if len(r.Disks) != 2 || r.Total == nil {
		t.Fatalf("disks=%d total=%v", len(r.Disks), r.Total)
	}
	if d := r.Disks[1]; d.Name != "d2" || d.UsedGB != 300 || d.WastedGB != 1.5 || d.UsedPercent != 75 {
		t.Errorf("bad d2: %+v", d)
	}
	if r.Total.Files != 52 {
		t.Errorf("total: %+v", r.Total)
	}
	if !r.HasScrubAge || r.ScrubOldestDays != 38 || r.ScrubMedianDays != 12 {
		t.Errorf("scrub: %+v", r)
	}
	if r.UnscrubbedPercent != 5 || r.Errors != 4 || r.ZeroSubsecondFiles != 2 || r.SyncInProgress {
		t.Errorf("misc: %+v", r)
	}
}

func TestRender(t *testing.T) {
	s := &state{res: map[string]cmdResult{"smart": {ok: true}, "status": {ok: true}}}
	s.smart, s.status = parseSmart(smartFixture), parseStatus(statusFixture)
	out := s.render()
	for _, want := range []string{
		`snapraid_smart_disk_fail_probability{disk="d2",device="/dev/sdc",serial="ZR5-XYZ 9"} 0.15`,
		`snapraid_disk_used_bytes{disk="d1"} 1e+11`,
		"snapraid_unscrubbed_ratio 0.05",
		"snapraid_array_errors 4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
