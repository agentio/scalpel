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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	connectUnaryHeaderCompression           = "Content-Encoding"
	connectUnaryHeaderAcceptCompression     = "Accept-Encoding"
	connectUnaryTrailerPrefix               = "Trailer-"
	connectStreamingHeaderCompression       = "Connect-Content-Encoding"
	connectStreamingHeaderAcceptCompression = "Connect-Accept-Encoding"
	connectHeaderTimeout                    = "Connect-Timeout-Ms"
	connectHeaderProtocolVersion            = "Connect-Protocol-Version"
	connectProtocolVersion                  = "1"
	headerVary                              = "Vary"

	connectFlagEnvelopeEndStream = 0b00000010

	connectUnaryContentTypePrefix     = "application/"
	connectUnaryContentTypeJSON       = connectUnaryContentTypePrefix + codecNameJSON
	connectStreamingContentTypePrefix = "application/connect+"
)

type connectStreamingUnmarshaler struct {
	envelopeReader

	endStreamErr *Error
	trailer      http.Header
}

func (u *connectStreamingUnmarshaler) Unmarshal(message any) *Error {
	err := u.envelopeReader.Unmarshal(message)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errSpecialEnvelope) {
		return err
	}
	env := u.last
	data := env.Data
	u.last.Data = nil // don't keep a reference to it
	defer u.bufferPool.Put(data)
	if !env.IsSet(connectFlagEnvelopeEndStream) {
		return errorf(CodeInternal, "protocol error: invalid envelope flags %d", env.Flags)
	}
	var end connectEndStreamMessage
	if err := json.Unmarshal(data.Bytes(), &end); err != nil {
		return errorf(CodeInternal, "unmarshal end stream message: %w", err)
	}
	for name, value := range end.Trailer {
		canonical := http.CanonicalHeaderKey(name)
		if name != canonical {
			delHeaderCanonical(end.Trailer, name)
			end.Trailer[canonical] = append(end.Trailer[canonical], value...)
		}
	}
	u.trailer = end.Trailer
	u.endStreamErr = end.Error.asError()
	return errSpecialEnvelope
}

func (u *connectStreamingUnmarshaler) Trailer() http.Header {
	return u.trailer
}

func (u *connectStreamingUnmarshaler) EndStreamError() *Error {
	return u.endStreamErr
}

type connectWireDetail ErrorDetail

func (d *connectWireDetail) MarshalJSON() ([]byte, error) {
	if d.wireJSON != "" {
		// If we unmarshaled this detail from JSON, return the original data. This
		// lets proxies w/o protobuf descriptors preserve human-readable details.
		return []byte(d.wireJSON), nil
	}
	wire := struct {
		Type  string          `json:"type"`
		Value string          `json:"value"`
		Debug json.RawMessage `json:"debug,omitempty"`
	}{
		Type:  typeNameForURL(d.pbAny.GetTypeUrl()),
		Value: base64.RawStdEncoding.EncodeToString(d.pbAny.GetValue()),
	}
	// Try to produce debug info, but expect failure when we don't have
	// descriptors.
	msg, err := d.getInner()
	if err == nil {
		var codec protoJSONCodec
		debug, err := codec.Marshal(msg)
		if err == nil {
			wire.Debug = debug
		}
	}
	return json.Marshal(wire)
}

func (d *connectWireDetail) UnmarshalJSON(data []byte) error {
	var wire struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if !strings.Contains(wire.Type, "/") {
		wire.Type = defaultAnyResolverPrefix + wire.Type
	}
	decoded, err := DecodeBinaryHeader(wire.Value)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	*d = connectWireDetail{
		pbAny: &anypb.Any{
			TypeUrl: wire.Type,
			Value:   decoded,
		},
		wireJSON: string(data),
	}
	return nil
}

func (d *connectWireDetail) getInner() (proto.Message, error) {
	if d.pbInner != nil {
		return d.pbInner, nil
	}
	return d.pbAny.UnmarshalNew()
}

type connectWireError struct {
	Code    Code                 `json:"code"`
	Message string               `json:"message,omitempty"`
	Details []*connectWireDetail `json:"details,omitempty"`
}

func (e *connectWireError) asError() *Error {
	if e == nil {
		return nil
	}
	if e.Code < minCode || e.Code > maxCode {
		e.Code = CodeUnknown
	}
	err := NewWireError(e.Code, errors.New(e.Message))
	if len(e.Details) > 0 {
		err.details = make([]*ErrorDetail, len(e.Details))
		for i, detail := range e.Details {
			err.details[i] = (*ErrorDetail)(detail)
		}
	}
	return err
}

func (e *connectWireError) UnmarshalJSON(data []byte) error {
	// We want to be lenient if the JSON has an unrecognized or invalid code.
	// So if that occurs, we leave the code unset but can still de-serialize
	// the other fields from the input JSON.
	var wireError struct {
		Code    string               `json:"code"`
		Message string               `json:"message"`
		Details []*connectWireDetail `json:"details"`
	}
	err := json.Unmarshal(data, &wireError)
	if err != nil {
		return err
	}
	e.Message = wireError.Message
	e.Details = wireError.Details
	// This will leave e.Code unset if we can't unmarshal the given string.
	_ = e.Code.UnmarshalText([]byte(wireError.Code))
	return nil
}

type connectEndStreamMessage struct {
	Error   *connectWireError `json:"error,omitempty"`
	Trailer http.Header       `json:"metadata,omitempty"`
}

func connectCodecForContentType(streamType StreamType, contentType string) string {
	if streamType == StreamTypeUnary {
		return strings.TrimPrefix(contentType, connectUnaryContentTypePrefix)
	}
	return strings.TrimPrefix(contentType, connectStreamingContentTypePrefix)
}

func connectValidateUnaryResponseContentType(
	requestCodecName string,
	httpMethod string,
	statusCode int,
	statusMsg string,
	responseContentType string,
) *Error {
	if statusCode != http.StatusOK {
		if statusCode == http.StatusNotModified && httpMethod == http.MethodGet {
			return NewWireError(CodeUnknown, errNotModifiedClient)
		}
		// Error responses must be JSON-encoded.
		if responseContentType == connectUnaryContentTypePrefix+codecNameJSON ||
			responseContentType == connectUnaryContentTypePrefix+codecNameJSONCharsetUTF8 {
			return nil
		}
		return NewError(
			httpToCode(statusCode),
			errors.New(statusMsg),
		)
	}
	// Normal responses must have valid content-type that indicates same codec as the request.
	if !strings.HasPrefix(responseContentType, connectUnaryContentTypePrefix) {
		// Doesn't even look like a Connect response? Use code "unknown".
		return errorf(
			CodeUnknown,
			"invalid content-type: %q; expecting %q",
			responseContentType,
			connectUnaryContentTypePrefix+requestCodecName,
		)
	}
	responseCodecName := connectCodecForContentType(
		StreamTypeUnary,
		responseContentType,
	)
	if responseCodecName == requestCodecName {
		return nil
	}
	// HACK: We likely want a better way to handle the optional "charset" parameter
	//       for application/json, instead of hard-coding. But this suffices for now.
	if (responseCodecName == codecNameJSON && requestCodecName == codecNameJSONCharsetUTF8) ||
		(responseCodecName == codecNameJSONCharsetUTF8 && requestCodecName == codecNameJSON) {
		// Both are JSON
		return nil
	}
	return errorf(
		CodeInternal,
		"invalid content-type: %q; expecting %q",
		responseContentType,
		connectUnaryContentTypePrefix+requestCodecName,
	)
}

func connectValidateStreamResponseContentType(requestCodecName string, streamType StreamType, responseContentType string) *Error {
	// Responses must have valid content-type that indicates same codec as the request.
	if !strings.HasPrefix(responseContentType, connectStreamingContentTypePrefix) {
		// Doesn't even look like a Connect response? Use code "unknown".
		return errorf(
			CodeUnknown,
			"invalid content-type: %q; expecting %q",
			responseContentType,
			connectStreamingContentTypePrefix+requestCodecName,
		)
	}
	responseCodecName := connectCodecForContentType(
		streamType,
		responseContentType,
	)
	if responseCodecName != requestCodecName {
		return errorf(
			CodeInternal,
			"invalid content-type: %q; expecting %q",
			responseContentType,
			connectStreamingContentTypePrefix+requestCodecName,
		)
	}
	return nil
}
