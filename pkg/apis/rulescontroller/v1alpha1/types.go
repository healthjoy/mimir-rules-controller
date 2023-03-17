package v1alpha1

import (
	"github.com/grafana/mimir/pkg/mimirtool/rules"
	"github.com/grafana/mimir/pkg/mimirtool/rules/rwrulefmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ConditionType is a valid value for Condition.Type
type ConditionType string

// Duration is a wrapper around string that can hold a duration string.
type Duration string

const (
	// ConditionTypeReady means the condition is in ready state
	ConditionTypeReady ConditionType = "Ready"
	// ConditionTypeFailed means the condition is in failed state
	ConditionTypeFailed ConditionType = "Failed"
)

const (
	// RuleFinalizer is the name of the finalizer added to Rule objects
	RuleFinalizer = "mimirrule.finalizers.k8s.healthjoy.com"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MimirRule is a specification for a MimirRule resource
type MimirRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleSpec   `json:"spec"`
	Status RuleStatus `json:"status"`
}

// RuleSpec is the spec for a MimirRule resource
type RuleSpec struct {
	Groups []RuleGroup `json:"groups"`
}

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
type RuleGroup struct {
	Name            string   `json:"name"`
	Interval        string   `json:"interval,omitempty"`
	EvaluationDelay string   `json:"evaluation_delay,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Rules           []Rule   `json:"rules"`
	SourceTenants   []string `json:"source_tenants,omitempty"`
}

// Rule is a recording or alerting rule.
type Rule struct {
	Record      string             `json:"record,omitempty"`
	Alert       string             `json:"alert,omitempty"`
	Expr        intstr.IntOrString `json:"expr"`
	For         Duration           `json:"for,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

// RuleStatus is the status for a MimirRule resource
type RuleStatus struct {
	Conditions []metav1.Condition `json:"conditions"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MimirRuleList is a list of MimirRule resources
type MimirRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MimirRule `json:"items"`
}

// GetMimirRuleNamespace converts a RuleSpec to a rwrulefmt.RuleGroup
func (mr *RuleSpec) GetMimirRuleNamespace(namespace string) (ruleNs *rules.RuleNamespace, err error) {
	ruleNs = &rules.RuleNamespace{}
	ruleNs.Namespace = namespace
	ruleNs.Groups = make([]rwrulefmt.RuleGroup, len(mr.Groups))
	for groupIdx, group := range mr.Groups {
		groupPtr := &ruleNs.Groups[groupIdx]

		groupPtr.Name = group.Name
		groupPtr.Limit = group.Limit
		groupPtr.SourceTenants = group.SourceTenants

		if group.Interval != "" {
			groupPtr.Interval, err = model.ParseDuration(group.Interval)
			if err != nil {
				return nil, err
			}
		}

		if group.EvaluationDelay != "" {
			*groupPtr.EvaluationDelay, err = model.ParseDuration(group.EvaluationDelay)
			if err != nil {
				return nil, err
			}
		}

		groupPtr.Rules = make([]rulefmt.RuleNode, len(group.Rules))
		for ruleIndex, rule := range group.Rules {
			rulePtr := &ruleNs.Groups[groupIdx].Rules[ruleIndex]

			rulePtr.Record = yaml.Node{Kind: yaml.ScalarNode, Value: rule.Record}
			rulePtr.Alert = yaml.Node{Kind: yaml.ScalarNode, Value: rule.Alert}
			rulePtr.Expr = yaml.Node{Kind: yaml.ScalarNode, Value: rule.Expr.String()}
			rulePtr.Labels = rule.Labels
			rulePtr.Annotations = rule.Annotations
			if rule.For != "" {
				rulePtr.For, err = model.ParseDuration(string(rule.For))
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return ruleNs, nil
}
