package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/voxis/backend/internal/port"
	"github.com/voxis/backend/internal/service"
)

type fakeDataPurger struct {
	planCalls, purgeCalls int
	report                service.PurgeReport
}

func (f *fakeDataPurger) Plan(context.Context, string) (service.PurgePlan, error) {
	f.planCalls++
	return f.report.PurgePlan, nil
}

func (f *fakeDataPurger) Purge(context.Context, string) (service.PurgeReport, error) {
	f.purgeCalls++
	return f.report, nil
}

func TestParsePurgeUserArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    purgeUserOptions
		wantErr bool
	}{
		{name: "dry run", args: []string{"--subject", "sub-1"}, want: purgeUserOptions{subject: "sub-1"}},
		{name: "confirm", args: []string{"--subject=sub-1", "--confirm"}, want: purgeUserOptions{subject: "sub-1", confirm: true}},
		{name: "missing subject", args: []string{"--confirm"}, wantErr: true},
		{name: "blank subject", args: []string{"--subject", "  "}, wantErr: true},
		{name: "stray argument", args: []string{"--subject", "sub-1", "extra"}, wantErr: true},
		{name: "unknown flag", args: []string{"--subject", "sub-1", "--force"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePurgeUserArgs(test.args, io.Discard)
			if (err != nil) != test.wantErr {
				t.Fatalf("parsePurgeUserArgs(%q) error = %v, wantErr %t", test.args, err, test.wantErr)
			}
			if !test.wantErr && got != test.want {
				t.Fatalf("parsePurgeUserArgs(%q) = %+v, want %+v", test.args, got, test.want)
			}
		})
	}
}

func TestExecutePurgeUserDryRunNeverPurges(t *testing.T) {
	purger := &fakeDataPurger{report: service.PurgeReport{PurgePlan: service.PurgePlan{
		UserID: "sub-1", OrganizationID: "org-1", Counts: port.PurgeCounts{Media: 3, APIKeys: 1},
	}}}
	var out bytes.Buffer
	if err := executePurgeUser(context.Background(), purger, purgeUserOptions{subject: "sub-1"}, &out); err != nil {
		t.Fatal(err)
	}
	if purger.purgeCalls != 0 || purger.planCalls != 1 {
		t.Fatalf("plan calls = %d, purge calls = %d; want a plan only", purger.planCalls, purger.purgeCalls)
	}
	for _, want := range []string{"media: 3", "API keys: 1", "Dry run: nothing was deleted"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output %q does not contain %q", out.String(), want)
		}
	}
}

func TestExecutePurgeUserConfirmReportsPendingProviderDeletions(t *testing.T) {
	purger := &fakeDataPurger{report: service.PurgeReport{
		PurgePlan:   service.PurgePlan{UserID: "sub-1", OrganizationID: "org-1"},
		MediaPurged: 2, PendingProviderDeletions: 1,
	}}
	var out bytes.Buffer
	if err := executePurgeUser(context.Background(), purger, purgeUserOptions{subject: "sub-1", confirm: true}, &out); err != nil {
		t.Fatal(err)
	}
	if purger.purgeCalls != 1 {
		t.Fatalf("purge calls = %d, want 1", purger.purgeCalls)
	}
	for _, want := range []string{"2 media purged", "not yet confirmed deletion of 1 job", "re-run"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output %q does not contain %q", out.String(), want)
		}
	}

	purger.report.OrganizationDeleted, purger.report.PendingProviderDeletions = true, 0
	out.Reset()
	if err := executePurgeUser(context.Background(), purger, purgeUserOptions{subject: "sub-1", confirm: true}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "records deleted") || !strings.Contains(out.String(), "until they rotate out") {
		t.Fatalf("output %q does not report the deletion and the backup copies", out.String())
	}
	if strings.Contains(out.String(), "transit key") {
		t.Fatalf("output %q still suggests deleting the transit key, which backups keep", out.String())
	}
}

func TestRunPurgeUserRejectsBadArgumentsWithUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := runPurgeUser([]string{"--confirm"}, io.Discard, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), purgeUserUsage) {
		t.Fatalf("stderr %q does not contain usage", stderr.String())
	}
}
