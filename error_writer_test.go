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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentio/scalpel/internal/assert"
)

func TestErrorWriter(t *testing.T) {
	t.Parallel()
	t.Run("Protocols", func(t *testing.T) {
		t.Parallel()
		writer := NewErrorWriter() // All supported by default
		t.Run("GRPC", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://localhost", nil)
			req.Header.Set("Content-Type", grpcContentTypeDefault)
			assert.True(t, writer.IsSupported(req))
			req.Header.Set("Content-Type", grpcContentTypePrefix+"json")
			assert.True(t, writer.IsSupported(req))
		})
	})
	t.Run("UnknownCodec", func(t *testing.T) {
		// An Unknown codec should return supported as the protocol is known and
		// the error codec is agnostic to the codec used. The server can respond
		// with a protocol error for the unknown codec.
		t.Parallel()
		writer := NewErrorWriter()
		unknownCodec := "invalid"
		t.Run("GRPC", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://localhost", nil)
			req.Header.Set("Content-Type", grpcContentTypePrefix+unknownCodec)
			assert.True(t, writer.IsSupported(req))
		})
	})
}
