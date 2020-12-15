package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"

	api "github.com/mmmknt/spike-mitigator/api/v1alpha1"
)

const metricsQueryTemplate = "sum:%s{cluster-name:%s} by {http.host}.as_rate()"

type MitigationCalculator struct {
	KubernetesClientset *kubernetes.Clientset
	DDClient            *datadog.APIClient
	SecretLister        corelisters.SecretLister
}

func (c *MitigationCalculator) Calculate(ctx context.Context, log logr.Logger, currentRoutingRule *RoutingRule, spec api.BalancingRuleSpec) (*RoutingRule, error) {
	maxCurrentCPUUtilizationPercentage := int32(0)
	for _, hpaName := range spec.MonitoredHPANames {
		hpa, err := c.KubernetesClientset.AutoscalingV1().HorizontalPodAutoscalers(spec.MonitoredHPANamespace).Get(ctx, hpaName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		log.Info("succeed to get HPA", "HPA", hpa)
		is := hpa.Status
		if *is.CurrentCPUUtilizationPercentage >= maxCurrentCPUUtilizationPercentage {
			maxCurrentCPUUtilizationPercentage = *is.CurrentCPUUtilizationPercentage
		}
	}

	// 1. Scaling with proper load -> keep current RoutingRule
	if maxCurrentCPUUtilizationPercentage >= spec.HPATriggerRate && maxCurrentCPUUtilizationPercentage < spec.MitigationTriggerRate {
		return currentRoutingRule, nil
	}

	// 2. Scaling with too heavy or not scaling -> recalculate
	sn := spec.SecretNamespace
	apiKeyRef := spec.MetricsStoreSecretRef.DDApiKeyRef
	ddApiKey, err := c.getSecretValue(sn, apiKeyRef.Name, apiKeyRef.Key)
	if err != nil {
		return nil, err
	}
	appKeyRef := spec.MetricsStoreSecretRef.DDAppKeyRef
	ddAppKey, err := c.getSecretValue(sn, appKeyRef.Name, appKeyRef.Key)
	if err != nil {
		return nil, err
	}

	// TODO tuning metrics window
	to := time.Now().Unix()
	from := to - int64(30)
	query := fmt.Sprintf(metricsQueryTemplate, spec.MetricsCondition.MetricsName, spec.MetricsCondition.ClusterName)
	ddCtx := context.WithValue(
		ctx,
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: ddApiKey,
			},
			"appKeyAuth": {
				Key: ddAppKey,
			},
		})
	metrics, err := c.getMetrics(ddCtx, log, query, from, to)
	if err != nil {
		return nil, err
	}
	return calculate(spec, currentRoutingRule, metrics, maxCurrentCPUUtilizationPercentage)
}

func (c *MitigationCalculator) getSecretValue(namespace, name, key string) (string, error) {
	secret, err := c.SecretLister.Secrets(namespace).Get(name)
	if err != nil {
		return "", err
	}
	bytes, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret data is not found with namespace %s, name %s and key %s", namespace, name, key)
	}
	return string(bytes), nil
}

func (c *MitigationCalculator) getMetrics(ctx context.Context, log logr.Logger, query string, from, to int64) (*Metrics, error) {
	resp, _, err := c.DDClient.MetricsApi.QueryMetrics(ctx).Query(query).From(from).To(to).Execute()
	if err != nil {
		return nil, err
	}
	log.Info("succeed to get metrics", "resp.Series", resp.Series)
	// calculate total and max request count and host
	totalCount := float64(0)
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
		var host string
		for _, tag := range strings.Split(scope, ",") {
			trimmed := strings.TrimPrefix(tag, "http.host:")
			if tag != trimmed {
				host = trimmed
				break
			}
		}
		if len(host) == 0 {
			return nil, fmt.Errorf("unable to specify host with metrics scope, %s", scope)
		}
		rcMap[Host(host)] = latestCount
		totalCount += latestCount
	}
	var metrics *Metrics
	if totalCount != 0 {
		metrics = &Metrics{
			TotalRequestCount: totalCount,
			RequestCountMap:   rcMap,
		}
	} else {
		metrics = nil
	}

	return metrics, nil
}

func calculate(spec api.BalancingRuleSpec, currentRule *RoutingRule, metrics *Metrics, maxCPUUtilizationPercentage int32) (*RoutingRule, error) {
	if metrics == nil {
		return currentRule, nil
	}

	receivableRequestCount := metrics.TotalRequestCount * (float64(spec.MitigationTriggerRate+spec.HPATriggerRate) / 2) / float64(maxCPUUtilizationPercentage)
	allRequestCount := float64(0)
	var allRequestCountSlice []RequestCount
	for host, rc := range metrics.RequestCountMap {
		count := rc
		rr := currentRule.RuleMap[host]
		if rr != nil {
			count += count * float64(rr.ExternalWeight) / float64(rr.InternalWeight)
		}
		allRequestCountSlice = append(allRequestCountSlice, RequestCount{
			Host:  host,
			Count: count,
		})
		allRequestCount += count
	}
	sort.Slice(allRequestCountSlice, func(i, j int) bool {
		return allRequestCountSlice[i].Count > allRequestCountSlice[j].Count
	})

	rr := &RoutingRule{RuleMap: map[Host]*RoutingRate{}}
	dc := allRequestCount - receivableRequestCount
	for _, rc := range allRequestCountSlice {
		if dc <= 0 {
			break
		}
		count := rc.Count
		externalCount := count
		if dc < externalCount {
			externalCount = dc
		}
		externalWeight := int32(externalCount / count * 100)
		// When external weight is 100, we are not able to detect all request counts.
		// So some request is needed to send internal.
		if externalWeight == 100 {
			externalWeight = 99
			externalCount = count * float64(externalWeight) / 100
		}
		dc = dc - externalCount
		rate := &RoutingRate{
			Host:           rc.Host,
			InternalWeight: 100 - externalWeight,
			ExternalWeight: externalWeight,
		}
		if rule := currentRule.RuleMap[rc.Host]; rule != nil {
			rate.Version = rule.Version
		}
		rr.RuleMap[rc.Host] = rate
	}
	return rr, nil
}

type Metrics struct {
	TotalRequestCount float64          `json:"totalRequestCount"`
	RequestCountMap   map[Host]float64 `json:",inline"`
}

type RequestCount struct {
	Host  `json:"host"`
	Count float64 `json:"count"`
}
