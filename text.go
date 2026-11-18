package main

// Text output (--text). This is the report of the software-uploader example of go-c8y v2:
// https://github.com/reubenmiller/go-c8y/tree/v2/examples/software-uploader
//
// The report is written to stderr. c8y collects the stdout of commands declared with
// "output: json" until the command exits, so stdout would only show the report at the end.

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// textReport writes the human readable report
type textReport struct {
	w io.Writer
}

func (r *textReport) printf(format string, args ...any) {
	fmt.Fprintf(r.w, format, args...)
}

// Header prints the application header
func (r *textReport) Header() {
	r.printf("╔══════════════════════════════════════════╗\n")
	r.printf("║   Software Package Uploader for C8Y      ║\n")
	r.printf("╚══════════════════════════════════════════╝\n\n")
}

// Scanning prints the directory or file which is processed
func (r *textReport) Scanning(opts *uploadOptions) {
	if opts.file != "" {
		r.printf("📦 Processing file: %s\n\n", opts.file)
		return
	}
	r.printf("🔍 Scanning directory: %s\n", opts.dir)
}

// Found prints the number of files which were found in the directory
func (r *textReport) Found(opts *uploadOptions, files []string) {
	if opts.file != "" {
		return
	}
	patterns := strings.Join(opts.patternsOrDefault(), ", ")
	if len(files) == 0 {
		r.printf("❌ No files found matching pattern(s): %s\n", patterns)
		return
	}
	r.printf("📦 Found %d file(s) matching pattern(s): %s\n\n", len(files), patterns)
}

// Skipped prints a file which is not uploaded
func (r *textReport) Skipped(file string, err error) {
	r.printf("Warning: skipping %s: %v\n", file, err)
}

// NoValidPackages prints that none of the files can be uploaded
func (r *textReport) NoValidPackages() {
	r.printf("❌ No valid software packages found\n")
}

// Plan prints a summary of what will be uploaded
func (r *textReport) Plan(summary *SoftwareSummary) {
	r.printf("📋 Upload Plan:\n")

	keys := make([]string, 0, len(summary.Groups))
	for key := range summary.Groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		versions := summary.Groups[key]
		if len(versions) == 0 {
			continue
		}

		versionStrs := make([]string, len(versions))
		for i, v := range versions {
			versionStrs[i] = v.Version
		}
		sort.Strings(versionStrs)

		// The name, architecture and type are the same for all versions of a group
		name := versions[0].Name
		arch := versions[0].Architecture
		softwareType := versions[0].SoftwareType
		qualifier := ""
		switch {
		case arch != "" && softwareType != "":
			qualifier = fmt.Sprintf(" [%s/%s]", arch, softwareType)
		case arch != "":
			qualifier = fmt.Sprintf(" [%s]", arch)
		case softwareType != "":
			qualifier = fmt.Sprintf(" [%s]", softwareType)
		}

		if len(versionStrs) <= 3 {
			r.printf("  • %s%s: %d version(s) [%s]\n", name, qualifier, len(versions), strings.Join(versionStrs, ", "))
		} else {
			r.printf("  • %s%s: %d version(s) [%s, ... and %d more]\n",
				name, qualifier, len(versions), strings.Join(versionStrs[:3], ", "), len(versionStrs)-3)
		}
	}

	r.printf("\n📊 Summary: %d software package(s), %d version(s) total\n\n",
		summary.TotalSoftware, summary.TotalVersions)
}

// DryRunDone prints that nothing was changed
func (r *textReport) DryRunDone() {
	r.printf("✓ Dry run complete - no changes made\n")
}

// Uploading prints the start of the upload
func (r *textReport) Uploading() {
	r.printf("📤 Uploading versions...\n")
}

// Results prints the upload results
func (r *textReport) Results(result *UploadResult, elapsed time.Duration) {
	r.printf("\n")
	if result.SuccessCount > 0 {
		r.printf("✅ Successfully processed %d version(s)\n", result.SuccessCount)
		if result.VersionsCreated > 0 {
			r.printf("   📤 Newly uploaded: %d\n", result.VersionsCreated)
		}
		if result.VersionsReplaced > 0 {
			r.printf("   🔄 Replaced: %d\n", result.VersionsReplaced)
		}
		if result.VersionsFound > 0 {
			r.printf("   ♻️  Already existed: %d\n", result.VersionsFound)
		}
	}

	if result.FailureCount > 0 {
		r.printf("❌ Failed to upload %d version(s):\n", result.FailureCount)
		for _, err := range result.Errors {
			switch {
			case err.Name != "" && err.Version != "":
				r.printf("  • %s v%s: %v\n", err.Name, err.Version, err.Error)
			case err.FilePath != "":
				r.printf("  • %s: %v\n", err.FilePath, err.Error)
			default:
				r.printf("  • %v\n", err.Error)
			}
		}
	}

	r.printf("\n⏱️  Total time: %.1fs\n", elapsed.Seconds())

	r.printf("\n%s\n", strings.Repeat("─", 50))
	r.printf("Total: %d processed (%d new, %d replaced, %d existing), %d failed\n",
		result.SuccessCount, result.VersionsCreated, result.VersionsReplaced, result.VersionsFound, result.FailureCount)
}
