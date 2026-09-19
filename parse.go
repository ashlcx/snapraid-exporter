package main

import (
	"regexp"
	"strconv"
	"strings"
)

type smartDisk struct {
	Name        string
	Device      string
	Serial      string
	TempC       float64
	HasTemp     bool
	PowerOnDays float64
	HasDays     bool
	ErrorCount  float64
	HasErrors   bool
	FailProb    float64 // 0..1
	HasFailProb bool
	SizeTB      float64
	HasSize     bool
}

type smartReport struct {
	Disks         []smartDisk
	ArrayFailProb float64 // 0..1
	HasArrayProb  bool
}

var arrayProbRe = regexp.MustCompile(`at least one disk is going to fail in the next year is (\d+)%`)

func num(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// parseSmart parses `snapraid smart` output. Data rows end in "<device> <disk>"
// where device starts with /dev/ (or is "-" for unknown).
func parseSmart(out string) smartReport {
	var r smartReport
	for _, line := range strings.Split(out, "\n") {
		if m := arrayProbRe.FindStringSubmatch(line); m != nil {
			p, _ := num(m[1])
			r.ArrayFailProb, r.HasArrayProb = p/100, true
			continue
		}
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		dev := f[len(f)-2]
		if !strings.HasPrefix(dev, "/dev/") && dev != "-" {
			continue
		}
		d := smartDisk{Name: f[len(f)-1], Device: dev, Serial: strings.Join(f[5:len(f)-2], " ")}
		d.TempC, d.HasTemp = num(f[0])
		d.PowerOnDays, d.HasDays = num(f[1])
		d.ErrorCount, d.HasErrors = num(f[2])
		if p, ok := num(strings.TrimSuffix(f[3], "%")); ok && strings.HasSuffix(f[3], "%") {
			d.FailProb, d.HasFailProb = p/100, true
		}
		d.SizeTB, d.HasSize = num(f[4])
		r.Disks = append(r.Disks, d)
	}
	return r
}

type statusDisk struct {
	Name                                  string
	Files, Fragmented, ExcessFragments    float64
	WastedGB, UsedGB, FreeGB, UsedPercent float64
}

type statusReport struct {
	Disks              []statusDisk
	Total              *statusDisk
	ScrubOldestDays    float64
	ScrubMedianDays    float64
	ScrubNewestDays    float64
	HasScrubAge        bool
	UnscrubbedPercent  float64
	HasUnscrubbed      bool
	Errors             float64
	SyncInProgress     bool
	ZeroSubsecondFiles float64
	NoErrorsReported   bool
}

var (
	scrubAgeRe   = regexp.MustCompile(`oldest block was scrubbed (\d+) days? ago, the median (\d+), the newest (\d+)`)
	unscrubbedRe = regexp.MustCompile(`The (\d+)% of the array is not scrubbed`)
	errorsRe     = regexp.MustCompile(`there are (\d+) errors?`)
	zeroSubsecRe = regexp.MustCompile(`You have (\d+) files? with zero sub-second`)
	syncActiveRe = regexp.MustCompile(`A sync is in progress`)
)

// parseStatus parses `snapraid status` output.
func parseStatus(out string) statusReport {
	var r statusReport
	for _, line := range strings.Split(out, "\n") {
		if m := scrubAgeRe.FindStringSubmatch(line); m != nil {
			r.ScrubOldestDays, _ = num(m[1])
			r.ScrubMedianDays, _ = num(m[2])
			r.ScrubNewestDays, _ = num(m[3])
			r.HasScrubAge = true
			continue
		}
		if m := unscrubbedRe.FindStringSubmatch(line); m != nil {
			r.UnscrubbedPercent, _ = num(m[1])
			r.HasUnscrubbed = true
			continue
		}
		if m := errorsRe.FindStringSubmatch(line); m != nil {
			r.Errors, _ = num(m[1])
			continue
		}
		if m := zeroSubsecRe.FindStringSubmatch(line); m != nil {
			r.ZeroSubsecondFiles, _ = num(m[1])
			continue
		}
		if syncActiveRe.MatchString(line) {
			r.SyncInProgress = true
			continue
		}
		if strings.Contains(line, "No error detected") {
			r.NoErrorsReported = true
			continue
		}
		// Table rows: 7 numeric columns (last is "NN%"), then an optional disk name.
		f := strings.Fields(line)
		if len(f) != 7 && len(f) != 8 || !strings.HasSuffix(f[6], "%") {
			continue
		}
		var v [7]float64
		ok := true
		for i := 0; i < 7; i++ {
			var good bool
			v[i], good = num(strings.TrimSuffix(f[i], "%"))
			ok = ok && good
		}
		if !ok {
			continue
		}
		d := statusDisk{Files: v[0], Fragmented: v[1], ExcessFragments: v[2], WastedGB: v[3], UsedGB: v[4], FreeGB: v[5], UsedPercent: v[6]}
		if len(f) == 8 {
			d.Name = f[7]
			r.Disks = append(r.Disks, d)
		} else {
			r.Total = &d
		}
	}
	return r
}
