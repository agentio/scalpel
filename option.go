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
	"net/http"
)

// A ClientOption configures a [Client].
//
// In addition to any options grouped in the documentation below, remember that
// any [Option] is also a valid ClientOption.
type ClientOption interface {
	applyToClient(*clientConfig)
}

// WithClientOptions composes multiple ClientOptions into one.
func WithClientOptions(options ...ClientOption) ClientOption {
	return &clientOptionsOption{options}
}

// WithGRPC configures clients to use the HTTP/2 gRPC protocol.
func WithGRPC() ClientOption {
	return &grpcOption{}
}

// A HandlerOption configures a [Handler].
//
// In addition to any options grouped in the documentation below, remember that
// any [Option] is also a HandlerOption.
type HandlerOption interface {
	applyToHandler(*handlerConfig)
}

// WithHandlerOptions composes multiple HandlerOptions into one.
func WithHandlerOptions(options ...HandlerOption) HandlerOption {
	return &handlerOptionsOption{options}
}

// WithRecover adds an interceptor that recovers from panics. The supplied
// function receives the context, [Spec], request headers, and the recovered
// value (which may be nil). It must return an error to send back to the
// client. It may also log the panic, emit metrics, or execute other
// error-handling logic. Handler functions must be safe to call concurrently.
//
// To preserve compatibility with [net/http]'s semantics, this interceptor
// doesn't handle panics with [http.ErrAbortHandler].
//
// By default, handlers don't recover from panics. Because the standard
// library's [http.Server] recovers from panics by default, this option isn't
// usually necessary to prevent crashes. Instead, it helps servers collect
// RPC-specific data during panics and send a more detailed error to
// clients.
func WithRecover(handle func(context.Context, Spec, http.Header, any) error) HandlerOption {
	return WithInterceptors(&recoverHandlerInterceptor{handle: handle})
}

// WithConditionalHandlerOptions allows procedures in the same service to have
// different configurations: for example, one procedure may need a much larger
// WithReadMaxBytes setting than the others.
//
// WithConditionalHandlerOptions takes a function which may inspect each
// procedure's Spec before deciding which options to apply. Returning a nil
// slice is safe.
func WithConditionalHandlerOptions(conditional func(spec Spec) []HandlerOption) HandlerOption {
	return &conditionalHandlerOptions{conditional: conditional}
}

// Option implements both [ClientOption] and [HandlerOption], so it can be
// applied both client-side and server-side.
type Option interface {
	ClientOption
	HandlerOption
}

// WithSchema provides a parsed representation of the schema for an RPC to a
// client or handler. The supplied schema is exposed as [Spec.Schema]. This
// option is typically added by generated code.
//
// For services using protobuf schemas, the supplied schema should be a
// [protoreflect.MethodDescriptor].
func WithSchema(schema any) Option {
	return &schemaOption{Schema: schema}
}

// WithRequestInitializer provides a function that initializes a new message.
// It may be used to dynamically construct request messages. It is called on
// server receives to construct the message to be unmarshaled into. The message
// will be a non nil pointer to the type created by the handler. Use the Schema
// field of the [Spec] to determine the type of the message.
func WithRequestInitializer(initializer func(spec Spec, message any) error) HandlerOption {
	return &initializerOption{Initializer: initializer}
}

// WithResponseInitializer provides a function that initializes a new message.
// It may be used to dynamically construct response messages. It is called on
// client receives to construct the message to be unmarshaled into. The message
// will be a non nil pointer to the type created by the client. Use the Schema
// field of the [Spec] to determine the type of the message.
func WithResponseInitializer(initializer func(spec Spec, message any) error) ClientOption {
	return &initializerOption{Initializer: initializer}
}

// WithCodec registers a serialization method with a client or handler.
// Handlers may have multiple codecs registered, and use whichever the client
// chooses. Clients may only have a single codec.
//
// By default, handlers and clients support binary Protocol Buffer data using
// [google.golang.org/protobuf/proto].
//
// Registering a codec with an empty name is a no-op.
func WithCodec(codec Codec) Option {
	return &codecOption{Codec: codec}
}

// WithReadMaxBytes limits the performance impact of pathologically large
// messages sent by the other party. For handlers, WithReadMaxBytes limits the size
// of a message that the client can send. For clients, WithReadMaxBytes limits the
// size of a message that the server can respond with. Limits apply to each Protobuf
// message, not to the stream as a whole.
//
// Setting WithReadMaxBytes to zero allows any message size. Both clients and
// handlers default to allowing any request size.
//
// Handlers may also use [http.MaxBytesHandler] to limit the total size of the
// HTTP request stream (rather than the per-message size). Connect handles
// [http.MaxBytesError] specially, so clients still receive errors with the
// appropriate error code and informative messages.
func WithReadMaxBytes(maxBytes int) Option {
	return &readMaxBytesOption{Max: maxBytes}
}

// WithSendMaxBytes prevents sending messages too large for the client/handler
// to handle without significant performance overhead. For handlers, WithSendMaxBytes
// limits the size of a message that the handler can respond with. For clients,
// WithSendMaxBytes limits the size of a message that the client can send. Limits
// apply to each message, not to the stream as a whole.
//
// Setting WithSendMaxBytes to zero allows any message size. Both clients and
// handlers default to allowing any message size.
func WithSendMaxBytes(maxBytes int) Option {
	return &sendMaxBytesOption{Max: maxBytes}
}

// WithIdempotency declares the idempotency of the procedure. This can determine
// whether a procedure call can safely be retried, and may affect which request
// modalities are allowed for a given procedure call.
//
// In most cases, you should not need to manually set this. It is normally set
// by the code generator for your schema. For protobuf schemas, it can be set like this:
//
//	rpc Ping(PingRequest) returns (PingResponse) {
//	  option idempotency_level = NO_SIDE_EFFECTS;
//	}
func WithIdempotency(idempotencyLevel IdempotencyLevel) Option {
	return &idempotencyOption{idempotencyLevel: idempotencyLevel}
}

// WithInterceptors configures a client or handler's interceptor stack. Repeated
// WithInterceptors options are applied in order, so
//
//	WithInterceptors(A) + WithInterceptors(B, C) == WithInterceptors(A, B, C)
//
// Unary interceptors compose like an onion. The first interceptor provided is
// the outermost layer of the onion: it acts first on the context and request,
// and last on the response and error.
//
// Stream interceptors also behave like an onion: the first interceptor
// provided is the outermost wrapper for the [StreamingClientConn] or
// [StreamingHandlerConn]. It's the first to see sent messages and the last to
// see received messages.
//
// Applied to client and handler, WithInterceptors(A, B, ..., Y, Z) produces:
//
//	 client.Send()       client.Receive()
//	       |                   ^
//	       v                   |
//	    A ---                 --- A
//	    B ---                 --- B
//	    : ...                 ... :
//	    Y ---                 --- Y
//	    Z ---                 --- Z
//	       |                   ^
//	       v                   |
//	  = = = = = = = = = = = = = = = =
//	               network
//	  = = = = = = = = = = = = = = = =
//	       |                   ^
//	       v                   |
//	    A ---                 --- A
//	    B ---                 --- B
//	    : ...                 ... :
//	    Y ---                 --- Y
//	    Z ---                 --- Z
//	       |                   ^
//	       v                   |
//	handler.Receive()   handler.Send()
//	       |                   ^
//	       |                   |
//	       '-> handler logic >-'
//
// Note that in clients, Send handles the request message(s) and Receive
// handles the response message(s). For handlers, it's the reverse. Depending
// on your interceptor's logic, you may need to wrap one method in clients and
// the other in handlers.
func WithInterceptors(interceptors ...Interceptor) Option {
	return &interceptorsOption{interceptors}
}

// WithOptions composes multiple Options into one.
func WithOptions(options ...Option) Option {
	return &optionsOption{options}
}

type schemaOption struct {
	Schema any
}

func (o *schemaOption) applyToClient(config *clientConfig) {
	config.Schema = o.Schema
}

func (o *schemaOption) applyToHandler(config *handlerConfig) {
	config.Schema = o.Schema
}

type initializerOption struct {
	Initializer func(spec Spec, message any) error
}

func (o *initializerOption) applyToHandler(config *handlerConfig) {
	config.Initializer = maybeInitializer{initializer: o.Initializer}
}

func (o *initializerOption) applyToClient(config *clientConfig) {
	config.Initializer = maybeInitializer{initializer: o.Initializer}
}

type maybeInitializer struct {
	initializer func(spec Spec, message any) error
}

func (o maybeInitializer) maybe(spec Spec, message any) error {
	if o.initializer != nil {
		return o.initializer(spec, message)
	}
	return nil
}

type clientOptionsOption struct {
	options []ClientOption
}

func (o *clientOptionsOption) applyToClient(config *clientConfig) {
	for _, option := range o.options {
		option.applyToClient(config)
	}
}

type codecOption struct {
	Codec Codec
}

func (o *codecOption) applyToClient(config *clientConfig) {
	if o.Codec == nil || o.Codec.Name() == "" {
		return
	}
	config.Codec = o.Codec
}

func (o *codecOption) applyToHandler(config *handlerConfig) {
	if o.Codec == nil || o.Codec.Name() == "" {
		return
	}
	config.Codecs[o.Codec.Name()] = o.Codec
}

type readMaxBytesOption struct {
	Max int
}

func (o *readMaxBytesOption) applyToClient(config *clientConfig) {
	config.ReadMaxBytes = o.Max
}

func (o *readMaxBytesOption) applyToHandler(config *handlerConfig) {
	config.ReadMaxBytes = o.Max
}

type sendMaxBytesOption struct {
	Max int
}

func (o *sendMaxBytesOption) applyToClient(config *clientConfig) {
	config.SendMaxBytes = o.Max
}

func (o *sendMaxBytesOption) applyToHandler(config *handlerConfig) {
	config.SendMaxBytes = o.Max
}

type handlerOptionsOption struct {
	options []HandlerOption
}

func (o *handlerOptionsOption) applyToHandler(config *handlerConfig) {
	for _, option := range o.options {
		option.applyToHandler(config)
	}
}

type idempotencyOption struct {
	idempotencyLevel IdempotencyLevel
}

func (o *idempotencyOption) applyToClient(config *clientConfig) {
	config.IdempotencyLevel = o.idempotencyLevel
}

func (o *idempotencyOption) applyToHandler(config *handlerConfig) {
	config.IdempotencyLevel = o.idempotencyLevel
}

type grpcOption struct {
}

func (o *grpcOption) applyToClient(config *clientConfig) {
	config.Protocol = &protocolGRPC{}
}

type interceptorsOption struct {
	Interceptors []Interceptor
}

func (o *interceptorsOption) applyToClient(config *clientConfig) {
	config.Interceptor = o.chainWith(config.Interceptor)
}

func (o *interceptorsOption) applyToHandler(config *handlerConfig) {
	config.Interceptor = o.chainWith(config.Interceptor)
}

func (o *interceptorsOption) chainWith(current Interceptor) Interceptor {
	if len(o.Interceptors) == 0 {
		return current
	}
	if current == nil && len(o.Interceptors) == 1 {
		return o.Interceptors[0]
	}
	if current == nil && len(o.Interceptors) > 1 {
		return newChain(o.Interceptors)
	}
	return newChain(append([]Interceptor{current}, o.Interceptors...))
}

type optionsOption struct {
	options []Option
}

func (o *optionsOption) applyToClient(config *clientConfig) {
	for _, option := range o.options {
		option.applyToClient(config)
	}
}

func (o *optionsOption) applyToHandler(config *handlerConfig) {
	for _, option := range o.options {
		option.applyToHandler(config)
	}
}

func withProtoBinaryCodec() Option {
	return WithCodec(&protoBinaryCodec{})
}

type conditionalHandlerOptions struct {
	conditional func(spec Spec) []HandlerOption
}

func (o *conditionalHandlerOptions) applyToHandler(config *handlerConfig) {
	spec := config.newSpec()
	if spec.Procedure == "" {
		return // ignore empty specs
	}
	for _, option := range o.conditional(spec) {
		option.applyToHandler(config)
	}
}
