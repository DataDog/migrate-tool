package ksm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

var ksmTagMapping = map[string]string{
	"cluster_name":          "kube_cluster_name",
	"container":             "kube_container_name",
	"cronjob":               "kube_cronjob",
	"daemonset":             "kube_daemon_set",
	"deployment":            "kube_deployment",
	"hpa":                   "horizontalpodautoscaler",
	"image":                 "image_name",
	"job":                   "kube_job",
	"job_name":              "kube_job",
	"namespace":             "kube_namespace",
	"phase":                 "pod_phase",
	"pod":                   "pod_name",
	"replicaset":            "kube_replica_set",
	"replicationcontroller": "kube_replication_controller",
	"statefulset":           "kube_stateful_set",
}

var ksmMetricsReplacer = strings.NewReplacer(
	"kubernetes_state.nodes.by_condition", "kubernetes_state.node.by_condition",
	"kubernetes_state.persistentvolumes.by_phase", "kubernetes_state.persistentvolume.by_phase",
)

func ksmOriginalTags() []string {
	tags := make([]string, 0, len(ksmTagMapping))
	for k := range ksmTagMapping {
		tags = append(tags, k)
	}
	return tags
}

var ksmTagReplaceRegexp = regexp.MustCompile(
	strings.Replace(`[\{\, ](PLACEHOLDER)[\}\:\, ]`, "PLACEHOLDER", strings.Join(ksmOriginalTags(), "|"), 1),
)

var variableReferenceRegexp = regexp.MustCompile(`\$[a-zA-Z0-9_-]+`)

type patchResult struct {
	err     error
	patched string
}

func patchQueryString(query string, usedVariables map[string]struct{}) (res patchResult) {
	if !strings.Contains(query, "kubernetes_state.") {
		return
	}

	// Tracking template variables used by KSM queries
	if usedVariables != nil {
		varMatch := variableReferenceRegexp.FindAllString(query, -1)
		for _, v := range varMatch {
			usedVariables[v[1:]] = struct{}{}
		}
	}

	// Replace tags
	patchedQuery := ksmTagReplaceRegexp.ReplaceAllStringFunc(query, func(match string) string {
		tag := match[1 : len(match)-1]
		newTag, found := ksmTagMapping[tag]
		if !found {
			res.err = fmt.Errorf("matched tag but unable to find replacement %s", tag)
			return match
		}

		return string(match[0]) + newTag + string(match[len(match)-1])
	})
	if res.err != nil {
		return
	}

	// Replace metrics
	patchedQuery = ksmMetricsReplacer.Replace(patchedQuery)
	if patchedQuery != query {
		res.patched = patchedQuery
	}

	return
}

type requestWidgetInterface interface {
	GetQOk() (*string, bool)
	SetQ(v string)
	GetQueriesOk() (*[]datadogV1.FormulaAndFunctionQueryDefinition, bool)
}

func patchRequestWidget(reqWidget requestWidgetInterface, usedVariables map[string]struct{}) (bool, error) {
	widgetPatched := false

	if q, ok := reqWidget.GetQOk(); ok && q != nil {
		res := patchQueryString(*q, usedVariables)
		if res.err != nil {
			return widgetPatched, res.err
		}

		if res.patched != "" {
			widgetPatched = true
			reqWidget.SetQ(res.patched)
		}
	}

	if queries, ok := reqWidget.GetQueriesOk(); ok && queries != nil {
		for i := range *queries {
			query := &(*queries)[i]
			if query.FormulaAndFunctionMetricQueryDefinition != nil {
				res := patchQueryString(query.FormulaAndFunctionMetricQueryDefinition.Query, usedVariables)
				if res.err != nil {
					return widgetPatched, res.err
				}

				if res.patched != "" {
					widgetPatched = true
					query.FormulaAndFunctionMetricQueryDefinition.Query = res.patched
				}
			}
		}
	}

	return widgetPatched, nil
}

var templateVarRegexp = regexp.MustCompile(
	strings.Replace(`(PLACEHOLDER)\.name`, "PLACEHOLDER", strings.Join(ksmOriginalTags(), "|"), 1),
)

func patchTemplateString(templateString string) (res patchResult) {
	patched := false
	patchedTemplate := templateVarRegexp.ReplaceAllStringFunc(templateString, func(match string) string {
		tag := match[:len(match)-5]
		newTag, found := ksmTagMapping[tag]
		if !found {
			res.err = fmt.Errorf("matched tag but unable to find replacement %s", tag)
			return match
		} else {
			patched = true
		}

		return newTag + ".name"
	})

	if patched {
		res.patched = patchedTemplate
	}

	return
}
