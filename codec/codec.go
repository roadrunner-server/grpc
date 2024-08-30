package codec

import (
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
)

type RawMessage []byte

// Name ..
// By default, gRPC registers and uses the "proto" codec, so it is not necessary to do this in your own code to send and receive proto messages.
// https://github.com/grpc/grpc-go/blob/master/Documentation/encoding.md#using-a-codec
const Name string = "proto"
const rm string = "rawMessage"

func (r RawMessage) Reset()       {}
func (RawMessage) ProtoMessage()  {}
func (RawMessage) String() string { return rm }

type Codec struct {
	Base encoding.CodecV2
}

// Marshal returns the wire format of v. rawMessages would be returned without encoding.
func (c *Codec) Marshal(v any) ([]byte, error) {
	if raw, ok := v.(RawMessage); ok {
		return raw, nil
	}

	data, err := c.Base.Marshal(v)
	if err != nil {
		return nil, err
	}

	return data.Materialize(), nil
}

// Unmarshal parses the wire format into v. rawMessages would not be unmarshalled.
func (c *Codec) Unmarshal(data []byte, v any) error {
	switch msg := v.(type) {
	case *RawMessage:
		if msg == nil {
			return nil
		}

		// assign data to v
		*msg = data
		return nil
	case proto.Message:
		// for the regular proto message, just unmarshal it
		err := proto.Unmarshal(data, msg)
		if err != nil {
			return err
		}
	default:
		// otherwise, use the base codec to unmarshal
		return c.Base.Unmarshal(mem.BufferSlice{mem.NewBuffer(&data, mem.DefaultBufferPool())}, v)
	}

	return nil
}

func (c *Codec) Name() string {
	return Name
}

// String return codec name.
func (c *Codec) String() string {
	return "raw:" + c.Base.Name()
}
