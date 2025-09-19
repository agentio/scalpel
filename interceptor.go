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
	"context"
)

// UnaryFunc is the generic signature of a unary RPC.
//
// The type of the request and response structs depend on the codec being used.
// When using Protobuf, request.Any() and response.Any() will always be
// [proto.Message] implementations.
type UnaryFunc func(context.Context, AnyRequest) (AnyResponse, error)

// StreamingClientFunc is the generic signature of a streaming RPC from the client's
// perspective.
type StreamingClientFunc func(context.Context, Spec) StreamingClientConn

// StreamingHandlerFunc is the generic signature of a streaming RPC from the
// handler's perspective.
type StreamingHandlerFunc func(context.Context, StreamingHandlerConn) error
