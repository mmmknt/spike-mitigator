package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/mmmknt/spike-mitigation-operator/api/v1"
)

//TODO test for calculate
// 1. single RoutingRule
// 1-1. create
// 1-2. update
// 1-2-1. current RoutingRule Host is equal to latest RoutingRule Host(update)
// 1-2-2. current RoutingRule Host isn't equal to latest RoutingRule Host(create and delete)
// 1-3. delete
// 1-4. nop
// 2. multi RoutingRule
// 2-1. create
// 2-2. update
// 2-2-1. current RoutingRules Host are equal to latest RoutingRules Host(update)
// 2-2-2. current RoutingRules Host aren't equal to latest RoutingRules Host(create and delete)
// 2-2-3. current RoutingRules Host are partial equal to latest RoutingRules Host(create, update and delete)
// 2-3. delete
// 2-4. nop
// 3. error cases

func TestUnableToGetMetrics(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-second"): {
			InternalWeight: int32(50),
			ExternalWeight: int32(50),
			Version:        "some-version",
		},
	}}
	maxCurrentCPUUtilizationPercentage := int32(100)

	routingRule, err := calculate(spec, rule, nil, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, rule, routingRule)
}

func TestFirstTimeOverMitigationTriggerRateAndGenerateSingleRoutingRule(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{}
	metrics := &Metrics{
		TotalRequestCount: 1000,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  100,
			Host("test-domain-second"): 100,
			Host("test-domain-third"):  700,
			Host("test-domain-fourth"): 100,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(100)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-third"): {
			InternalWeight: 58,
			ExternalWeight: 42,
			Version:        "",
		},
	}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}

func TestCurrentSingleRoutingRuleAndGenerateSameHostSingleRoutingRule(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-second"): {
			InternalWeight: int32(50),
			ExternalWeight: int32(50),
			Version:        "some-version",
		},
	}}
	metrics := &Metrics{
		TotalRequestCount: 1000,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  100,
			Host("test-domain-second"): 700,
			Host("test-domain-third"):  100,
			Host("test-domain-fourth"): 100,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(100)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-second"): {
			InternalWeight: 29,
			ExternalWeight: 71,
			Version:        "some-version",
		},
	}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}

func TestCurrentSingleRoutingRuleAndGenerateDifferentSingleRoutingRule(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-second"): {
			InternalWeight: int32(50),
			ExternalWeight: int32(50),
			Version:        "some-version",
		},
	}}
	metrics := &Metrics{
		TotalRequestCount: 1700,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  100,
			Host("test-domain-second"): 300,
			Host("test-domain-third"):  1200,
			Host("test-domain-fourth"): 100,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(100)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-third"): {
			InternalWeight: 33,
			ExternalWeight: 67,
			Version:        "",
		},
	}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}

func TestSufficientResourceAndRoutingRuleIsNotNeeded(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-first"): {
			InternalWeight: int32(50),
			ExternalWeight: int32(50),
			Version:        "some-version",
		},
		Host("test-domain-second"): {
			InternalWeight: int32(50),
			ExternalWeight: int32(50),
			Version:        "some-version",
		},
	}}
	metrics := &Metrics{
		TotalRequestCount: 1000,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  100,
			Host("test-domain-second"): 300,
			Host("test-domain-third"):  500,
			Host("test-domain-fourth"): 100,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(30)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}

func TestFirstTimeOverMitigationTriggerRateAndGenerateMultiRoutingRules(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{}
	metrics := &Metrics{
		TotalRequestCount: 1000,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  200,
			Host("test-domain-second"): 300,
			Host("test-domain-third"):  400,
			Host("test-domain-fourth"): 100,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(200)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-third"): {
			InternalWeight: 1,
			ExternalWeight: 99,
			Version:        "",
		},
		Host("test-domain-second"): {
			InternalWeight: 16,
			ExternalWeight: 84,
			Version:        "",
		},
	}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}

func TestCurrentMultiRoutingRulesAndGenerateMultiRoutingRules(t *testing.T) {
	spec := v1.MitigationRuleSpec{
		HPATriggerRate:        60,
		MitigationTriggerRate: 80,
	}
	rule := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-fourth"): {
			InternalWeight: int32(40),
			ExternalWeight: int32(60),
			Version:        "some-version",
		},
		Host("test-domain-first"): {
			InternalWeight: int32(80),
			ExternalWeight: int32(20),
			Version:        "other-version",
		},
	}}
	metrics := &Metrics{
		TotalRequestCount: 1000,
		RequestCountMap: map[Host]float64{
			Host("test-domain-first"):  100,
			Host("test-domain-second"): 300,
			Host("test-domain-third"):  400,
			Host("test-domain-fourth"): 200,
		},
	}
	maxCurrentCPUUtilizationPercentage := int32(150)

	expected := &RoutingRule{RuleMap: map[Host]*RoutingRate{
		Host("test-domain-fourth"): {
			InternalWeight: 1,
			ExternalWeight: 99,
			Version:        "some-version",
		},
		Host("test-domain-third"): {
			InternalWeight: 10,
			ExternalWeight: 90,
			Version:        "",
		},
	}}

	routingRule, err := calculate(spec, rule, metrics, maxCurrentCPUUtilizationPercentage)
	assert.NoError(t, err)
	assert.Equal(t, expected, routingRule)
}
