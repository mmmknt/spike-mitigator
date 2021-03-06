package controllers

type Host string

type RoutingRule struct {
	RuleMap map[Host]*RoutingRate `json:",inline"`
}

func (rr *RoutingRule) Add(host Host, re *RoutingRate) {
	re.Host = host
	rr.RuleMap[host] = re
}

func (rr *RoutingRule) GetRoutingRule(host Host) *RoutingRate {
	if rr == nil || rr.RuleMap == nil {
		return nil
	}
	return rr.RuleMap[host]
}

func (rr *RoutingRule) Equal(target *RoutingRule) bool {
	return rr == target
}

type RoutingRate struct {
	// Host is only used for logging
	Host           Host   `json:"host"`
	InternalWeight int32  `json:"internalWeight"`
	ExternalWeight int32  `json:"externalWeight"`
	Version        string `json:"version"`
}
