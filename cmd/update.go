package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/migrate-tool/pkg/client"
	"github.com/DataDog/migrate-tool/pkg/config"
)

func newUpdateCommand(config *config.Config) *cobra.Command {
	var inputDirectory string
	var updateAll bool

	cmd := &cobra.Command{
		Use:   "update [input files]",
		Short: "Update all (touched) files in input directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return update(cmd.Context(), *config, inputDirectory, updateAll)
		},
	}

	cmd.Flags().StringVarP(&inputDirectory, "input", "i", "objects", "Input folder")
	cmd.Flags().BoolVarP(&updateAll, "update-all", "u", false, "Update all files, not just touched ones")

	return cmd
}

type updateOutput struct {
	failedPaths  []error
	updatedPaths []string
}

func update(ctx context.Context, cfg config.Config, inputDirectory string, updateAll bool) error {
	output := updateOutput{}

	currentDir := ""
	filesToUpdate := map[objectRef]string{}
	err := filepath.WalkDir(inputDirectory, func(path string, d fs.DirEntry, err error) error {
		// If d is nil, it means we were not able to go into the root directory.
		if d == nil {
			return err
		}

		if d.IsDir() {
			currentDir = d.Name()
			return nil
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

		if updateAll || filepath.Ext(d.Name()) == touchedExt {
			objectRef, err := objectRefFromFile(currentDir, d.Name())
			if err != nil {
				output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to parse file name %s/%s, err: %w", currentDir, d.Name(), err))
				return nil
			}

			if _, found := filesToUpdate[objectRef]; !found {
				jsonPath := strings.Replace(path, touchedExt, jsonExt, 1)
				filesToUpdate[objectRef] = jsonPath
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	datadogClient := client.Datadog()
	dashAPI := datadogV1.NewDashboardsApi(datadogClient)
	monitorsAPI := datadogV1.NewMonitorsApi(datadogClient)

	i := 0
	for ref, path := range filesToUpdate {
		if i%10 == 0 {
			log.Println("Progressing, updating", ref.Type, "object", i, "out of", len(filesToUpdate))
		}
		i++

		// Set proper creds
		ctx, err = client.DatadogCredentials(ctx, cfg, ref.OrgID)
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("%w object type: %s, id: %s", err, ref.Type, ref.ID))
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			output.failedPaths = append(output.failedPaths, fmt.Errorf("failed to read file at %s, err: %w", path, err))
			continue
		}

		switch ref.Type {
		case dashboardObjType:
			dashboard := &datadogV1.Dashboard{}
			err := json.Unmarshal(content, dashboard)

			_, _, err = dashAPI.UpdateDashboard(ctx, ref.ID, *dashboard)
			if err != nil {
				err = fmt.Errorf("failed to update dashboard %s, err: %w", ref.ID, err)
			}

		case monitorObjType:
			monitor := &datadogV1.Monitor{}
			err := json.Unmarshal(content, monitor)

			intID, err := strconv.Atoi(ref.ID)
			if err != nil {
				err = fmt.Errorf("failed to parse monitor ID %s, err: %w", ref.ID, err)
			}

			_, _, err = monitorsAPI.UpdateMonitor(ctx, int64(intID), datadogV1.MonitorUpdateRequest{
				Query:   &monitor.Query,
				Name:    monitor.Name,
				Message: monitor.Message,
			})
			if err != nil {
				err = fmt.Errorf("failed to update monitor %s, err: %w", ref.ID, err)
			}

		default:
			err = fmt.Errorf("invalid object type: %s", ref.Type)
		}

		// Update was successful, we can remove the touched file
		if err == nil {
			output.updatedPaths = append(output.updatedPaths, path)
		}
	}

	fmt.Printf("\nFinished updating\n")
	fmt.Printf("Updated objects: %d\n", len(output.updatedPaths))
	fmt.Printf("Update failures: %d\n", len(output.failedPaths))
	for _, err := range output.failedPaths {
		fmt.Println(err)
	}
	fmt.Println()

	if len(output.failedPaths) > 0 {
		return fmt.Errorf("failed to patch some objects")
	}
	return nil
}
