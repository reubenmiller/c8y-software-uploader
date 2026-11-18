// c8y-software-uploader is a go-c8y-cli extension which uploads software packages to the
// Cumulocity software repository. It is based on the software-uploader example of go-c8y v2:
// https://github.com/reubenmiller/go-c8y/tree/v2/examples/software-uploader
//
// Build it into the extension folder and install it locally:
//
//	go build -o bin/c8y-software-uploader .
//	c8y extensions install .
//	c8y software-uploader upload --dir ./dist --pattern "*.deb" --dry
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/reubenmiller/go-c8y-cli/pkg/extsdk"
	"github.com/reubenmiller/go-c8y/v2/pkg/c8y/api"
	"github.com/reubenmiller/go-c8y/v2/pkg/c8y/api/authentication"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Session modes (see "c8y sessions" and the --sessionMode flag)
const (
	modeProd = "prod"
	modeQual = "qual"
)

type uploadOptions struct {
	dir          string
	file         string
	patterns     []string
	typeMappings []string
	softwareType string
	version      string
	namePrefix   string
	concurrency  int
	replace      bool
	noProgress   bool
	logLevel     string
	text         bool
}

func newUploadCmd() *cobra.Command {
	opts := &uploadOptions{}
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload software packages to the Cumulocity software repository",
		Long: `Upload software packages to the Cumulocity software repository.

The software name, version, architecture and type are parsed from the filenames.
A software item is created per name, architecture and type, and each file is
uploaded as a version of it. Existing versions are not changed, unless --replace
is used, so the command can be run again safely.

Use the c8y --dry flag to show what would be uploaded without changing anything.

The command writes one JSON result per version, which c8y formats (--output,
--select, views). Use --text to show a human readable report instead, which is
written to stderr while the upload runs.`,
		Example: `  c8y software-uploader upload --dir ./dist --pattern "*.deb" --dry
  # Show which Debian packages would be uploaded

  c8y software-uploader upload --dir ./dist --pattern "*.deb" --pattern "*.rpm" --pattern "*.apk"
  # Upload Debian, RPM and Alpine packages

  c8y software-uploader upload --file ./tedge_1.4.2_arm64.deb
  # Upload a single file

  c8y software-uploader upload --dir ./firmware --type-map "*.bin=firmware" --output table
  # Map files to a software type, and show the results as a table

  c8y software-uploader upload --dir ./dist --pattern "*.deb" --text
  # Show a human readable report instead of the JSON results`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(cmd.Context(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dir, "dir", "", "Directory to search (recursively) for software packages")
	flags.StringVar(&opts.file, "file", "", "Single software package to upload (alternative to --dir)")
	flags.StringArrayVar(&opts.patterns, "pattern", nil, "Glob pattern of the files to upload (repeatable). Default: *")
	flags.StringArrayVar(&opts.typeMappings, "type-map", nil, "Map a filename glob to a software type, e.g. '*.bin=firmware' (repeatable, first match wins)")
	flags.StringVar(&opts.softwareType, "type", "", "Software type of all packages, e.g. firmware or apt. Overrides the detected type")
	flags.StringVar(&opts.version, "version", "", "Version of all packages. Overrides the detected version")
	flags.StringVar(&opts.namePrefix, "name-prefix", "", "Prefix added to every software name")
	flags.IntVar(&opts.concurrency, "concurrency", 5, "Number of concurrent uploads (1-20)")
	flags.BoolVar(&opts.replace, "replace", false, "Replace the binary of versions which already exist")
	flags.BoolVar(&opts.noProgress, "no-progress", false, "Don't show the progress bar. It is not shown when stderr is not a terminal")
	flags.StringVar(&opts.logLevel, "log-level", "error", "Log level: error, warn, info or debug. debug also logs the HTTP requests")
	flags.BoolVar(&opts.text, "text", false, "Show a human readable report (on stderr) instead of the JSON results")
	cmd.MarkFlagsMutuallyExclusive("dir", "file")
	cmd.MarkFlagsOneRequired("dir", "file")
	_ = cmd.MarkFlagDirname("dir")
	_ = cmd.RegisterFlagCompletionFunc("log-level", cobra.FixedCompletions([]string{"error", "warn", "info", "debug"}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions([]string{"apt", "rpm", "apk", "opkg", "pacman", "firmware"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func runUpload(ctx context.Context, opts *uploadOptions) error {
	level, err := parseLogLevel(opts.logLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	typeMappings, err := parseTypeMappings(opts.typeMappings)
	if err != nil {
		return err
	}

	extCtx, err := extsdk.FromEnv()
	if err != nil {
		return err
	}
	dryRun := extCtx.DryRun()

	// text is nil when the JSON results are written
	var text *textReport
	if opts.text {
		text = &textReport{w: os.Stderr}
		text.Header()
		text.Scanning(opts)
	}

	files, err := collectFiles(opts)
	if err != nil {
		return err
	}
	if text != nil {
		text.Found(opts, files)
	}
	if len(files) == 0 {
		if text == nil {
			info("No files found matching pattern(s): %s", strings.Join(opts.patternsOrDefault(), ", "))
		}
		return nil
	}

	var infos []*SoftwareInfo
	skip := func(file string, err error) {
		if text != nil {
			text.Skipped(file, err)
		} else {
			slog.Warn("Skipping file", "file", file, "error", err)
		}
	}
	for _, f := range files {
		parsed, err := ParseSoftwareFromFilename(f, opts.softwareType, opts.namePrefix, typeMappings...)
		if err != nil {
			skip(f, err)
			continue
		}
		// The parser still runs, so that the version is removed from the name
		if opts.version != "" {
			parsed.Version = opts.version
		}
		if err := ValidateSoftwareInfo(parsed); err != nil {
			skip(f, err)
			continue
		}
		infos = append(infos, parsed)
	}
	if len(infos) == 0 {
		if text != nil {
			text.NoValidPackages()
		} else {
			info("No valid software packages found in %d file(s)", len(files))
		}
		return nil
	}

	summary := CreateSummary(infos)
	if text != nil {
		text.Plan(summary)
	} else {
		info("Found %d software package(s), %d version(s)", summary.TotalSoftware, summary.TotalVersions)
	}

	if dryRun {
		if text != nil {
			text.DryRunDone()
			return nil
		}
		return extsdk.Print(plannedResults(infos))
	}

	if err := checkMode(extCtx.Mode, opts.replace); err != nil {
		return err
	}
	if !extCtx.HasSession() {
		return errors.New("no active session. Activate a session with set-session, or use --session")
	}

	client := api.NewClient(api.ClientOptions{
		BaseURL:            extCtx.Host,
		Auth:               authentication.FromEnvironment(),
		InsecureSkipVerify: extCtx.Insecure,
		Debug:              level == slog.LevelDebug,
		Agent:              "c8y-software-uploader",
	})

	config := &UploadConfig{
		Client:      client,
		Concurrency: opts.concurrency,
		// The type is already applied by the parser. Only pass it on, so it is used
		// when creating the software items
		SoftwareType: opts.softwareType,
		Force:        opts.replace,
	}

	if text != nil {
		text.Uploading()
	}

	// stdout is used for the JSON output, so the progress bar is written to stderr
	showProgress := !opts.noProgress && term.IsTerminal(int(os.Stderr.Fd()))
	bar := progressbar.NewOptions(len(infos),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetDescription("Uploading"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetVisibility(showProgress),
	)

	start := time.Now()
	result := UploadSoftwareVersions(ctx, config, infos, func(completed, total int, currentFile string) {
		_ = bar.Set(completed)
	})
	_ = bar.Finish()
	elapsed := time.Since(start)

	versions := result.Versions
	if len(versions) == 0 && len(result.Errors) > 0 {
		// The software items could not be created (the errors are already logged), so
		// no version was processed. Report all versions as failed
		versions = failedResults(infos, result.Errors[0].Error)
	}

	failed := 0
	for _, v := range versions {
		if v.Status == StatusFailed {
			failed++
		}
	}

	if text != nil {
		text.Results(result, elapsed)
	} else {
		info("Processed %d version(s) in %.1fs: %d new, %d replaced, %d existing, %d failed",
			len(versions), elapsed.Seconds(),
			result.VersionsCreated, result.VersionsReplaced, result.VersionsFound, failed)
		if err := extsdk.Print(versions); err != nil {
			return err
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to upload %d version(s)", failed)
	}
	return nil
}

// failedResults returns all versions with the failed status and the given error
func failedResults(infos []*SoftwareInfo, err error) []VersionResult {
	results := plannedResults(infos)
	for i := range results {
		results[i].Status = StatusFailed
		results[i].Error = err.Error()
	}
	return results
}

func (o *uploadOptions) patternsOrDefault() []string {
	if len(o.patterns) == 0 {
		return []string{"*"}
	}
	return o.patterns
}

// collectFiles returns the files to upload, either the single --file or the files in --dir
// which match a pattern
func collectFiles(opts *uploadOptions) ([]string, error) {
	if opts.file != "" {
		stat, err := os.Stat(opts.file)
		if err != nil {
			return nil, fmt.Errorf("file not found: %s", opts.file)
		}
		if stat.IsDir() {
			return nil, fmt.Errorf("%s is a directory, use --dir instead", opts.file)
		}
		return []string{opts.file}, nil
	}

	stat, err := os.Stat(opts.dir)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s", opts.dir)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not a directory, use --file instead", opts.dir)
	}
	return findMatchingFiles(opts.dir, opts.patternsOrDefault())
}

// findMatchingFiles finds all files in the directory (recursively) which match any of the patterns
func findMatchingFiles(dir string, patterns []string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		filename := filepath.Base(path)
		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, filename)
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", pattern, err)
			}
			if matched {
				matches = append(matches, path)
				break
			}
		}
		return nil
	})
	return matches, err
}

// plannedResults returns the versions which would be uploaded, for a dry run
func plannedResults(infos []*SoftwareInfo) []VersionResult {
	results := make([]VersionResult, 0, len(infos))
	for _, info := range infos {
		results = append(results, NewVersionResult(info, StatusPlanned))
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Name != results[j].Name {
			return results[i].Name < results[j].Name
		}
		return results[i].Version < results[j].Version
	})
	return results
}

// checkMode checks that the session mode allows the upload. Uploads create data, which the
// prod mode does not allow. Replacing a version deletes its binary, which needs the dev or ci mode.
// c8y passes the session mode (respecting --sessionMode) in C8Y_SETTINGS_MODE
func checkMode(mode string, replace bool) error {
	switch strings.ToLower(mode) {
	case modeProd:
		return fmt.Errorf("uploading software is not allowed in the %q session mode. Use --sessionMode dev or qual, or change the session mode", mode)
	case modeQual:
		if replace {
			return fmt.Errorf("--replace deletes the existing binaries, which is not allowed in the %q session mode. Use --sessionMode dev, or change the session mode", mode)
		}
	}
	return nil
}

func parseTypeMappings(values []string) ([]TypeMapping, error) {
	mappings := make([]TypeMapping, 0, len(values))
	for _, value := range values {
		idx := strings.LastIndex(value, "=")
		if idx <= 0 || idx == len(value)-1 {
			return nil, fmt.Errorf("invalid type mapping %q: expected pattern=softwaretype (e.g. *.bin=firmware)", value)
		}
		mappings = append(mappings, TypeMapping{Pattern: value[:idx], SoftwareType: value[idx+1:]})
	}
	return mappings, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "error":
		return slog.LevelError, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	}
	return slog.LevelError, fmt.Errorf("invalid log level %q: expected error, warn, info or debug", value)
}

// info writes a message for the user to stderr, as stdout only contains the JSON output
func info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func main() {
	root := extsdk.NewRootCommand("c8y-software-uploader")
	root.Short = "Upload software packages to Cumulocity"
	root.AddCommand(newUploadCmd())
	extsdk.Execute(root)
}
