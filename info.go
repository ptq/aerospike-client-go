// Copyright 2013-2015 Aerospike, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aerospike

import (
	"bytes"
	"encoding/binary"
	"strings"
	"time"

	. "github.com/THE108/aerospike-client-go/logger"
	. "github.com/THE108/aerospike-client-go/types"
)

const (
	_DEFAULT_TIMEOUT = 2 * time.Second
	_NO_TIMEOUT      = 365 * 24 * time.Hour
)

// Access server's info monitoring protocol.
type info struct {
	msg *Message
}

// RequestNodeInfo gets info values by name from the specified database server node.
func RequestNodeInfo(node *Node, name ...string) (map[string]string, error) {
	conn, err := node.GetConnection(_DEFAULT_TIMEOUT)
	if err != nil {
		return nil, err
	}

	response, err := RequestInfo(conn, name...)
	if err != nil {
		node.InvalidateConnection(conn)
		return nil, err
	}
	node.PutConnection(conn)
	return response, nil
}

// RequestNodeStats returns statistics for the specified node as a map
func RequestNodeStats(node *Node) (map[string]string, error) {
	infoMap, err := RequestNodeInfo(node, "statistics")
	if err != nil {
		return nil, err
	}

	res := map[string]string{}

	v, exists := infoMap["statistics"]
	if !exists {
		return res, nil
	}

	values := strings.Split(v, ";")
	for i := range values {
		kv := strings.Split(values[i], "=")
		if len(kv) > 1 {
			res[kv[0]] = kv[1]
		}
	}

	return res, nil
}

// Send multiple commands to server and store results.
func newInfo(conn *Connection, commands ...string) (*info, error) {
	commandStr := strings.Trim(strings.Join(commands, "\n"), " ")
	if strings.Trim(commandStr, " ") != "" {
		commandStr += "\n"
	}
	newInfo := &info{
		msg: NewMessage(MSG_INFO, []byte(commandStr)),
	}

	if err := newInfo.sendCommand(conn); err != nil {
		return nil, err
	}
	return newInfo, nil
}

// RequestInfo gets info values by name from the specified connection.
func RequestInfo(conn *Connection, names ...string) (map[string]string, error) {
	info, err := newInfo(conn, names...)
	if err != nil {
		return nil, err
	}
	return info.parseMultiResponse()
}

// Issue request and set results buffer. This method is used internally.
// The static request methods should be used instead.
func (nfo *info) sendCommand(conn *Connection) error {
	// Write.
	if _, err := conn.Write(nfo.msg.Serialize()); err != nil {
		Logger.Debug("Failed to send command.")
		return err
	}

	// Read - reuse input buffer.
	header := bytes.NewBuffer(make([]byte, MSG_HEADER_SIZE))
	if _, err := conn.Read(header.Bytes(), MSG_HEADER_SIZE); err != nil {
		return err
	}
	if err := binary.Read(header, binary.BigEndian, &nfo.msg.MessageHeader); err != nil {
		Logger.Debug("Failed to read command response.")
		return err
	}

	// Logger.Debug("Header Response: %v %v %v %v", t.Type, t.Version, t.Length(), t.DataLen)
	if err := nfo.msg.Resize(nfo.msg.Length()); err != nil {
		return err
	}
	_, err := conn.Read(nfo.msg.Data, len(nfo.msg.Data))
	return err
}

func (nfo *info) parseSingleResponse(name string) (string, error) {
	return "-", nil
}

func (nfo *info) parseMultiResponse() (map[string]string, error) {
	responses := make(map[string]string)
	offset := int64(0)
	begin := int64(0)

	dataLen := int64(len(nfo.msg.Data))

	// Create reusable StringBuilder for performance.
	for offset < dataLen {
		b := nfo.msg.Data[offset]

		if b == '\t' {
			name := nfo.msg.Data[begin:offset]
			offset++
			begin = offset

			// Parse field value.
			for offset < dataLen {
				if nfo.msg.Data[offset] == '\n' {
					break
				}
				offset++
			}

			if offset > begin {
				value := nfo.msg.Data[begin:offset]
				responses[string(name)] = string(value)
			} else {
				responses[string(name)] = ""
			}
			offset++
			begin = offset
		} else if b == '\n' {
			if offset > begin {
				name := nfo.msg.Data[begin:offset]
				responses[string(name)] = ""
			}
			offset++
			begin = offset
		} else {
			offset++
		}
	}

	if offset > begin {
		name := nfo.msg.Data[begin:offset]
		responses[string(name)] = ""
	}
	return responses, nil
}
