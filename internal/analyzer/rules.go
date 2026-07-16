package analyzer

var registeredRules []Rule

func Rules() []Rule { return append([]Rule(nil), registeredRules...) }
