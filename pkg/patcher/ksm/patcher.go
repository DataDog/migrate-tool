package ksm

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/migrate-tool/pkg/config"
)

type Patcher struct{}

func (Patcher) PatchMonitor(_ context.Context, _ config.Config, monitor *datadogV1.Monitor) (bool, error) {
	res := patchQueryString(monitor.Query, nil)
	if res.err != nil {
		return false, res.err
	}

	if res.patched == "" {
		return false, nil
	}
	monitor.Query = res.patched

	// If we patched the query, we may need to update title and message
	if monitor.Name != nil {
		res := patchTemplateString(*monitor.Name)
		if res.err != nil {
			return false, res.err
		}

		if res.patched != "" {
			monitor.Name = &res.patched
		}
	}

	if monitor.Message != nil {
		res := patchTemplateString(*monitor.Message)
		if res.err != nil {
			return false, res.err
		}

		if res.patched != "" {
			monitor.Message = &res.patched
		}
	}

	return true, nil
}

func (Patcher) PatchDashboard(_ context.Context, _ config.Config, dashboard *datadogV1.Dashboard) (bool, error) {
	// Tracking template variables used by KSM queries
	usedTemplateVariables := make(map[string]struct{})

	dashboardPatched, err := patchWidgets(dashboard.Widgets, usedTemplateVariables)
	if err != nil {
		return false, err
	}

	for varIndex := range dashboard.TemplateVariables {
		variable := &dashboard.TemplateVariables[varIndex]

		currentPrefix := variable.Prefix.Get()
		if _, found := usedTemplateVariables[variable.Name]; found && currentPrefix != nil {
			if newVal, found := ksmTagMapping[*currentPrefix]; found {
				variable.Prefix.Set(&newVal)
				dashboardPatched = true
			}
		}
	}

	return dashboardPatched, nil
}

func patchWidgets(widgets []datadogV1.Widget, usedTemplateVariables map[string]struct{}) (bool, error) {
	widgetsPatched := false

	for widgetIndex := range widgets {
		widget := &widgets[widgetIndex]

		if widget.Definition.GroupWidgetDefinition != nil {
			patched, err := patchWidgets(widget.Definition.GroupWidgetDefinition.Widgets, usedTemplateVariables)
			if err != nil {
				return false, err
			}

			widgetsPatched = widgetsPatched || patched
		}

		if widget.Definition.ChangeWidgetDefinition != nil {
			for reqIndex := range widget.Definition.ChangeWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.ChangeWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}

		if widget.Definition.TableWidgetDefinition != nil {
			for reqIndex := range widget.Definition.TableWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.TableWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}

		if widget.Definition.QueryValueWidgetDefinition != nil {
			for reqIndex := range widget.Definition.QueryValueWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.QueryValueWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}

		if widget.Definition.TimeseriesWidgetDefinition != nil {
			for reqIndex := range widget.Definition.TimeseriesWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.TimeseriesWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}

		if widget.Definition.ToplistWidgetDefinition != nil {
			for reqIndex := range widget.Definition.ToplistWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.ToplistWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}

		if widget.Definition.TreeMapWidgetDefinition != nil {
			for reqIndex := range widget.Definition.TreeMapWidgetDefinition.Requests {
				patched, err := patchRequestWidget(&widget.Definition.TreeMapWidgetDefinition.Requests[reqIndex], usedTemplateVariables)
				if err != nil {
					return false, err
				}

				widgetsPatched = widgetsPatched || patched
			}
		}
	}

	return widgetsPatched, nil
}
