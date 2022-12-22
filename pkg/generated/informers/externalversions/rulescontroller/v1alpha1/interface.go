/*
Copyright 2022 The mimir-rules-controller Authors.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "github.com/healthjoy/mimir-rules-controller/pkg/generated/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// MimirRules returns a MimirRuleInformer.
	MimirRules() MimirRuleInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// MimirRules returns a MimirRuleInformer.
func (v *version) MimirRules() MimirRuleInformer {
	return &mimirRuleInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
