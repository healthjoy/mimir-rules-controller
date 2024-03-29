/*
Copyright 2022 The mimir-rules-controller Authors.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/healthjoy/mimir-rules-controller/pkg/apis/rulescontroller/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// MimirRuleLister helps list MimirRules.
// All objects returned here must be treated as read-only.
type MimirRuleLister interface {
	// List lists all MimirRules in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.MimirRule, err error)
	// MimirRules returns an object that can list and get MimirRules.
	MimirRules(namespace string) MimirRuleNamespaceLister
	MimirRuleListerExpansion
}

// mimirRuleLister implements the MimirRuleLister interface.
type mimirRuleLister struct {
	indexer cache.Indexer
}

// NewMimirRuleLister returns a new MimirRuleLister.
func NewMimirRuleLister(indexer cache.Indexer) MimirRuleLister {
	return &mimirRuleLister{indexer: indexer}
}

// List lists all MimirRules in the indexer.
func (s *mimirRuleLister) List(selector labels.Selector) (ret []*v1alpha1.MimirRule, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.MimirRule))
	})
	return ret, err
}

// MimirRules returns an object that can list and get MimirRules.
func (s *mimirRuleLister) MimirRules(namespace string) MimirRuleNamespaceLister {
	return mimirRuleNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// MimirRuleNamespaceLister helps list and get MimirRules.
// All objects returned here must be treated as read-only.
type MimirRuleNamespaceLister interface {
	// List lists all MimirRules in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.MimirRule, err error)
	// Get retrieves the MimirRule from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.MimirRule, error)
	MimirRuleNamespaceListerExpansion
}

// mimirRuleNamespaceLister implements the MimirRuleNamespaceLister
// interface.
type mimirRuleNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all MimirRules in the indexer for a given namespace.
func (s mimirRuleNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.MimirRule, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.MimirRule))
	})
	return ret, err
}

// Get retrieves the MimirRule from the indexer for a given namespace and name.
func (s mimirRuleNamespaceLister) Get(name string) (*v1alpha1.MimirRule, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("mimirrule"), name)
	}
	return obj.(*v1alpha1.MimirRule), nil
}
