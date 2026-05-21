// Package proto defines the WebSocket control-plane message protocol
// shared between tunnd-server and the tunnd client.
//
// Two frame kinds travel over the WebSocket:
//
//   - Kind 0x01 (FrameKindJSON): a tagged JSON Envelope. Used for all control
//     messages — register, registered, error, open, open_tcp, req_done, close,
//     ping, pong. These are infrequent and structured.
//
//   - Kind 0x02 (FrameKindBinaryData): a tagged binary payload — the first byte
//     is the kind, the next 36 bytes are the stream UUID (ASCII), and the rest
//     is the raw byte stream. Used for high-volume MsgData traffic. Saves the
//     ~33% base64 inflation and avoids per-chunk JSON parsing.
package proto

import (
	"encoding/json"
	"fmt"
)

// MsgType identifies a control-plane message.
type MsgType string

const (
	// Client → Server
	MsgRegister MsgType = "register" // request a tunnel subdomain
	MsgPing     MsgType = "ping"

	// Server → Client
	MsgRegistered MsgType = "registered" // tunnel is live, public URL confirmed
	MsgError      MsgType = "error"
	MsgPong       MsgType = "pong"
	MsgOpen       MsgType = "open"     // open a local connection for this stream (HTTP)
	MsgOpenTCP    MsgType = "open_tcp" // open a local TCP connection — fully bidirectional
	MsgReqDone    MsgType = "req_done" // server finished sending request bytes (HTTP)

	// Bidirectional
	MsgData  MsgType = "data"  // raw bytes for a stream
	MsgClose MsgType = "close" // stream is fully done (both sides)
)

// Envelope wraps every control message.
type Envelope struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RegisterPayload is sent by the client to request a tunnel.
type RegisterPayload struct {
	Token     string `json:"token"`
	Subdomain string `json:"subdomain,omitempty"`
	Protocol  string `json:"protocol"` // "http" | "tcp"
	LocalPort int    `json:"local_port"`
}

// RegisteredPayload confirms a tunnel is live.
type RegisteredPayload struct {
	Subdomain string `json:"subdomain"`
	PublicURL string `json:"public_url"`
	TunnelID  string `json:"tunnel_id"`
}

// ErrorPayload carries a server-side error to the client.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OpenPayload tells the client to open a local connection for this stream.
type OpenPayload struct {
	StreamID string `json:"stream_id"`
}

// ReqDonePayload signals that the server has finished sending the HTTP request
// bytes for a stream. The client should now read the local response and start
// sending MsgData frames back.
type ReqDonePayload struct {
	StreamID string `json:"stream_id"`
}

// DataPayload carries raw bytes for a stream (bidirectional).
type DataPayload struct {
	StreamID string `json:"stream_id"`
	Data     []byte `json:"data"` // base64-encoded in JSON
}

// ClosePayload signals that a stream is fully done.
type ClosePayload struct {
	StreamID string `json:"stream_id"`
}

// Frame kinds
//
// On the wire we send two kinds of frames over the WebSocket:
//
//  1. JSON envelopes — for control messages (rare, structured): register,
//     registered, error, open, open_tcp, req_done, ping, pong, close.
//     First byte is FrameKindJSON, remaining bytes are JSON of an Envelope.
//
//  2. Binary data frames — for raw stream bytes (frequent, large):
//     First byte is FrameKindBinaryData, next 36 bytes are the stream UUID
//     (ASCII), the rest is the raw payload. Saves 33% bandwidth (no base64)
//     and avoids JSON parsing per chunk.
const (
	FrameKindJSON       byte = 0x01
	FrameKindBinaryData byte = 0x02

	// streamIDLen is the fixed length of a UUID string in ASCII.
	streamIDLen = 36
)

// EncodeBinaryData builds a binary data frame: [FrameKindBinaryData, streamID(36), payload...].
// streamID must be exactly streamIDLen bytes long.
func EncodeBinaryData(streamID string, payload []byte) ([]byte, error) {
	if len(streamID) != streamIDLen {
		return nil, fmt.Errorf("streamID must be %d bytes, got %d", streamIDLen, len(streamID))
	}
	out := make([]byte, 1+streamIDLen+len(payload))
	out[0] = FrameKindBinaryData
	copy(out[1:1+streamIDLen], streamID)
	copy(out[1+streamIDLen:], payload)
	return out, nil
}

// DecodeBinaryData parses a binary data frame and returns (streamID, payload).
// Caller is responsible for retaining payload (it shares memory with raw).
func DecodeBinaryData(raw []byte) (string, []byte, error) {
	if len(raw) < 1+streamIDLen {
		return "", nil, fmt.Errorf("binary frame too short: %d bytes", len(raw))
	}
	if raw[0] != FrameKindBinaryData {
		return "", nil, fmt.Errorf("not a binary data frame: kind=0x%02x", raw[0])
	}
	return string(raw[1 : 1+streamIDLen]), raw[1+streamIDLen:], nil
}

// EncodeJSON wraps an Envelope as a kind-tagged JSON frame.
// Use this for control messages.
func EncodeJSON(msgType MsgType, payload any) ([]byte, error) {
	body, err := Encode(msgType, payload)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+len(body))
	out[0] = FrameKindJSON
	copy(out[1:], body)
	return out, nil
}

// DecodeJSON parses a kind-tagged JSON envelope frame.
func DecodeJSON(raw []byte) (*Envelope, error) {
	if len(raw) < 1 {
		return nil, fmt.Errorf("frame too short")
	}
	if raw[0] != FrameKindJSON {
		return nil, fmt.Errorf("not a JSON frame: kind=0x%02x", raw[0])
	}
	return Decode(raw[1:])
}

// FrameKind returns the first-byte kind of a frame, or 0 if empty.
func FrameKind(raw []byte) byte {
	if len(raw) == 0 {
		return 0
	}
	return raw[0]
}
func Encode(msgType MsgType, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Envelope{Type: msgType, Payload: raw})
}

// Decode unmarshals raw bytes into an Envelope.
func Decode(data []byte) (*Envelope, error) {
	var env Envelope
	return &env, json.Unmarshal(data, &env)
}

// DecodePayload unmarshals the Payload field of an Envelope into dst.
func DecodePayload(env *Envelope, dst any) error {
	return json.Unmarshal(env.Payload, dst)
}
