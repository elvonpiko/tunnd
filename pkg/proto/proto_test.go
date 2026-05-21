package proto_test

import (
	"encoding/json"
	"testing"

	"github.com/elvonpiko/tunnd/pkg/proto"
)

func TestEncodeDecode_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		msgType proto.MsgType
		payload any
		check   func(t *testing.T, env *proto.Envelope)
	}{
		{
			name:    "register",
			msgType: proto.MsgRegister,
			payload: proto.RegisterPayload{Token: "tnnd_abc", Subdomain: "myapp", Protocol: "http", LocalPort: 3000},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.RegisterPayload
				if err := proto.DecodePayload(env, &p); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if p.Token != "tnnd_abc" {
					t.Errorf("Token = %q, want tnnd_abc", p.Token)
				}
				if p.LocalPort != 3000 {
					t.Errorf("LocalPort = %d, want 3000", p.LocalPort)
				}
			},
		},
		{
			name:    "registered",
			msgType: proto.MsgRegistered,
			payload: proto.RegisteredPayload{Subdomain: "myapp", PublicURL: "https://myapp.t.test", TunnelID: "tid-1"},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.RegisteredPayload
				proto.DecodePayload(env, &p)
				if p.Subdomain != "myapp" {
					t.Errorf("Subdomain = %q", p.Subdomain)
				}
			},
		},
		{
			name:    "data",
			msgType: proto.MsgData,
			payload: proto.DataPayload{StreamID: "s1", Data: []byte("hello world")},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.DataPayload
				proto.DecodePayload(env, &p)
				if string(p.Data) != "hello world" {
					t.Errorf("Data = %q", p.Data)
				}
			},
		},
		{
			name:    "req_done",
			msgType: proto.MsgReqDone,
			payload: proto.ReqDonePayload{StreamID: "stream-xyz"},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.ReqDonePayload
				proto.DecodePayload(env, &p)
				if p.StreamID != "stream-xyz" {
					t.Errorf("StreamID = %q, want stream-xyz", p.StreamID)
				}
			},
		},
		{
			name:    "open",
			msgType: proto.MsgOpen,
			payload: proto.OpenPayload{StreamID: "open-1"},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.OpenPayload
				proto.DecodePayload(env, &p)
				if p.StreamID != "open-1" {
					t.Errorf("StreamID = %q", p.StreamID)
				}
			},
		},
		{
			name:    "error",
			msgType: proto.MsgError,
			payload: proto.ErrorPayload{Code: "auth_failed", Message: "bad token"},
			check: func(t *testing.T, env *proto.Envelope) {
				var p proto.ErrorPayload
				proto.DecodePayload(env, &p)
				if p.Code != "auth_failed" {
					t.Errorf("Code = %q", p.Code)
				}
			},
		},
		{
			name:    "pong_nil_payload",
			msgType: proto.MsgPong,
			payload: nil,
			check: func(t *testing.T, env *proto.Envelope) {
				if env.Type != proto.MsgPong {
					t.Errorf("Type = %q", env.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := proto.Encode(tt.msgType, tt.payload)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if len(raw) == 0 {
				t.Fatal("encoded bytes empty")
			}

			env, err := proto.Decode(raw)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if env.Type != tt.msgType {
				t.Errorf("Type = %q, want %q", env.Type, tt.msgType)
			}

			tt.check(t, env)
		})
	}
}

func TestDecode_RejectsInvalidJSON(t *testing.T) {
	if _, err := proto.Decode([]byte("not{json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodePayload_RejectsMalformedPayload(t *testing.T) {
	env := &proto.Envelope{
		Type:    proto.MsgError,
		Payload: json.RawMessage(`{invalid`),
	}
	var p proto.ErrorPayload
	if err := proto.DecodePayload(env, &p); err == nil {
		t.Fatal("expected error for malformed payload JSON")
	}
}

func TestBinaryFrame_RoundTrip(t *testing.T) {
	streamID := "12345678-1234-1234-1234-123456789abc" // 36 bytes
	payload := []byte("the quick brown fox jumps over the lazy dog")

	frame, err := proto.EncodeBinaryData(streamID, payload)
	if err != nil {
		t.Fatalf("EncodeBinaryData: %v", err)
	}
	if proto.FrameKind(frame) != proto.FrameKindBinaryData {
		t.Fatalf("FrameKind = %x, want %x", proto.FrameKind(frame), proto.FrameKindBinaryData)
	}

	gotID, gotPayload, err := proto.DecodeBinaryData(frame)
	if err != nil {
		t.Fatalf("DecodeBinaryData: %v", err)
	}
	if gotID != streamID {
		t.Errorf("streamID = %q, want %q", gotID, streamID)
	}
	if string(gotPayload) != string(payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestBinaryFrame_RejectsBadStreamID(t *testing.T) {
	if _, err := proto.EncodeBinaryData("too-short", []byte("data")); err == nil {
		t.Fatal("expected error for short stream id")
	}
}

func TestJSONFrame_RoundTrip(t *testing.T) {
	frame, err := proto.EncodeJSON(proto.MsgPing, nil)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if proto.FrameKind(frame) != proto.FrameKindJSON {
		t.Fatalf("FrameKind = %x, want %x", proto.FrameKind(frame), proto.FrameKindJSON)
	}

	env, err := proto.DecodeJSON(frame)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if env.Type != proto.MsgPing {
		t.Errorf("env.Type = %q, want ping", env.Type)
	}
}

func TestBinaryFrame_NoBase64Inflation(t *testing.T) {
	streamID := "12345678-1234-1234-1234-123456789abc"
	payload := make([]byte, 32*1024) // 32 KiB
	for i := range payload {
		payload[i] = byte(i)
	}

	binFrame, _ := proto.EncodeBinaryData(streamID, payload)
	jsonFrame, _ := proto.Encode(proto.MsgData, proto.DataPayload{StreamID: streamID, Data: payload})

	// Binary frame should be tiny overhead: 1 (kind) + 36 (id) + 32768 = 32805 bytes.
	// JSON frame inflates payload via base64 to ~43.7 KiB and adds JSON wrapping.
	if len(binFrame) >= len(jsonFrame) {
		t.Errorf("binary frame (%d) should be smaller than JSON frame (%d)",
			len(binFrame), len(jsonFrame))
	}
	wantBin := 1 + 36 + len(payload)
	if len(binFrame) != wantBin {
		t.Errorf("binary frame size = %d, want %d", len(binFrame), wantBin)
	}
}
