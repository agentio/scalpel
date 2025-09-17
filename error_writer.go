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
	"strings"
)

// protocolType is one of the supported RPC protocols.
type protocolType uint8

const (
	unknownProtocol protocolType = iota
	grpcProtocol
)

// An ErrorWriter writes errors to an [http.ResponseWriter] in the format
// expected by an RPC client. This is especially useful in server-side net/http
// middleware, where you may wish to handle requests from RPC and non-RPC
// clients with the same code.
//
// ErrorWriters are safe to use concurrently.
type ErrorWriter struct {
	bufferPool                   *bufferPool
	protobuf                     Codec
	requireConnectProtocolHeader bool
}

// NewErrorWriter constructs an ErrorWriter. Handler options may be passed to
// configure the error writer behaviour to match the handlers.
// [WithRequiredConnectProtocolHeader] will assert that Connect protocol
// requests include the version header allowing the error writer to correctly
// classify the request.
// Options supplied via [WithConditionalHandlerOptions] are ignored.
func NewErrorWriter(opts ...HandlerOption) *ErrorWriter {
	config := newHandlerConfig("", StreamTypeUnary, opts)
	codecs := newReadOnlyCodecs(config.Codecs)
	return &ErrorWriter{
		bufferPool:                   config.BufferPool,
		protobuf:                     codecs.Protobuf(),
		requireConnectProtocolHeader: config.RequireConnectProtocolHeader,
	}
}

func (w *ErrorWriter) classifyRequest(request *http.Request) protocolType {
	ctype := canonicalizeContentType(getHeaderCanonical(request.Header, headerContentType))
	isPost := request.Method == http.MethodPost
	switch {
	case isPost && (ctype == grpcContentTypeDefault || strings.HasPrefix(ctype, grpcContentTypePrefix)):
		return grpcProtocol
	default:
		return unknownProtocol
	}
}

// IsSupported checks whether a request is using one of the ErrorWriter's
// supported RPC protocols.
func (w *ErrorWriter) IsSupported(request *http.Request) bool {
	return w.classifyRequest(request) != unknownProtocol
}

// Write an error, using the format appropriate for the RPC protocol in use.
// Callers should first use IsSupported to verify that the request is using one
// of the ErrorWriter's supported RPC protocols. If the protocol is unknown,
// Write will send the error as unprefixed, Connect-formatted JSON.
//
// Write does not read or close the request body.
func (w *ErrorWriter) Write(response http.ResponseWriter, request *http.Request, err error) error {
	ctype := canonicalizeContentType(getHeaderCanonical(request.Header, headerContentType))
	switch protocolType := w.classifyRequest(request); protocolType {
	case grpcProtocol:
		setHeaderCanonical(response.Header(), headerContentType, ctype)
		return w.writeGRPC(response, err)
	case unknownProtocol:
		fallthrough
	default:
		return nil
	}
}

func (w *ErrorWriter) writeGRPC(response http.ResponseWriter, err error) error {
	trailers := make(http.Header, 2) // need space for at least code & message
	grpcErrorToTrailer(trailers, w.protobuf, err)
	// To make net/http reliably send trailers without a body, we must set the
	// Trailers header rather than using http.TrailerPrefix. See
	// https://github.com/golang/go/issues/54723.
	keys := make([]string, 0, len(trailers))
	for k := range trailers {
		keys = append(keys, k)
	}
	setHeaderCanonical(response.Header(), headerTrailer, strings.Join(keys, ","))
	response.WriteHeader(http.StatusOK)
	mergeHeaders(response.Header(), trailers)
	return nil
}
