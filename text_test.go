package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTextReportPlan(t *testing.T) {
	var out strings.Builder
	report := &textReport{w: &out}
	infos := []*SoftwareInfo{
		{Name: "tedge", Version: "1.4.2", Architecture: "arm64", SoftwareType: "apt"},
		{Name: "tedge", Version: "1.4.1", Architecture: "arm64", SoftwareType: "apt"},
		{Name: "tedge", Version: "1.4.0", Architecture: "arm64", SoftwareType: "apt"},
		{Name: "tedge", Version: "1.3.0", Architecture: "arm64", SoftwareType: "apt"},
		{Name: "plugin", Version: "0.3.0", SoftwareType: "apt"},
	}
	report.Plan(CreateSummary(infos))

	assert.Contains(t, out.String(), "  • plugin [apt]: 1 version(s) [0.3.0]\n")
	assert.Contains(t, out.String(), "  • tedge [arm64/apt]: 4 version(s) [1.3.0, 1.4.0, 1.4.1, ... and 1 more]\n")
	assert.Contains(t, out.String(), "📊 Summary: 2 software package(s), 5 version(s) total")
}

func TestTextReportResults(t *testing.T) {
	var out strings.Builder
	report := &textReport{w: &out}
	report.Results(&UploadResult{
		SuccessCount:    2,
		FailureCount:    1,
		VersionsCreated: 1,
		VersionsFound:   1,
		Errors:          []UploadError{{Name: "tedge", Version: "1.4.2", Error: errors.New("boom")}},
	}, 1500*time.Millisecond)

	assert.Contains(t, out.String(), "✅ Successfully processed 2 version(s)")
	assert.Contains(t, out.String(), "   📤 Newly uploaded: 1\n")
	assert.Contains(t, out.String(), "  • tedge v1.4.2: boom\n")
	assert.Contains(t, out.String(), "⏱️  Total time: 1.5s")
	assert.Contains(t, out.String(), "Total: 2 processed (1 new, 0 replaced, 1 existing), 1 failed")
}
