package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/spf13/cobra"

	"github.com/DataDog/migrate-tool/pkg/config"
	"github.com/DataDog/migrate-tool/pkg/patcher/ksm"
)

type patcher interface {
	PatchMonitor(context.Context, config.Config, *datadogV1.Monitor) (bool, error)
	PatchDashboard(context.Context, config.Config, *datadogV1.Dashboard) (bool, error)
}

func newPatchCommand(config *config.Config) *cobra.Command {
	var inputDirectory, patcherID string

	cmd := &cobra.Command{
		Use:   "patch [input files]",
		Short: "Patch all specified datadog objects in input files using selected patcher",
		RunE: func(cmd *cobra.Command, args []string) error {
			var patcher patcher

			switch strings.ToLower(patcherID) {
			case "ksm-to-core":
				patcher = ksm.Patcher{}
			default:
				cmd.Usage()
				return fmt.Errorf("missing or unknown patcher %s", patcherID)
			}

			return patch(cmd.Context(), *config, inputDirectory, patcher)
		},
	}

	cmd.Flags().StringVarP(&patcherID, "patcher", "p", "", "Name of the patcher to use")
	cmd.Flags().StringVarP(&inputDirectory, "input", "i", "objects", "Input folder")

	return cmd
}

type patchOutput struct {
	failedPaths  []error
	patchedPaths []string
}

func patch(ctx context.Context, cfg config.Config, inputDirectory string, patcher patcher) error {
	output := patchOutput{}
	err := filepath.WalkDir(inputDirectory, func(path string, d fs.DirEntry, err error) error {
		// If d is nil, it means we were not able to go into the root directory.
		if d == nil {
			return err
		}

		// Skip non-regular files.
		if !d.Type().IsRegular() {
			return nil
		}

		// If we fail on a file, we just record the error and continue.
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to walk file at %s, err: %w", path, err))
			return nil
		}

		// Check file ext and parse file name
		objType, _, ext, err := parseObjectFileName(d.Name())
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to parse file name at %s, err: %w", path, err))
			return nil
		}
		if ext != jsonExt {
			return nil
		}

		// We can now process the file.
		content, err := os.ReadFile(path)
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to read file at %s, err: %w", path, err))
			return nil
		}

		// Apply the patch
		var object any
		var patched bool
		switch objType {
		case dashboardObjType:
			dashboard := &datadogV1.Dashboard{}
			object = dashboard
			patched, err = callPatcher(ctx, cfg, content, dashboard, patcher.PatchDashboard)

		case monitorObjType:
			monitor := &datadogV1.Monitor{}
			object = monitor
			patched, err = callPatcher(ctx, cfg, content, monitor, patcher.PatchMonitor)
		}
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to patch object at %s, err: %w", path, err))
			return nil
		}

		// Write the patched object back to FS
		if patched {
			newContent, err := json.MarshalIndent(object, "", "\t")
			if err != nil {
				output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to marshal patched object at %s, err: %w", path, err))
				return nil
			}
			err = os.WriteFile(path, newContent, 0o660)
			if err != nil {
				output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to write patched object at %s, err: %w", path, err))
				return nil
			}

			touchedPath := strings.Replace(path, jsonExt, touchedExt, 1)
			err = os.WriteFile(touchedPath, []byte{}, 0o660)
			if err != nil {
				// We still report err at `path` and not `touchedPath`
				output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to write touched file at %s, err: %w", path, err))
				return nil
			}

			output.patchedPaths = append(output.patchedPaths, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nFinished patching %s\n", inputDirectory)
	fmt.Printf("Patched files: %d\n", len(output.patchedPaths))
	fmt.Printf("Patched failures: %d\n", len(output.failedPaths))
	for _, err := range output.failedPaths {
		fmt.Println(err)
	}
	fmt.Println()

	if len(output.failedPaths) > 0 {
		return fmt.Errorf("failed to patch some objects")
	}
	return nil
}

type patchFunc[T any] func(context.Context, config.Config, *T) (bool, error)

func callPatcher[T any](ctx context.Context, cfg config.Config, content []byte, obj *T, patchFunc patchFunc[T]) (bool, error) {
	if err := json.Unmarshal(content, obj); err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON, err: %w", err)
	}

	patched, err := patchFunc(ctx, cfg, obj)
	if err != nil {
		return false, fmt.Errorf("failed to patch object, err: %w", err)
	}

	return patched, nil
}
