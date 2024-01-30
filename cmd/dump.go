package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/spf13/cobra"

	"github.com/DataDog/migrate-tool/pkg/client"
	"github.com/DataDog/migrate-tool/pkg/config"
)

func newDumpCommand(config *config.Config) *cobra.Command {
	var dashboardFilePath, monitorFilePath, outputDirectory string
	var updateExisting bool

	cmd := &cobra.Command{
		Use:   "dump [input files]",
		Short: "Dump all specified datadog objects in input files",
		RunE: func(cmd *cobra.Command, args []string) error {
			atLeastOne := false

			if dashboardFilePath != "" {
				atLeastOne = true
				if err := dump(cmd.Context(), *config, dashboardFilePath, outputDirectory, updateExisting, dashboardDumper{}); err != nil {
					return err
				}
			}

			if monitorFilePath != "" {
				atLeastOne = true
				if err := dump(cmd.Context(), *config, monitorFilePath, outputDirectory, updateExisting, monitorDumper{}); err != nil {
					return err
				}
			}

			if !atLeastOne {
				cmd.Usage()
				return fmt.Errorf("At least one input file necessary")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&dashboardFilePath, "dashboards", "d", "", "Path to the dashboard source file")
	cmd.Flags().StringVarP(&monitorFilePath, "monitors", "m", "", "Path to the monitor source file")
	cmd.Flags().StringVarP(&outputDirectory, "output", "o", "objects", "Output folder")
	cmd.Flags().BoolVarP(&updateExisting, "update-existing", "u", false, "Update existing objects from Datadog API")

	return cmd
}

type serializedRef struct {
	OrgID       int    `json:"ORG_ID"`
	MonitorID   int    `json:"MONITOR_ID,omitempty"`
	DashboardID string `json:"DASHBOARD_ID,omitempty"`
}

func objectRefFromInputRef(serializedRef serializedRef) (objectRef, error) {
	objectRef := objectRef{OrgID: serializedRef.OrgID}

	switch {
	case serializedRef.MonitorID != 0:
		objectRef.Type = monitorObjType
		objectRef.ID = strconv.Itoa(serializedRef.MonitorID)
	case serializedRef.DashboardID != "":
		objectRef.Type = dashboardObjType
		objectRef.ID = serializedRef.DashboardID
	default:
		return objectRef, fmt.Errorf("invalid input ref: %+v", serializedRef)
	}

	return objectRef, nil
}

type serializedRefs []serializedRef

type objectDumper interface {
	objType() string
	dump(context.Context, config.Config, *datadog.APIClient, serializedRef) (any, error)
}

type dumpOutput struct {
	failedRefs   []error
	existingRefs serializedRefs
	dumpedRefs   serializedRefs
}

func dump(ctx context.Context, cfg config.Config, inputFilePath string, baseOutputDir string, updateExisting bool, dumper objectDumper) error {
	content, err := os.ReadFile(inputFilePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", inputFilePath, err)
	}

	sRefs := serializedRefs{}
	if err := json.Unmarshal(content, &sRefs); err != nil {
		return fmt.Errorf("failed to unmarshal file %s: %w", inputFilePath, err)
	}

	// First pass to create folders in case it fails, we can fail early
	outputDirs := make(map[string]struct{})
	for _, sRef := range sRefs {
		outputDirs[filepath.Join(baseOutputDir, strconv.Itoa(sRef.OrgID))] = struct{}{}
	}
	for outputDir := range outputDirs {
		if err := os.MkdirAll(outputDir, 0o770); err != nil {
			return fmt.Errorf("failed to create output folder %s: %w", outputDir, err)
		}
	}

	// Second pass to dump objects
	output := dumpOutput{}
	datadogClient := client.Datadog()
	for i, sRef := range sRefs {
		objectRef, err := objectRefFromInputRef(sRef)
		if err != nil {
			output.failedRefs = append(output.failedRefs, fmt.Errorf("failed to parse input ref: %+v: %w", sRef, err))
			continue
		}

		if i%20 == 0 {
			log.Println("Progressing, dumping", objectRef.Type, "object", i, "out of", len(sRefs))
		}

		localPath := filepath.Join(baseOutputDir, objectFilePath(sRef.OrgID, objectRef.Type, objectRef.ID))
		if !updateExisting {
			_, err := os.Stat(localPath)
			if err == nil {
				output.existingRefs = append(output.existingRefs, sRef)
				continue
			}
		}

		// Set proper creds
		credCtx, err := client.DatadogCredentials(ctx, cfg, sRef.OrgID)
		if err != nil {
			output.failedRefs = append(output.failedRefs, fmt.Errorf("%w object type: %s, id: %s", err, objectRef.Type, objectRef.ID))
			continue
		}

		obj, err := dumper.dump(credCtx, cfg, datadogClient, sRef)
		if err != nil {
			output.failedRefs = append(output.failedRefs, fmt.Errorf("failed to process object from org: %d, type: %s, id: %s: %w", sRef.OrgID, objectRef.Type, objectRef.ID, err))
			continue
		}

		objBytes, err := json.MarshalIndent(obj, "", "\t")
		if err != nil {
			output.failedRefs = append(output.failedRefs, fmt.Errorf("failed to marshal object from org: %d, type: %s, id: %s: %w", sRef.OrgID, objectRef.Type, objectRef.ID, err))
			continue
		}

		err = os.WriteFile(localPath, objBytes, 0o660)
		if err != nil {
			output.failedRefs = append(output.failedRefs, fmt.Errorf("failed to write object from org: %d, type: %s, id: %s: %w", sRef.OrgID, objectRef.Type, objectRef.ID, err))
			continue
		}
		output.dumpedRefs = append(output.dumpedRefs, sRef)
	}

	// Print results to stdout
	fmt.Printf("\nFinished dumping %s\n", dumper.objType())
	fmt.Printf("Existing refs: %d\n", len(output.existingRefs))
	fmt.Printf("Dumped refs: %d\n", len(output.dumpedRefs))
	fmt.Printf("Failed refs: %d\n", len(output.failedRefs))
	for _, err := range output.failedRefs {
		fmt.Println(err)
	}

	if len(output.failedRefs) > 0 {
		return fmt.Errorf("failed to dump some objects")
	}
	return nil
}

// Dashboards
type dashboardDumper struct{}

func (dashboardDumper) objType() string {
	return dashboardObjType
}

func (dashboardDumper) dump(ctx context.Context, config config.Config, client *datadog.APIClient, ref serializedRef) (any, error) {
	dashAPI := datadogV1.NewDashboardsApi(client)

	dashboard, _, err := dashAPI.GetDashboard(ctx, ref.DashboardID)
	if err != nil {
		return nil, err
	}

	return dashboard, nil
}

// Monitors
type monitorDumper struct{}

func (monitorDumper) objType() string {
	return monitorObjType
}

func (monitorDumper) dump(ctx context.Context, config config.Config, client *datadog.APIClient, ref serializedRef) (any, error) {
	monitorAPI := datadogV1.NewMonitorsApi(client)

	monitor, _, err := monitorAPI.GetMonitor(ctx, int64(ref.MonitorID))
	if err != nil {
		return nil, err
	}

	return monitor, nil
}
