package igo

// This file defines basic message structures, and conversions between
// 'wire format' messages and the ComposedMsg struct.

import (
    "encoding/json"
    "encoding/hex"
    "crypto/sha256"
    "crypto/hmac"
    zmq "github.com/alecthomas/gozmq"
    uuid "github.com/nu7hatch/gouuid"
)

type MsgHeader struct  {
    Msg_id string `json:"msg_id"`
    Username string `json:"username"`
    Session string `json:"session"`
    Msg_type string `json:"msg_type"`
}

// ComposedMsg represents an entire message in a high-level structure.
type ComposedMsg struct {
    Header MsgHeader
    Parent_header MsgHeader
    Metadata map[string]interface{}
    Content interface{}
}

// InvalidSignatureError is returned when the signature on a received message does not
// validate.
type InvalidSignatureError struct {}
func (e *InvalidSignatureError) Error() string {
    return "A message had an invalid signature"
}

// WireMsgToComposedMsg translates a multipart ZMQ messages received from a socket into
// a ComposedMsg struct and a slice of return identities. This includes verifying the
// message signature.
func WireMsgToComposedMsg(msgparts [][]byte, signkey []byte) (msg ComposedMsg,
                            identities [][]byte, err error) {
    i := 0
    for string(msgparts[i]) != "<IDS|MSG>" {
        i++
    }
    identities = msgparts[:i]
    // msgparts[i] is the delimiter

    // Validate signature
    if len(signkey) != 0 {
        mac := hmac.New(sha256.New, signkey)
        for _, msgpart := range msgparts[i+2:i+6] {
            mac.Write(msgpart)
        }
        signature := make([]byte, hex.DecodedLen(len(msgparts[i+1])))
        hex.Decode(signature, msgparts[i+1])
        if !hmac.Equal(mac.Sum(nil), signature) {
            return msg, nil, &InvalidSignatureError{}
        }
    }
    json.Unmarshal(msgparts[i+2], &msg.Header)
    json.Unmarshal(msgparts[i+3], &msg.Parent_header)
    json.Unmarshal(msgparts[i+4], &msg.Metadata)
    json.Unmarshal(msgparts[i+5], &msg.Content)
    return
}

// ToWireMsg translates a ComposedMsg into a multipart ZMQ message ready to send, and
// signs it. This does not add the return identities or the delimiter.
func (msg ComposedMsg) ToWireMsg(signkey []byte) (msgparts [][]byte) {
    msgparts = make([][]byte, 5)
    header, _ := json.Marshal(msg.Header)
    msgparts[1] = header
    parent_header, _ := json.Marshal(msg.Parent_header)
    msgparts[2] = parent_header
    if msg.Metadata == nil {
        msg.Metadata = make(map[string]interface{})
    }
    metadata, _ := json.Marshal(msg.Metadata)
    msgparts[3] = metadata
    content, _ := json.Marshal(msg.Content)
    msgparts[4] = content

    // Sign the message
    if len(signkey) != 0 {
        mac := hmac.New(sha256.New, signkey)
        for _, msgpart := range msgparts[1:] {
            mac.Write(msgpart)
        }
        msgparts[0] = make([]byte, hex.EncodedLen(mac.Size()))
        hex.Encode(msgparts[0], mac.Sum(nil))
    }
    return
}

// MsgReceipt represents a received message, its return identities, and the sockets for
// communication.
type MsgReceipt struct {
    Msg ComposedMsg
    Identities [][]byte
    Sockets SocketGroup
}

// SendResponse sends a message back to return identites of the received message.
func (receipt *MsgReceipt) SendResponse(socket *zmq.Socket, msg ComposedMsg) {
    socket.SendMultipart(receipt.Identities, zmq.SNDMORE)
    socket.Send([]byte("<IDS|MSG>"), zmq.SNDMORE)
    socket.SendMultipart(msg.ToWireMsg(receipt.Sockets.Key), 0)
    logger.Println("<--", msg.Header.Msg_type)
    logger.Printf("%+v\n", msg.Content)
}

// NewMsg creates a new ComposedMsg to respond to a parent message. This includes setting
// up its headers.
func NewMsg(msg_type string, parent ComposedMsg) (msg ComposedMsg) {
    msg.Parent_header = parent.Header
    msg.Header.Session = parent.Header.Session
    msg.Header.Username = parent.Header.Username
    msg.Header.Msg_type = msg_type
    u, _ := uuid.NewV4()
    msg.Header.Msg_id = u.String()
    return
}
