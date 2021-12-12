/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v2beta1

import (
	v2beta1 "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ProviderHostLister helps list ProviderHosts.
// All objects returned here must be treated as read-only.
type ProviderHostLister interface {
	// List lists all ProviderHosts in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v2beta1.ProviderHost, err error)
	// ProviderHosts returns an object that can list and get ProviderHosts.
	ProviderHosts(namespace string) ProviderHostNamespaceLister
	ProviderHostListerExpansion
}

// providerHostLister implements the ProviderHostLister interface.
type providerHostLister struct {
	indexer cache.Indexer
}

// NewProviderHostLister returns a new ProviderHostLister.
func NewProviderHostLister(indexer cache.Indexer) ProviderHostLister {
	return &providerHostLister{indexer: indexer}
}

// List lists all ProviderHosts in the indexer.
func (s *providerHostLister) List(selector labels.Selector) (ret []*v2beta1.ProviderHost, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v2beta1.ProviderHost))
	})
	return ret, err
}

// ProviderHosts returns an object that can list and get ProviderHosts.
func (s *providerHostLister) ProviderHosts(namespace string) ProviderHostNamespaceLister {
	return providerHostNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// ProviderHostNamespaceLister helps list and get ProviderHosts.
// All objects returned here must be treated as read-only.
type ProviderHostNamespaceLister interface {
	// List lists all ProviderHosts in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v2beta1.ProviderHost, err error)
	// Get retrieves the ProviderHost from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v2beta1.ProviderHost, error)
	ProviderHostNamespaceListerExpansion
}

// providerHostNamespaceLister implements the ProviderHostNamespaceLister
// interface.
type providerHostNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all ProviderHosts in the indexer for a given namespace.
func (s providerHostNamespaceLister) List(selector labels.Selector) (ret []*v2beta1.ProviderHost, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v2beta1.ProviderHost))
	})
	return ret, err
}

// Get retrieves the ProviderHost from the indexer for a given namespace and name.
func (s providerHostNamespaceLister) Get(name string) (*v2beta1.ProviderHost, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v2beta1.Resource("providerhost"), name)
	}
	return obj.(*v2beta1.ProviderHost), nil
}
