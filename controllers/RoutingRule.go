package controllers

type Host string

type RoutingRule struct {
	RuleMap map[Host]*RoutingRate
}

func (rr *RoutingRule) Add(host Host, re *RoutingRate) {
	rr.RuleMap[host] = re
}

func (rr *RoutingRule) GetRoutingRule(host Host) *RoutingRate {
	if rr == nil || rr.RuleMap == nil {
		return nil
	}
	return rr.RuleMap[host]
}

func (rr *RoutingRule) Equal(target RoutingRule) bool {
	// TODO
	return false
}

type RoutingRate struct {
	InternalWeight int32
	ExternalWeight int32
	Version        string
}
