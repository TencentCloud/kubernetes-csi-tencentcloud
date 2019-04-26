/*
Copyright 2018 The Kubernetes Authors.

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

package util

// ForAllFunc is a unary predicate for visiting all cache entries
// matching the `pattern' in CachePersister's ForAll function.
type ForAllFunc func(identifier string) error

// CacheEntryNotFound is an error type for "Not Found" cache errors
type CacheEntryNotFound struct {
	error
}

// CachePersister interface implemented for store
type CachePersister interface {
	Create(identifier string, data interface{}) error
	Get(identifier string, data interface{}) error
	ForAll(pattern string, destObj interface{}, f ForAllFunc) error
	Delete(identifier string) error
}

// NewCachePersister returns K8sCachePersister
func NewCachePersister() CachePersister {
	k8scm := &K8sCMCache{}
	k8scm.Client = NewK8sClient()
	k8scm.Namespace = GetK8sNamespace()
	return k8scm
}

