// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scalpel

import (
	"strings"
)

const (
	compressionIdentity = "identity"
)

type compressionPool struct {
}

// readOnlyCompressionPools is a read-only interface to a map of named
// compressionPools.
type readOnlyCompressionPools interface {
	Get(string) *compressionPool
	Contains(string) bool
	// Wordy, but clarifies how this is different from readOnlyCodecs.Names().
	CommaSeparatedNames() string
}

func newReadOnlyCompressionPools(
	nameToPool map[string]*compressionPool,
	reversedNames []string,
) readOnlyCompressionPools {
	// Client and handler configs keep compression names in registration order,
	// but we want the last registered to be the most preferred.
	names := make([]string, 0, len(reversedNames))
	seen := make(map[string]struct{}, len(reversedNames))
	for i := len(reversedNames) - 1; i >= 0; i-- {
		name := reversedNames[i]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return &namedCompressionPools{
		nameToPool:          nameToPool,
		commaSeparatedNames: strings.Join(names, ","),
	}
}

type namedCompressionPools struct {
	nameToPool          map[string]*compressionPool
	commaSeparatedNames string
}

func (m *namedCompressionPools) Get(name string) *compressionPool {
	if name == "" || name == compressionIdentity {
		return nil
	}
	return m.nameToPool[name]
}

func (m *namedCompressionPools) Contains(name string) bool {
	_, ok := m.nameToPool[name]
	return ok
}

func (m *namedCompressionPools) CommaSeparatedNames() string {
	return m.commaSeparatedNames
}
