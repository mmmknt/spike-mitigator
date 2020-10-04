package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	v1 "github.com/mmmknt/spike-mitigation-operator/api/v1"
)

type MitigationCalculator struct {
	KubernetesClientset *kubernetes.Clientset
	DDClient            *datadog.APIClient
}

func (c *MitigationCalculator) Calculate(ctx context.Context, log logr.Logger, currentRoutingRule *RoutingRule, spec v1.MitigationRuleSpec) (*RoutingRule, error) {
	// TODO make namespace changeable
	defaultNamespace := "default"
	hpaList, err := c.KubernetesClientset.AutoscalingV1().HorizontalPodAutoscalers(defaultNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	log.Info("succeed to get HPA list", "HPA", hpaList)

	maxCurrentCPUUtilizationPercentage := int32(0)
	for i := range hpaList.Items {
		// TODO make changeable target hpa
		item := hpaList.Items[i]
		is := item.Status
		if *is.CurrentCPUUtilizationPercentage >= maxCurrentCPUUtilizationPercentage {
			maxCurrentCPUUtilizationPercentage = *is.CurrentCPUUtilizationPercentage
		}
	}
	// 1. Scaling with proper load -> keep current RoutingRule
	if maxCurrentCPUUtilizationPercentage >= int32(spec.HPATriggerRate) && maxCurrentCPUUtilizationPercentage < int32(spec.MitigationTriggerRate) {
		return currentRoutingRule, nil
	}

	// 2. Scaling with too heavy or not scaling -> recalculate
	// TODO tuning metrics window
	to := time.Now().Unix()
	from := to - int64(30)
	// TODO be able to specify query
	query := "sum:http_server_request_count{*} by {http.host}.as_count()"
	ddCtx := context.WithValue(
		ctx,
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: spec.DDApiKey,
			},
			"appKeyAuth": {
				Key: spec.DDAppKey,
			},
		})
	metrics, err := c.getMetrics(ddCtx, log, query, from, to)
	if err != nil {
		return nil, err
	}
	return calculate(spec, currentRoutingRule, metrics, maxCurrentCPUUtilizationPercentage)
}

func (c *MitigationCalculator) getMetrics(ctx context.Context, log logr.Logger, query string, from, to int64) (*Metrics, error) {
	resp, _, err := c.DDClient.MetricsApi.QueryMetrics(ctx).Query(query).From(from).To(to).Execute()
	if err != nil {
		return nil, err
	}
	log.Info("succeed to get metrics", "resp.Series", resp.Series)
	// calculate total and max request count and host
	totalCount := float64(0)
	maxHost := ""
	maxCount := float64(0)
	rcMap := make(map[Host]float64)
	for _, se := range *resp.Series {
		log.Info("se", "se", se)
		pl, ok := se.GetPointlistOk()
		if !ok {
			break
		}
		latestTimestamp := float64(0)
		latestCount := float64(0)
		for _, point := range *pl {
			if point[0] > latestTimestamp {
				latestTimestamp = point[0]
				latestCount = point[1]
			}
		}
		scope := se.GetScope()
		host := strings.TrimPrefix(scope, "http.host:")
		if latestCount > maxCount {
			maxHost = host
			maxCount = latestCount
		}
		rcMap[Host(host)] = latestCount
		totalCount += latestCount
	}
	var metrics *Metrics
	if maxHost != "" {
		metrics = &Metrics{
			MaxHost:           maxHost,
			TotalRequestCount: totalCount,
			RequestCountMap:   rcMap,
		}
	}

	return metrics, nil
}

func calculate(spec v1.MitigationRuleSpec, currentRule *RoutingRule, metrics *Metrics, maxCPUUtilizationPercentage int32) (*RoutingRule, error) {
	if metrics == nil {
		return &RoutingRule{RuleMap: map[Host]*RoutingRate{}}, nil
	}
	maxHost := metrics.MaxHost
	totalRequestCount := metrics.TotalRequestCount
	rcMap := metrics.RequestCountMap
	// TODO calculate latest routing rule
	deltaPercentage := float64(maxCPUUtilizationPercentage) - float64(spec.MitigationTriggerRate+spec.HPATriggerRate)/2
	externalRequestCount := float64(0)
	for host, rate := range currentRule.RuleMap {
		if rate.ExternalWeight != 0 {
			// TODO calculate total request count
			externalRequestCount += rcMap[host] * float64(rate.ExternalWeight) / float64(rate.InternalWeight)
		}
	}
	externalRequestCount += deltaPercentage * totalRequestCount / 100
	f := rcMap[Host(maxHost)]
	maxrr := currentRule.GetRoutingRule(Host(maxHost))
	if maxrr != nil {
		f = f * float64(100/maxrr.InternalWeight)
	}
	iw := int32((f - externalRequestCount) * 100 / f)
	if iw >= 100 {
		iw = 100
	}
	ew := 100 - iw
	//log.Info("calculated result", "maxHost", maxHost, "internalWeight", iw, "externalWeight", ew)
	fmt.Printf("calculated result. maxHost: %v, internalWeight: %v, externalWeight: %v", maxHost, iw, ew)
	rr := &RoutingRule{RuleMap: map[Host]*RoutingRate{}}
	if iw != 100 {
		version := ""
		if maxrr != nil {
			version = maxrr.Version
		}
		rr.RuleMap = map[Host]*RoutingRate{
			Host(maxHost): {
				InternalWeight: iw,
				ExternalWeight: ew,
				Version:        version,
			},
		}
	}
	return rr, nil
}

type Metrics struct {
	MaxHost           string           `json:"maxHost"`
	TotalRequestCount float64          `json:"totalRequestCount"`
	RequestCountMap   map[Host]float64 `json:",inline"`
}
